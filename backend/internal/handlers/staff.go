package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"database/sql"
	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/dbtime"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

type StaffHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// GET /api/staff
func (h *StaffHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, provider_id, first_name, last_name, role, COALESCE(email, ''), COALESCE(phone, ''),
		       hire_date, background_check_date, status, created_at, updated_at
		FROM staff WHERE provider_id = ? ORDER BY last_name, first_name`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	out := make([]models.Staff, 0)
	for rows.Next() {
		var s models.Staff
		// hire_date and background_check_date are TEXT columns; parse via helper.
		var hireStr, createdStr, updatedStr string
		var bgCheckStr sql.NullString
		if err := rows.Scan(&s.ID, &s.ProviderID, &s.FirstName, &s.LastName, &s.Role, &s.Email, &s.Phone,
			&hireStr, &bgCheckStr, &s.Status, &createdStr, &updatedStr); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		s.HireDate = parseSQLiteTime(hireStr)
		if bgCheckStr.Valid && bgCheckStr.String != "" {
			t := parseSQLiteTime(bgCheckStr.String)
			s.BackgroundCheck = &t
		}
		s.CreatedAt = parseSQLiteTime(createdStr)
		s.UpdatedAt = parseSQLiteTime(updatedStr)
		out = append(out, s)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// POST /api/staff
func (h *StaffHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	var in struct {
		models.Staff
		PreferredContactChannel string `json:"preferred_contact_channel"`
	}
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
	channel := in.PreferredContactChannel
	if channel == "" {
		channel = "email"
	}
	if channel != "email" && channel != "sms" && channel != "both" {
		httpx.RenderError(w, r, httpx.BadRequestf("preferred_contact_channel must be one of email, sms, both"))
		return
	}
	if (channel == "sms" || channel == "both") && in.Phone == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("phone is required when preferred_contact_channel is sms or both"))
		return
	}
	in.ID = base62.NewID()[:22]
	in.ProviderID = pid

	_, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO staff (id, provider_id, first_name, last_name, role, email, phone, hire_date,
		                  background_check_date, status, preferred_contact_channel,
		                  contact_channel_set_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		in.ID, in.ProviderID, in.FirstName, in.LastName, in.Role, in.Email, in.Phone, in.HireDate,
		in.BackgroundCheck, in.Status, channel)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitStaffCreate(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), in.ID, r)
	httpx.RenderJSON(w, http.StatusCreated, in)
}

// GET /api/staff/{id}
func (h *StaffHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	var s models.Staff
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT id, provider_id, first_name, last_name, role, COALESCE(email, ''), COALESCE(phone,''),
		       hire_date, background_check_date, status, created_at, updated_at
		FROM staff WHERE id = ? AND provider_id = ?`, id, pid).
		Scan(&s.ID, &s.ProviderID, &s.FirstName, &s.LastName, &s.Role, &s.Email, &s.Phone,
			dbtime.Into(&s.HireDate), dbtime.IntoPtr(&s.BackgroundCheck), &s.Status, dbtime.Into(&s.CreatedAt), dbtime.Into(&s.UpdatedAt))
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
		FirstName               *string    `json:"first_name"`
		LastName                *string    `json:"last_name"`
		Role                    *string    `json:"role"`
		Email                   *string    `json:"email"`
		Phone                   *string    `json:"phone"`
		BackgroundCheck         *time.Time `json:"background_check_date"`
		Status                  *string    `json:"status"`
		PreferredContactChannel *string    `json:"preferred_contact_channel"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.PreferredContactChannel != nil {
		c := *in.PreferredContactChannel
		if c != "email" && c != "sms" && c != "both" {
			httpx.RenderError(w, r, httpx.BadRequestf("preferred_contact_channel must be one of email, sms, both"))
			return
		}
	}
	_, err := h.Pool.ExecContext(r.Context(), `
		UPDATE staff SET
		  first_name                = COALESCE(?, first_name),
		  last_name                 = COALESCE(?, last_name),
		  role                      = COALESCE(?, role),
		  email                     = COALESCE(?, email),
		  phone                     = COALESCE(?, phone),
		  background_check_date     = COALESCE(?, background_check_date),
		  status                    = COALESCE(?, status),
		  preferred_contact_channel = COALESCE(?, preferred_contact_channel),
		  contact_channel_set_at    = CASE WHEN ? IS NOT NULL THEN CURRENT_TIMESTAMP ELSE contact_channel_set_at END,
		  updated_at                = CURRENT_TIMESTAMP
		WHERE id = ? AND provider_id = ?`,
		in.FirstName, in.LastName, in.Role, in.Email, in.Phone, in.BackgroundCheck, in.Status,
		in.PreferredContactChannel, in.PreferredContactChannel,
		id, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitStaffUpdate(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, r)
	h.Get(w, r)
}

// DELETE /api/staff/{id}  — soft delete via status='terminated'.
func (h *StaffHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE staff SET status = 'terminated', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND provider_id = ?`, id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitStaffDelete(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, r)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/staff/{id}/documents
func (h *StaffHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id, ''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
		       created_at, updated_at, deleted_at
		FROM documents
		WHERE provider_id = ? AND subject_kind = 'staff' AND subject_id = ? AND deleted_at IS NULL
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
			dbtime.IntoPtr(&d.IssuedAt), dbtime.IntoPtr(&d.ExpiresAt), &d.OCRConfidence, &d.OCRSource,
			&d.UploadedBy, &d.UploadedVia, dbtime.IntoPtr(&d.LastChaseSentAt),
			dbtime.Into(&d.CreatedAt), dbtime.Into(&d.UpdatedAt), dbtime.IntoPtr(&d.DeletedAt)); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		d.Kind = models.DocumentKind(kind)
		out = append(out, d)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}
