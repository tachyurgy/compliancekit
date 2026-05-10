package handlers

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

type ChildHandler struct {
	Pool         *sql.DB
	Magic        *magiclink.Service
	FrontendBase string
	Log          *slog.Logger
}

// scanChild centralises the read-shape so adding a column is a one-place edit.
// All Get/List queries pull the same set of columns in the same order.
const childSelectCols = `
  id, provider_id, first_name, last_name, date_of_birth,
  COALESCE(enroll_date, enrollment_date, '') AS enroll_date,
  COALESCE(parent_name, ''),
  COALESCE(parent_email, ''),
  COALESCE(parent_phone, ''),
  COALESCE(preferred_contact_channel, 'email'),
  COALESCE(classroom, ''),
  COALESCE(allergies, ''),
  COALESCE(medical_conditions, COALESCE(medical_notes, '')),
  COALESCE(status, 'enrolled') AS status,
  withdrawn_at, created_at, updated_at`

func scanChild(rows interface {
	Scan(dest ...any) error
}, c *models.Child) error {
	var dobStr, enrollStr, createdStr, updatedStr string
	var withdrawnStr sql.NullString
	if err := rows.Scan(&c.ID, &c.ProviderID, &c.FirstName, &c.LastName, &dobStr, &enrollStr,
		&c.ParentName, &c.ParentEmail, &c.ParentPhone, &c.PreferredContactChannel,
		&c.Classroom, &c.Allergies, &c.MedicalConditions, &c.Status,
		&withdrawnStr, &createdStr, &updatedStr); err != nil {
		return err
	}
	c.DOB = parseSQLiteTime(dobStr)
	c.EnrollDate = parseSQLiteTime(enrollStr)
	c.CreatedAt = parseSQLiteTime(createdStr)
	c.UpdatedAt = parseSQLiteTime(updatedStr)
	if withdrawnStr.Valid && withdrawnStr.String != "" {
		t := parseSQLiteTime(withdrawnStr.String)
		c.WithdrawnAt = &t
	}
	return nil
}

// GET /api/children
func (h *ChildHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT `+childSelectCols+`
		FROM children
		WHERE provider_id = ?
		ORDER BY last_name, first_name`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	out := make([]models.Child, 0)
	for rows.Next() {
		var c models.Child
		if err := scanChild(rows, &c); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, c)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// childCreateInput is the inbound JSON shape for POST /api/children. We accept
// both snake_case and the omitempty fields directly off models.Child.
type childCreateInput struct {
	FirstName               string    `json:"first_name"`
	LastName                string    `json:"last_name"`
	DOB                     time.Time `json:"date_of_birth"`
	EnrollDate              time.Time `json:"enroll_date"`
	ParentName              string    `json:"parent_name"`
	ParentEmail             string    `json:"parent_email"`
	ParentPhone             string    `json:"parent_phone"`
	PreferredContactChannel string    `json:"preferred_contact_channel"`
	Classroom               string    `json:"classroom"`
	Allergies               string    `json:"allergies"`
	MedicalConditions       string    `json:"medical_conditions"`
	Status                  string    `json:"status"`
}

// POST /api/children
func (h *ChildHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	var in childCreateInput
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
	channel := in.PreferredContactChannel
	if channel == "" {
		channel = "email"
	}
	if channel != "email" && channel != "sms" && channel != "both" {
		httpx.RenderError(w, r, httpx.BadRequestf("preferred_contact_channel must be one of email, sms, both"))
		return
	}
	if (channel == "sms" || channel == "both") && in.ParentPhone == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("parent_phone is required when preferred_contact_channel is sms or both"))
		return
	}
	id := base62.NewID()[:22]

	// Populate both the canonical 000002 columns (enrollment_date, guardians)
	// and the handler-era columns. See migration 000015 for the column reconcile.
	_, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth,
		                     enrollment_date, enroll_date, guardians,
		                     parent_name, parent_email, parent_phone,
		                     preferred_contact_channel, contact_channel_set_at,
		                     classroom, allergies, medical_conditions,
		                     status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,'[]',?,?,?,?,CURRENT_TIMESTAMP,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		id, pid, in.FirstName, in.LastName, in.DOB,
		in.EnrollDate, in.EnrollDate,
		in.ParentName, in.ParentEmail, in.ParentPhone,
		channel,
		in.Classroom, in.Allergies, in.MedicalConditions, in.Status)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitChildCreate(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, r)

	// Echo the stored row back. We rebuild from the input rather than re-read
	// because we want to return 201 Created with the canonical body shape;
	// h.Get would render 200.
	out := models.Child{
		ID:                      id,
		ProviderID:              pid,
		FirstName:               in.FirstName,
		LastName:                in.LastName,
		DOB:                     in.DOB,
		EnrollDate:              in.EnrollDate,
		ParentName:              in.ParentName,
		ParentEmail:             in.ParentEmail,
		ParentPhone:             in.ParentPhone,
		PreferredContactChannel: channel,
		Classroom:               in.Classroom,
		Allergies:               in.Allergies,
		MedicalConditions:       in.MedicalConditions,
		Status:                  in.Status,
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	httpx.RenderJSON(w, http.StatusCreated, out)
}

// GET /api/children/{id}
func (h *ChildHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	row := h.Pool.QueryRowContext(r.Context(), `
		SELECT `+childSelectCols+`
		FROM children WHERE id = ? AND provider_id = ?`, id, pid)
	var c models.Child
	if err := scanChild(row, &c); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.RenderError(w, r, httpx.ErrNotFound)
			return
		}
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
		FirstName               *string    `json:"first_name"`
		LastName                *string    `json:"last_name"`
		DOB                     *time.Time `json:"date_of_birth"`
		EnrollDate              *time.Time `json:"enroll_date"`
		ParentName              *string    `json:"parent_name"`
		ParentEmail             *string    `json:"parent_email"`
		ParentPhone             *string    `json:"parent_phone"`
		PreferredContactChannel *string    `json:"preferred_contact_channel"`
		Classroom               *string    `json:"classroom"`
		Allergies               *string    `json:"allergies"`
		MedicalConditions       *string    `json:"medical_conditions"`
		Status                  *string    `json:"status"`
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
		UPDATE children SET
		  first_name                = COALESCE(?, first_name),
		  last_name                 = COALESCE(?, last_name),
		  date_of_birth             = COALESCE(?, date_of_birth),
		  enroll_date               = COALESCE(?, enroll_date),
		  parent_name               = COALESCE(?, parent_name),
		  parent_email              = COALESCE(?, parent_email),
		  parent_phone              = COALESCE(?, parent_phone),
		  preferred_contact_channel = COALESCE(?, preferred_contact_channel),
		  contact_channel_set_at    = CASE WHEN ? IS NOT NULL THEN CURRENT_TIMESTAMP ELSE contact_channel_set_at END,
		  classroom                 = COALESCE(?, classroom),
		  allergies                 = COALESCE(?, allergies),
		  medical_conditions        = COALESCE(?, medical_conditions),
		  status                    = COALESCE(?, status),
		  updated_at                = CURRENT_TIMESTAMP
		WHERE id = ? AND provider_id = ?`,
		in.FirstName, in.LastName, in.DOB, in.EnrollDate,
		in.ParentName, in.ParentEmail, in.ParentPhone,
		in.PreferredContactChannel, in.PreferredContactChannel,
		in.Classroom, in.Allergies, in.MedicalConditions, in.Status,
		id, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitChildUpdate(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, r)
	h.Get(w, r)
}

