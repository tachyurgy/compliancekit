package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"database/sql"
	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
	"github.com/tachyurgy/compliancekit/backend/internal/ocr"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

type DocumentHandler struct {
	Pool        *sql.DB
	Storage     *storage.Client
	OCR         ocr.OCR
	ExpiryExtra *ocr.ExpirationExtractor
	Log         *slog.Logger
}

// POST /api/documents/presign
// { "subject_kind":"child","subject_id":"...","kind":"immunization_record","mime_type":"application/pdf" }
// -> { "document_id": "...", "upload_url": "...", "storage_key": "..." }
func (h *DocumentHandler) Presign(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	var in struct {
		SubjectKind string `json:"subject_kind"`
		SubjectID   string `json:"subject_id"`
		Kind        string `json:"kind"`
		MIMEType    string `json:"mime_type"`
		Title       string `json:"title"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.SubjectKind == "" || in.Kind == "" || in.MIMEType == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("subject_kind, kind, mime_type required"))
		return
	}
	// Refuse to mint a presigned URL the browser will PUT against a bucket
	// we can't actually write to. Without AWS creds (or a MinIO endpoint) the
	// upload will bounce with 403; surfacing 503 here gives the UI a clear
	// signal to tell the user "document upload isn't configured yet."
	if !h.Storage.IsConfigured() {
		httpx.RenderError(w, r, &httpx.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "storage_not_configured",
			Message: "document upload is not configured; set AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY (or AWS_ENDPOINT_URL_S3 for local MinIO)",
		})
		return
	}

	docID := base62.NewID()[:22]
	bucket := h.Storage.Buckets().RawUploads
	key := fmt.Sprintf("providers/%s/%s/%s/%s/%s",
		pid, in.SubjectKind, safeID(in.SubjectID), in.Kind, docID+"."+extFor(in.MIMEType))

	url, err := h.Storage.PresignPutURL(r.Context(), bucket, key, in.MIMEType, 15*time.Minute)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	title := in.Title
	if title == "" {
		title = strings.ReplaceAll(in.Kind, "_", " ")
	}
	// insert pending row — OCR fills expires_at later
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO documents (id, provider_id, subject_kind, subject_id, kind, title,
		                      storage_bucket, storage_key, mime_type, size_bytes,
		                      uploaded_by, uploaded_via, created_at, updated_at)
		VALUES (?,?,?,NULLIF(?,''),?,?,?,?,?,0,?,'provider',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		docID, pid, in.SubjectKind, in.SubjectID, in.Kind, title, bucket, key, in.MIMEType, mw.UserIDFrom(r.Context())); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"document_id": docID, "upload_url": url, "storage_key": key,
	})
}

// POST /api/documents/{id}/finalize — called after the browser completes the S3 PUT.
// Kicks off OCR + expiration extraction synchronously (for v1).
func (h *DocumentHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	var (
		bucket, key, mime, kind string
		sizeBytes               int64
	)
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT storage_bucket, storage_key, mime_type, kind FROM documents
		WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		id, pid).Scan(&bucket, &key, &mime, &kind)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}

	// Download the blob, run OCR, extract expiration.
	body, _, err := h.Storage.GetDocument(r.Context(), key)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer body.Close()

	var (
		expires    *time.Time
		confidence float64
		source     string
	)
	if h.OCR != nil && h.ExpiryExtra != nil {
		res, err := h.OCR.Extract(r.Context(), body, mime)
		if err != nil {
			h.Log.Warn("finalize: ocr failed", "err", err, "doc", id)
		} else if er, err := h.ExpiryExtra.Extract(r.Context(), res.Text, kind); err != nil {
			h.Log.Warn("finalize: expiry extract failed", "err", err, "doc", id)
		} else {
			source = res.Source
			confidence = er.Confidence
			if er.ExpirationDate != "" {
				if t, err := time.Parse("2006-01-02", er.ExpirationDate); err == nil {
					expires = &t
				}
			}
		}
	}

	if _, err := h.Pool.ExecContext(r.Context(), `
		UPDATE documents SET size_bytes = ?, expires_at = ?, ocr_confidence = ?, ocr_source = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND provider_id = ?`,
		sizeBytes, expires, confidence, source, id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitDocumentUpload(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, map[string]any{
		"kind":           kind,
		"mime_type":      mime,
		"ocr_source":     source,
		"ocr_confidence": confidence,
	}, r)
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"document_id": id, "expires_at": expires, "ocr_confidence": confidence, "ocr_source": source,
	})
}

// GET /api/documents/{id}
func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	var d models.Document
	var kind string
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id,''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
		       created_at, updated_at, deleted_at
		FROM documents WHERE id = ? AND provider_id = ?`, id, pid).
		Scan(&d.ID, &d.ProviderID, &d.SubjectKind, &d.SubjectID, &kind, &d.Title,
			&d.StorageBucket, &d.StorageKey, &d.MIMEType, &d.SizeBytes,
			&d.IssuedAt, &d.ExpiresAt, &d.OCRConfidence, &d.OCRSource,
			&d.UploadedBy, &d.UploadedVia, &d.LastChaseSentAt,
			&d.CreatedAt, &d.UpdatedAt, &d.DeletedAt)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	d.Kind = models.DocumentKind(kind)
	// include a short-lived presigned GET
	getURL, _ := h.Storage.PresignGetURL(r.Context(), d.StorageBucket, d.StorageKey, 10*time.Minute)
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"document": d, "download_url": getURL})
}

