package handlers

import (
	"context"
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
//
// Writes both the working values (`expires_at`, `kind`) AND the OCR-extracted
// audit values (`expiration_extracted_value`, `document_type_extracted_value`,
// `extraction_confidence`) per REQ-006. Until a human confirms via /confirm,
// `expiration_confirmed_at` stays NULL and the chase engine treats this row
// as advisory.
func (h *DocumentHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	h.runFinalize(w, r, pid, id, mw.UserIDFrom(r.Context()))
}

// runFinalize is the shared implementation reused by the portal upload flow.
// userID may be empty for parent/staff portal uploads — uploaded_by_user_id
// is always nullable in that case.
func (h *DocumentHandler) runFinalize(w http.ResponseWriter, r *http.Request, pid, id, userID string) {
	var (
		bucket, key, mime, kind string
	)
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT storage_bucket, storage_key, mime_type, kind FROM documents
		WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		id, pid).Scan(&bucket, &key, &mime, &kind)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}

	// Download the blob, run OCR, extract expiration. Browser uploads land
	// in the raw-uploads bucket via presigned PUT; documents.storage_bucket
	// records where the bytes actually live so we read from the right place.
	body, _, err := h.Storage.GetFromBucket(r.Context(), bucket, key)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer body.Close()

	var (
		expiresStr   string
		expires      *time.Time
		ocrConfidence float64
		extractConf  float64
		source       string
	)
	if h.OCR != nil && h.ExpiryExtra != nil {
		res, err := h.OCR.Extract(r.Context(), body, mime)
		if err != nil {
			h.Log.Warn("finalize: ocr failed", "err", err, "doc", id)
		} else if er, err := h.ExpiryExtra.Extract(r.Context(), res.Text, kind); err != nil {
			h.Log.Warn("finalize: expiry extract failed", "err", err, "doc", id)
		} else {
			source = res.Source
			ocrConfidence = res.Confidence
			extractConf = er.Confidence
			expiresStr = er.ExpirationDate
			if er.ExpirationDate != "" {
				if t, err := time.Parse("2006-01-02", er.ExpirationDate); err == nil {
					expires = &t
				}
			}
		}
	}

	// Heuristic backstop for extraction_confidence: if the model gave us a
	// recognizable date, treat as high (0.95); empty string + extracted
	// confidence 0 → 0.0; otherwise pass the model's number through.
	if extractConf == 0 && expiresStr != "" {
		extractConf = 0.95
	}

	if _, err := h.Pool.ExecContext(r.Context(), `
		UPDATE documents
		   SET expires_at = ?,
		       expiration_date = COALESCE(expiration_date, ?),
		       expiration_extracted_value = ?,
		       document_type_extracted_value = COALESCE(document_type_extracted_value, ?),
		       extraction_confidence = ?,
		       ocr_confidence = ?,
		       ocr_source = ?,
		       ocr_status = 'completed',
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND provider_id = ?`,
		expires, expires, expiresStr, kind, extractConf, ocrConfidence, source,
		id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// REQ-007: supersede prior active documents of the same kind for the
	// same subject. The chase engine filters on superseded_at IS NULL, so
	// flipping this here (right after OCR completes) stops any further
	// reminders against the old doc as soon as a replacement lands. We
	// stamp superseded_by_document_id with the new doc id so the audit
	// trail records what replaced what; the operator UI can render
	// "replaced by …" from this pair.
	if err := supersedePriorDocuments(r.Context(), h.Pool, pid, id); err != nil {
		// Non-fatal: the upload + OCR already succeeded. Log + continue.
		h.Log.Warn("finalize: supersede flip failed", "err", err, "doc", id)
	}
	auditlog.EmitDocumentUpload(r.Context(), h.Pool, pid, userID, id, map[string]any{
		"kind":                  kind,
		"mime_type":             mime,
		"ocr_source":            source,
		"ocr_confidence":        ocrConfidence,
		"extraction_confidence": extractConf,
	}, r)
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"document_id":           id,
		"expires_at":            expires,
		"ocr_confidence":        ocrConfidence,
		"extraction_confidence": extractConf,
		"ocr_source":            source,
		// echoed for the portal UI so it can render the confirmation card
		// without a second round-trip
		"document_type_extracted_value": kind,
		"expiration_extracted_value":    expiresStr,
	})
}

