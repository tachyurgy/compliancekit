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

type StaffHandler struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

// GET /api/staff
func (h *StaffHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, provider_id, first_name, last_name, role, email, COALESCE(phone, ''),
		       hire_date, background_check_date, status, created_at, updated_at
		FROM staff WHERE provider_id = $1 ORDER BY last_name, first_name`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	out := make([]models.Staff, 0)
	for rows.Next() {
		var s models.Staff
		if err := rows.Scan(&s.ID, &s.ProviderID, &s.FirstName, &s.LastName, &s.Role, &s.Email, &s.Phone,
			&s.HireDate, &s.BackgroundCheck, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, s)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// POST /api/staff
func (h *StaffHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	var in models.Staff
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.FirstName == "" || in.LastName == "" || in.Email == "" || in.Role == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("first_name, last_name, email, role required"))
		return
	}
	if in.HireDate.IsZero() {
		in.HireDate = time.Now()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	in.ID = base62.NewID()[:22]
	in.ProviderID = pid

	_, err := h.Pool.Exec(r.Context(), `
		INSERT INTO staff (id, provider_id, first_name, last_name, role, email, phone, hire_date,
		                  background_check_date, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW())`,
		in.ID, in.ProviderID, in.FirstName, in.LastName, in.Role, in.Email, in.Phone, in.HireDate,
		in.BackgroundCheck, in.Status)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusCreated, in)
}

// GET /api/staff/{id}
func (h *StaffHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	var s models.Staff
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id, provider_id, first_name, last_name, role, email, COALESCE(phone,''),
		       hire_date, background_check_date, status, created_at, updated_at
		FROM staff WHERE id = $1 AND provider_id = $2`, id, pid).
		Scan(&s.ID, &s.ProviderID, &s.FirstName, &s.LastName, &s.Role, &s.Email, &s.Phone,
			&s.HireDate, &s.BackgroundCheck, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, s)
}

// PATCH /api/staff/{id}
func (h *StaffHandler) Update(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	var in struct {
		FirstName       *string    `json:"first_name"`
		LastName        *string    `json:"last_name"`
		Role            *string    `json:"role"`
		Email           *string    `json:"email"`
		Phone           *string    `json:"phone"`
		BackgroundCheck *time.Time `json:"background_check_date"`
		Status          *string    `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	_, err := h.Pool.Exec(r.Context(), `
		UPDATE staff SET
		  first_name            = COALESCE($3, first_name),
		  last_name             = COALESCE($4, last_name),
		  role                  = COALESCE($5, role),
		  email                 = COALESCE($6, email),
		  phone                 = COALESCE($7, phone),
		  background_check_date = COALESCE($8, background_check_date),
		  status                = COALESCE($9, status),
		  updated_at            = NOW()
		WHERE id = $1 AND provider_id = $2`,
		id, pid, in.FirstName, in.LastName, in.Role, in.Email, in.Phone, in.BackgroundCheck, in.Status)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	h.Get(w, r)
}

// DELETE /api/staff/{id}  — soft delete via status='terminated'.
func (h *StaffHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := h.Pool.Exec(r.Context(),
		`UPDATE staff SET status = 'terminated', updated_at = NOW() WHERE id = $1 AND provider_id = $2`, id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/staff/{id}/documents
func (h *StaffHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id, ''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
		       created_at, updated_at, deleted_at
		FROM documents
		WHERE provider_id = $1 AND subject_kind = 'staff' AND subject_id = $2 AND deleted_at IS NULL
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