// DELETE /api/children/{id}  — hard delete (parent-request scenario).
// Use Withdraw for the soft-delete enrollment-ended path.
func (h *ChildHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := h.Pool.ExecContext(r.Context(),
		`DELETE FROM children WHERE id = ? AND provider_id = ?`, id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitChildDelete(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, r)
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/children/{id}/withdraw — soft delete: the child has left the
// program. Records retention rules require we keep the row queryable for
// inspections (Handbook § 7.3 retention varies; default 12mo post-departure).
// withdrawn_at is the stamp; status flips to 'withdrawn'.
func (h *ChildHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := h.Pool.ExecContext(r.Context(), `
		UPDATE children
		   SET withdrawn_at = CURRENT_TIMESTAMP,
		       status = 'withdrawn',
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND provider_id = ? AND withdrawn_at IS NULL`,
		id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	// Revoke any outstanding parent-upload tokens — withdrawn child should
	// no longer accept uploads. Best-effort: log and continue if this fails.
	if h.Magic != nil {
		if err := h.Magic.Revoke(r.Context(), magiclink.KindParentUpload, id); err != nil {
			h.Log.Warn("withdraw: revoke tokens", "err", err, "child", id)
		}
	}
	auditlog.EmitChildUpdate(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, r)
	h.Get(w, r)
}

// GET /api/children/{id}/upload-link — convenience alias around the existing
// MintParentPortalLink endpoint, kept for symmetry with REQ-008's "operator
// can copy the link" surface. Returns { url, expires_at, token_kind }.
//
// We re-use the magic-link service directly (rather than calling the provider
// handler) so we don't need to thread the Emailer here. The provider-side
// MintParentPortalLink endpoint at POST /api/children/:id/portal-link is the
// canonical send-via-email path.
func (h *ChildHandler) UploadLink(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if h.Magic == nil {
		httpx.RenderError(w, r, httpx.ErrInternal)
		return
	}
	// Verify subject ownership.
	var firstName string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT first_name FROM children WHERE id = ? AND provider_id = ? AND withdrawn_at IS NULL`,
		id, pid).Scan(&firstName)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	_, path, err := h.Magic.Generate(r.Context(), magiclink.KindParentUpload, pid, id, 0)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	url := h.FrontendBase + path
	expiresAt := time.Now().Add(magiclink.TTLFor(magiclink.KindParentUpload)).UTC()
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"url":        url,
		"expires_at": expiresAt.Format(time.RFC3339),
		"token_kind": string(magiclink.KindParentUpload),
		"subject_id": id,
	})
}

// GET /api/children/{id}/documents
func (h *ChildHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id, ''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
		       COALESCE(expiration_extracted_value,''), COALESCE(document_type_extracted_value,''),
		       COALESCE(extraction_confidence,0), expiration_confirmed_at,
		       COALESCE(expiration_confirmed_by_user_id,''), COALESCE(does_not_expire,0),
		       created_at, updated_at, deleted_at
		FROM documents
		WHERE provider_id = ? AND subject_kind = 'child' AND subject_id = ? AND deleted_at IS NULL
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
		var dne int
		if err := rows.Scan(&d.ID, &d.ProviderID, &d.SubjectKind, &d.SubjectID, &kind, &d.Title,
			&d.StorageBucket, &d.StorageKey, &d.MIMEType, &d.SizeBytes,
			&d.IssuedAt, &d.ExpiresAt, &d.OCRConfidence, &d.OCRSource,
			&d.UploadedBy, &d.UploadedVia, &d.LastChaseSentAt,
			&d.ExpirationExtractedValue, &d.DocumentTypeExtractedValue,
			&d.ExtractionConfidence, &d.ExpirationConfirmedAt,
			&d.ExpirationConfirmedByUserID, &dne,
			&d.CreatedAt, &d.UpdatedAt, &d.DeletedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		d.Kind = models.DocumentKind(kind)
		d.DoesNotExpire = dne != 0
		out = append(out, d)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

