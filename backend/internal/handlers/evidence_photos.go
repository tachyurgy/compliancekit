package handlers

// REQ-038: arbitrary photo evidence on drills + postings (and any other
// subject — schema's generic). We store the bytes in S3 the same way
// document uploads do, but skip OCR/extraction. The point is just "snap
// a photo, attach it to this drill/posting so the inspector can see
// staff signed the muster sheet."
//
// Endpoints:
//   POST   /api/evidence-photos        multipart, one or more files
//   GET    /api/evidence-photos?subjectKind=...&subjectId=...
//   PATCH  /api/evidence-photos/{id}   caption-only edit
//   DELETE /api/evidence-photos/{id}   soft delete
//
// Storage path: providers/<pid>/evidence/<subjectKind>/<safeSubjectID>/<photoID>.<ext>

import (
	"database/sql"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/dbtime"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

// EvidencePhotoHandler owns CRUD for the evidence_photos table.
type EvidencePhotoHandler struct {
	Pool    *sql.DB
	Storage *storage.Client
	Log     *slog.Logger
}

// EvidencePhoto is the JSON projection of an evidence_photos row, plus a
// short-lived download URL the UI can render directly into an <img>.
type EvidencePhoto struct {
	ID               string    `json:"id"`
	ProviderID       string    `json:"provider_id"`
	SubjectKind      string    `json:"subject_kind"`
	SubjectID        string    `json:"subject_id"`
	StorageBucket    string    `json:"storage_bucket"`
	StorageKey       string    `json:"storage_key"`
	MIMEType         string    `json:"mime_type"`
	Caption          string    `json:"caption"`
	UploadedByUserID string    `json:"uploaded_by_user_id,omitempty"`
	DownloadURL      string    `json:"download_url,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

var validEvidenceSubjects = map[string]struct{}{
	"drill": {}, "posting": {}, "generic": {},
}

var validEvidenceMIME = map[string]string{
	"image/jpeg": "jpg",
	"image/jpg":  "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/heic": "heic",
	"image/gif":  "gif",
}

// 25 MiB per file. Larger than a phone JPG, small enough that an attacker
// can't OOM the box. We rely on http.MaxBytesReader at the request level
// to cap the total request size; multipart parsing here uses
// ParseMultipartForm with 10 MiB in-memory buffer so larger uploads spool
// to temp files.
const evidencePhotoMaxBytes = 25 * 1024 * 1024

// Create is multipart: each file part is one photo. Supports an optional
// "caption" form field (applied to every uploaded photo in the batch).
// Multipart layout:
//
//	subject_kind   text
//	subject_id     text
//	caption        text (optional, applied to all photos in this batch)
//	files          one or more file parts (any field name accepted)
func (h *EvidencePhotoHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	uid := mw.UserIDFrom(r.Context())

	if !h.Storage.IsConfigured() {
		httpx.RenderError(w, r, &httpx.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "storage_not_configured",
			Message: "photo upload is not configured; AWS credentials missing",
		})
		return
	}

	// Cap the request body up front so a runaway client can't fill the disk.
	// Cap of 8x per-file gives 8 photos at the per-file limit; in practice
	// the operator UI uploads <= 4 at a time.
	r.Body = http.MaxBytesReader(w, r.Body, evidencePhotoMaxBytes*8)
	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		httpx.RenderError(w, r, httpx.BadRequestf("could not parse multipart form: %v", err))
		return
	}

	subjectKind := strings.TrimSpace(r.FormValue("subject_kind"))
	subjectID := strings.TrimSpace(r.FormValue("subject_id"))
	caption := strings.TrimSpace(r.FormValue("caption"))
	if _, ok := validEvidenceSubjects[subjectKind]; !ok {
		httpx.RenderError(w, r, httpx.BadRequestf("subject_kind must be one of drill|posting|generic"))
		return
	}
	if subjectID == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("subject_id required"))
		return
	}

	// Collect every file part. Multipart accepts repeated form keys; we
	// don't care which key the client used as long as a file part is
	// present.
	var parts []*multipart.FileHeader
	if r.MultipartForm != nil {
		for _, fhSlice := range r.MultipartForm.File {
			parts = append(parts, fhSlice...)
		}
	}
	if len(parts) == 0 {
		httpx.RenderError(w, r, httpx.BadRequestf("at least one file part required"))
		return
	}

	bucket := h.Storage.Buckets().RawUploads
	out := make([]EvidencePhoto, 0, len(parts))

	for _, fh := range parts {
		mime := fh.Header.Get("Content-Type")
		if mime == "" {
			mime = "application/octet-stream"
		}
		ext, ok := validEvidenceMIME[mime]
		if !ok {
			httpx.RenderError(w, r, httpx.BadRequestf("unsupported mime %q (allowed: jpeg/png/webp/heic/gif)", mime))
			return
		}
		if fh.Size > evidencePhotoMaxBytes {
			httpx.RenderError(w, r, httpx.BadRequestf("file %q exceeds %d byte limit", fh.Filename, evidencePhotoMaxBytes))
			return
		}

		photoID := base62.NewID()[:22]
		key := fmt.Sprintf("providers/%s/evidence/%s/%s/%s.%s",
			pid, subjectKind, safeID(subjectID), photoID, ext)

		f, err := fh.Open()
		if err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		if err := h.Storage.PutToBucket(r.Context(), bucket, key, mime, f); err != nil {
			f.Close()
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		f.Close()

		var userArg any
		if uid != "" {
			userArg = uid
		}
		if _, err := h.Pool.ExecContext(r.Context(), `
			INSERT INTO evidence_photos (id, provider_id, subject_kind, subject_id,
			                             storage_bucket, storage_key, mime_type,
			                             caption, uploaded_by_user_id, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
			photoID, pid, subjectKind, subjectID, bucket, key, mime, caption, userArg); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}

		photo := EvidencePhoto{
			ID:               photoID,
			ProviderID:       pid,
			SubjectKind:      subjectKind,
			SubjectID:        subjectID,
			StorageBucket:    bucket,
			StorageKey:       key,
			MIMEType:         mime,
			Caption:          caption,
			UploadedByUserID: uid,
			CreatedAt:        time.Now().UTC(),
		}
		if url, err := h.Storage.PresignGetURL(r.Context(), bucket, key, 30*time.Minute); err == nil {
			photo.DownloadURL = url
		}
		out = append(out, photo)

		auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
			ProviderID: pid,
			ActorKind:  auditlog.ActorKindProviderAdmin,
			ActorID:    uid,
			Action:     "evidence_photo.create",
			TargetKind: "evidence_photo",
			TargetID:   photoID,
			Metadata: map[string]any{
				"subject_kind": subjectKind,
				"subject_id":   subjectID,
				"mime_type":    mime,
			},
			IP:        auditlog.ClientIP(r),
			UserAgent: r.UserAgent(),
		})
	}

	httpx.RenderJSON(w, http.StatusCreated, out)
}

