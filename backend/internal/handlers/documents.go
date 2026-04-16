package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
	"github.com/tachyurgy/compliancekit/backend/internal/ocr"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

type DocumentHandler struct {
	Pool         *pgxpool.Pool
	Storage      *storage.Client
	OCR          ocr.OCR
	ExpiryExtra  *ocr.ExpirationExtractor
	Log          *slog.Logger
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
	if _, err := h.Pool.Exec(r.Context(), `
		INSERT INTO documents (id, provider_id, subject_kind, subject_id, kind, title,
		                      storage_bucket, storage_key, mime_type, size_bytes,
		                      uploaded_by, uploaded_via, created_at, updated_at)
		VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,0,$10,'provider',NOW(),NOW())`,
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
	err := h.Pool.QueryRow(r.Context(), `
		SELECT storage_bucket, storage_key, mime_type, kind FROM documents
		WHERE id = $1 AND provider_id = $2 AND deleted_at IS NULL`,
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

	if _, err := h.Pool.Exec(r.Context(), `
		UPDATE documents SET size_bytes = $3, expires_at = $4, ocr_confidence = $5, ocr_source = $6, updated_at = NOW()
		WHERE id = $1 AND provider_id = $2`,
		id, pid, sizeBytes, expires, confidence, source); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
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
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id,''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
		       created_at, updated_at, deleted_at
		FROM documents WHERE id = $1 AND provider_id = $2`, id, pid).
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

	where := "provider_id = $1 AND deleted_at IS NULL"
	args := []any{pid}
	if subjectKind != "" {
		where += fmt.Sprintf(" AND subject_kind = $%d", len(args)+1)
		args = append(args, subjectKind)
	}
	if subjectID != "" {
		where += fmt.Sprintf(" AND subject_id = $%d", len(args)+1)
		args = append(args, subjectID)
	}

	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id,''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
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
	if _, err := h.Pool.Exec(r.Context(),
		`UPDATE documents SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND provider_id = $2`,
		id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
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