// GET /api/documents?subject_kind=child&subject_id=...  (or provider-wide)
func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	q := r.URL.Query()
	subjectKind := q.Get("subject_kind")
	subjectID := q.Get("subject_id")

	where := "provider_id = ? AND deleted_at IS NULL"
	args := []any{pid}
	if subjectKind != "" {
		where += " AND subject_kind = ?"
		args = append(args, subjectKind)
	}
	if subjectID != "" {
		where += " AND subject_id = ?"
		args = append(args, subjectID)
	}

	// NOTE: the `documents` table has two overlapping column sets (migration
	// 000004's canonical names: owner_kind/owner_id/doc_type/s3_key, and the
	// handler-era aliases: subject_kind/subject_id/kind/storage_key). Columns
	// `issued_at`, `ocr_source`, `uploaded_by`, `last_chase_sent_at` were never
	// migrated in; we synthesize defaults for them so List doesn't 500. See
	// handlers/dashboard.go loadDocs for the mirror query.
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, provider_id,
		       COALESCE(subject_kind, owner_kind, '') AS subject_kind,
		       COALESCE(subject_id, owner_id, '') AS subject_id,
		       COALESCE(kind, doc_type, '') AS kind,
		       COALESCE(title, original_filename, '') AS title,
		       COALESCE(storage_bucket, '') AS storage_bucket,
		       COALESCE(storage_key, s3_key, '') AS storage_key,
		       COALESCE(mime_type, '') AS mime_type,
		       COALESCE(size_bytes, byte_size, 0) AS size_bytes,
		       NULL AS issued_at,
		       expiration_date AS expires_at,
		       COALESCE(ocr_confidence, 0),
		       '' AS ocr_source,
		       '' AS uploaded_by,
		       COALESCE(uploaded_via, ''),
		       NULL AS last_chase_sent_at,
		       created_at, updated_at, deleted_at
		FROM documents WHERE `+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	out := make([]models.Document, 0)
	for rows.Next() {
		var d models.Document
		var kind string
		if err := rows.Scan(&d.ID, &d.ProviderID, &d.SubjectKind, &d.SubjectID, &kind, &d.Title,
			&d.StorageBucket, &d.StorageKey, &d.MIMEType, &d.SizeBytes,
			&d.IssuedAt, &d.ExpiresAt, &d.OCRConfidence, &d.OCRSource,
			&d.UploadedBy, &d.UploadedVia, &d.LastChaseSentAt,
			&d.CreatedAt, &d.UpdatedAt, &d.DeletedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		d.Kind = models.DocumentKind(kind)
		out = append(out, d)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// DELETE /api/documents/{id}  (soft delete)
func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE documents SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND provider_id = ?`,
		id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitDocumentDelete(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, r)
	w.WriteHeader(http.StatusNoContent)
}

func safeID(s string) string {
	if s == "" {
		return "_"
	}
	// Only allow base62-ish chars in S3 keys
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c == '-', c == '_':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	return string(b)
}

func extFor(mime string) string {
	switch mime {
	case "application/pdf":
		return "pdf"
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/heic":
		return "heic"
	case "image/webp":
		return "webp"
	default:
		return "bin"
	}
}