// List returns every photo for a (subject_kind, subject_id) pair, scoped
// to the caller's provider. Soft-deleted rows are excluded. Each row
// carries a fresh 30-minute presigned GET so the UI can render thumbnails
// without a per-photo round-trip.
func (h *EvidencePhotoHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	q := r.URL.Query()
	subjectKind := strings.TrimSpace(q.Get("subjectKind"))
	subjectID := strings.TrimSpace(q.Get("subjectId"))
	if subjectKind == "" || subjectID == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("subjectKind and subjectId query params required"))
		return
	}
	if _, ok := validEvidenceSubjects[subjectKind]; !ok {
		httpx.RenderError(w, r, httpx.BadRequestf("invalid subjectKind"))
		return
	}

	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, provider_id, subject_kind, subject_id,
		       storage_bucket, storage_key, mime_type, caption,
		       COALESCE(uploaded_by_user_id, ''), created_at
		FROM evidence_photos
		WHERE provider_id = ? AND subject_kind = ? AND subject_id = ?
		  AND deleted_at IS NULL
		ORDER BY created_at ASC LIMIT 200`,
		pid, subjectKind, subjectID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()

	out := make([]EvidencePhoto, 0)
	for rows.Next() {
		var p EvidencePhoto
		var created dbtime.NullTime
		if err := rows.Scan(&p.ID, &p.ProviderID, &p.SubjectKind, &p.SubjectID,
			&p.StorageBucket, &p.StorageKey, &p.MIMEType, &p.Caption,
			&p.UploadedByUserID, &created); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		p.CreatedAt = created.ValueOrZero()
		if h.Storage != nil && h.Storage.IsConfigured() {
			if url, err := h.Storage.PresignGetURL(r.Context(), p.StorageBucket, p.StorageKey, 30*time.Minute); err == nil {
				p.DownloadURL = url
			}
		}
		out = append(out, p)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// Update is caption-only — the bytes are immutable, the metadata is not.
// Re-uploading a photo replaces the row entirely; for an inline edit of
// the operator's note, we just rewrite caption + return.
func (h *EvidencePhotoHandler) Update(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	var in struct {
		Caption string `json:"caption"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	res, err := h.Pool.ExecContext(r.Context(), `
		UPDATE evidence_photos SET caption = ?
		 WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		strings.TrimSpace(in.Caption), id, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"id": id, "caption": in.Caption})
}

// Delete is a soft delete (deleted_at = NOW). The S3 object stays — purge
// rolls up at retention time, same as documents.
func (h *EvidencePhotoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	res, err := h.Pool.ExecContext(r.Context(), `
		UPDATE evidence_photos SET deleted_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		id, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    mw.UserIDFrom(r.Context()),
		Action:     "evidence_photo.delete",
		TargetKind: "evidence_photo",
		TargetID:   id,
		IP:         auditlog.ClientIP(r),
		UserAgent:  r.UserAgent(),
	})
	w.WriteHeader(http.StatusNoContent)
}