// confirmInput is the body shape for both manager and portal confirmation.
type confirmInput struct {
	DocumentType       string `json:"document_type"`
	ExpirationDate     string `json:"expiration_date"` // YYYY-MM-DD
	DoesNotExpire      bool   `json:"does_not_expire"`
	AcceptedExtraction bool   `json:"accepted_extraction"`
}

// POST /api/documents/{id}/confirm — manager-side confirmation. Stamps
// expiration_confirmed_at, persists user-edited document_type / expiration_date
// (preserving the *_extracted_value audit fields untouched).
//
// REQ-006: until this fires, the chase engine should not nag for this doc.
func (h *DocumentHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	uid := mw.UserIDFrom(r.Context())
	h.runConfirm(w, r, pid, id, uid)
}

func (h *DocumentHandler) runConfirm(w http.ResponseWriter, r *http.Request, pid, id, userID string) {
	var in confirmInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.DocumentType == "" && !in.DoesNotExpire && in.ExpirationDate == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("document_type plus expiration_date or does_not_expire is required"))
		return
	}

	var expires *time.Time
	if !in.DoesNotExpire && in.ExpirationDate != "" {
		t, err := time.Parse("2006-01-02", in.ExpirationDate)
		if err != nil {
			httpx.RenderError(w, r, httpx.BadRequestf("expiration_date must be YYYY-MM-DD"))
			return
		}
		expires = &t
	}

	res, err := h.Pool.ExecContext(r.Context(), `
		UPDATE documents
		   SET kind = COALESCE(NULLIF(?, ''), kind),
		       doc_type = COALESCE(NULLIF(?, ''), doc_type),
		       expires_at = ?,
		       expiration_date = ?,
		       does_not_expire = ?,
		       expiration_confirmed_at = CURRENT_TIMESTAMP,
		       expiration_confirmed_by_user_id = NULLIF(?, ''),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		in.DocumentType, in.DocumentType,
		expires, expires,
		boolToInt(in.DoesNotExpire),
		userID,
		id, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     "document.confirm",
		TargetKind: "document",
		TargetID:   id,
		Metadata: map[string]any{
			"accepted_extraction": in.AcceptedExtraction,
			"does_not_expire":     in.DoesNotExpire,
		},
		IP:        auditlog.ClientIP(r),
		UserAgent: r.UserAgent(),
	})
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"document_id":              id,
		"expiration_confirmed_at": time.Now().UTC().Format(time.RFC3339),
		"does_not_expire":         in.DoesNotExpire,
	})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// supersedePriorDocuments marks every prior active document with the same
// (provider_id, subject_kind, subject_id, kind) as superseded by newID.
// "Active" = deleted_at IS NULL AND superseded_at IS NULL. Idempotent —
// running twice is a no-op because the WHERE filter excludes already-
// superseded rows.
//
// Used by:
//   - runFinalize, when a new upload completes OCR.
//   - SupersedeStaffDocument / explicit manager endpoint.
//
// The chase candidate query filters on superseded_at IS NULL so a stale
// "your CPR expires" never goes out after the parent uploaded the new CPR.
func supersedePriorDocuments(ctx context.Context, pool *sql.DB, providerID, newDocID string) error {
	// Resolve the new doc's identity. We need (subject_kind, subject_id, kind)
	// to find peers. subject_id can be NULL for provider-level docs; in that
	// case we don't supersede (the concept doesn't apply — a fire-drill
	// certificate uploaded today shouldn't retire last quarter's drill log).
	var subjectKind, subjectID, kind string
	err := pool.QueryRowContext(ctx, `
		SELECT COALESCE(subject_kind,''), COALESCE(subject_id,''), COALESCE(kind,'')
		FROM documents WHERE id = ? AND provider_id = ?`, newDocID, providerID).
		Scan(&subjectKind, &subjectID, &kind)
	if err != nil {
		return fmt.Errorf("supersede: lookup new doc: %w", err)
	}
	if subjectKind == "" || subjectID == "" || kind == "" {
		return nil
	}
	_, err = pool.ExecContext(ctx, `
		UPDATE documents
		   SET superseded_at = CURRENT_TIMESTAMP,
		       superseded_by_document_id = ?,
		       updated_at = CURRENT_TIMESTAMP
		 WHERE provider_id = ?
		   AND subject_kind = ?
		   AND subject_id = ?
		   AND kind = ?
		   AND id != ?
		   AND deleted_at IS NULL
		   AND superseded_at IS NULL`,
		newDocID, providerID, subjectKind, subjectID, kind, newDocID)
	if err != nil {
		return fmt.Errorf("supersede: update peers: %w", err)
	}
	return nil
}

// POST /api/staff/{id}/document/{doc_id}/supersede — REQ-007 manual
// supersede. Admin-only. Sets superseded_at = now() on the document
// without a replacement (superseded_by_document_id stays NULL). Idempotent.
//
// Used when an operator wants to retire an old doc that the auto-supersede
// path can't infer (e.g. they uploaded the replacement under a different
// kind, or the staff member transferred and the old cert is no longer
// relevant). Routed under /api/staff/{id}/... so the path matches the
// REQ-007 spec; we verify the document belongs to the staff member and
// the provider.
func (h *DocumentHandler) ManualSupersede(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	staffID := chi.URLParam(r, "id")
	docID := chi.URLParam(r, "doc_id")
	if staffID == "" || docID == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("staff id and document id required"))
		return
	}
	// Verify ownership: doc belongs to the provider, to the staff subject,
	// and is not already superseded or deleted. Idempotent: if it's already
	// superseded we return 200 without writing.
	var subjectKind, subjectID string
	var alreadySuperseded sql.NullString
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT COALESCE(subject_kind,''), COALESCE(subject_id,''), superseded_at
		FROM documents WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		docID, pid).Scan(&subjectKind, &subjectID, &alreadySuperseded)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	if subjectKind != "staff" || subjectID != staffID {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	if alreadySuperseded.Valid && alreadySuperseded.String != "" {
		httpx.RenderJSON(w, http.StatusOK, map[string]any{
			"document_id":   docID,
			"superseded_at": alreadySuperseded.String,
			"idempotent":    true,
		})
		return
	}
	if _, err := h.Pool.ExecContext(r.Context(), `
		UPDATE documents
		   SET superseded_at = CURRENT_TIMESTAMP,
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND provider_id = ?`,
		docID, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    mw.UserIDFrom(r.Context()),
		Action:     "document.supersede",
		TargetKind: "document",
		TargetID:   docID,
		Metadata:   map[string]any{"manual": true},
		IP:         auditlog.ClientIP(r),
		UserAgent:  r.UserAgent(),
	})
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"document_id":   docID,
		"superseded_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// GET /api/documents/{id}
func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	d, err := h.loadDocument(r.Context(), pid, id)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	getURL, _ := h.Storage.PresignGetURL(r.Context(), d.StorageBucket, d.StorageKey, 10*time.Minute)
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"document": d, "download_url": getURL})
}

// loadDocument is the canonical read shape for one document — used by
// /api/documents/:id and the portal's /portal/document/:id endpoint. Returns
// every OCR-confirmation field (REQ-006) so the UI can render the
// extracted-vs-confirmed state without a second round-trip.
func (h *DocumentHandler) loadDocument(ctx context.Context, pid, id string) (models.Document, error) {
	var d models.Document
	var kind string
	var dne int
	err := h.Pool.QueryRowContext(ctx, `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id,''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
		       COALESCE(expiration_extracted_value,''), COALESCE(document_type_extracted_value,''),
		       COALESCE(extraction_confidence,0), expiration_confirmed_at,
		       COALESCE(expiration_confirmed_by_user_id,''), COALESCE(does_not_expire,0),
		       created_at, updated_at, deleted_at
		FROM documents WHERE id = ? AND provider_id = ?`, id, pid).
		Scan(&d.ID, &d.ProviderID, &d.SubjectKind, &d.SubjectID, &kind, &d.Title,
			&d.StorageBucket, &d.StorageKey, &d.MIMEType, &d.SizeBytes,
			&d.IssuedAt, &d.ExpiresAt, &d.OCRConfidence, &d.OCRSource,
			&d.UploadedBy, &d.UploadedVia, &d.LastChaseSentAt,
			&d.ExpirationExtractedValue, &d.DocumentTypeExtractedValue,
			&d.ExtractionConfidence, &d.ExpirationConfirmedAt,
			&d.ExpirationConfirmedByUserID, &dne,
			&d.CreatedAt, &d.UpdatedAt, &d.DeletedAt)
	if err != nil {
		return models.Document{}, err
	}
	d.Kind = models.DocumentKind(kind)
	d.DoesNotExpire = dne != 0
	return d, nil
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
