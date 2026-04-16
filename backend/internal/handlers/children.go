package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

type ChildHandler struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

// GET /api/children
func (h *ChildHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, provider_id, first_name, last_name, date_of_birth, enroll_date,
		       COALESCE(parent_email, ''), COALESCE(parent_phone, ''), COALESCE(classroom, ''),
		       status, created_at, updated_at
		FROM children WHERE provider_id = $1 ORDER BY last_name, first_name`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	out := make([]models.Child, 0)
	for rows.Next() {
		var c models.Child
		if err := rows.Scan(&c.ID, &c.ProviderID, &c.FirstName, &c.LastName, &c.DOB, &c.EnrollDate,
			&c.ParentEmail, &c.ParentPhone, &c.Classroom, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, c)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// POST /api/children
func (h *ChildHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	var in models.Child
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.FirstName == "" || in.LastName == "" || in.DOB.IsZero() {
		httpx.RenderError(w, r, httpx.BadRequestf("first_name, last_name, and date_of_birth are required"))
		return
	}
	if in.EnrollDate.IsZero() {
		in.EnrollDate = time.Now()
	}
	if in.Status == "" {
		in.Status = "enrolled"
	}
	in.ID = base62.NewID()[:22]
	in.ProviderID = pid

	_, err := h.Pool.Exec(r.Context(), `
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enroll_date,
		                     parent_email, parent_phone, classroom, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW())`,
		in.ID, in.ProviderID, in.FirstName, in.LastName, in.DOB, in.EnrollDate,
		in.ParentEmail, in.ParentPhone, in.Classroom, in.Status)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusCreated, in)
}

// GET /api/children/{id}
func (h *ChildHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	var c models.Child
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id, provider_id, first_name, last_name, date_of_birth, enroll_date,
		       COALESCE(parent_email,''), COALESCE(parent_phone,''), COALESCE(classroom,''),
		       status, created_at, updated_at
		FROM children WHERE id = $1 AND provider_id = $2`, id, pid).
		Scan(&c.ID, &c.ProviderID, &c.FirstName, &c.LastName, &c.DOB, &c.EnrollDate,
			&c.ParentEmail, &c.ParentPhone, &c.Classroom, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, c)
}

// PATCH /api/children/{id}
func (h *ChildHandler) Update(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	var in struct {
		FirstName   *string    `json:"first_name"`
		LastName    *string    `json:"last_name"`
		DOB         *time.Time `json:"date_of_birth"`
		ParentEmail *string    `json:"parent_email"`
		ParentPhone *string    `json:"parent_phone"`
		Classroom   *string    `json:"classroom"`
		Status      *string    `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	_, err := h.Pool.Exec(r.Context(), `
		UPDATE children SET
		  first_name    = COALESCE($3, first_name),
		  last_name     = COALESCE($4, last_name),
		  date_of_birth = COALESCE($5, date_of_birth),
		  parent_email  = COALESCE($6, parent_email),
		  parent_phone  = COALESCE($7, parent_phone),
		  classroom     = COALESCE($8, classroom),
		  status        = COALESCE($9, status),
		  updated_at    = NOW()
		WHERE id = $1 AND provider_id = $2`,
		id, pid, in.FirstName, in.LastName, in.DOB, in.ParentEmail, in.ParentPhone, in.Classroom, in.Status)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	h.Get(w, r)
}

// DELETE /api/children/{id}  — hard delete (children rarely soft-deleted; parent request).
func (h *ChildHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := h.Pool.Exec(r.Context(),
		`DELETE FROM children WHERE id = $1 AND provider_id = $2`, id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/children/{id}/documents
func (h *ChildHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id, ''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
		       created_at, updated_at, deleted_at
		FROM documents
		WHERE provider_id = $1 AND subject_kind = 'child' AND subject_id = $2 AND deleted_at IS NULL
		ORDER BY created_at DESC`, pid, id)
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
