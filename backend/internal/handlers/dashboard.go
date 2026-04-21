package handlers

import (
	"log/slog"
	"net/http"

	"database/sql"

	"github.com/tachyurgy/compliancekit/backend/internal/compliance"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

type DashboardHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// GET /api/dashboard
// Assembles ProviderFacts from DB rows and evaluates the compliance engine.
func (h *DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	facts, state, err := h.loadFacts(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	report := compliance.Evaluate(state, facts)

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"score":              report.Score,
		"violations":         report.Violations,
		"upcoming_deadlines": report.UpcomingDeadlines90d,
		"rules_evaluated":    report.RulesEvaluated,
		"state":              state,
		"counts": map[string]int{
			"children": len(facts.Children),
			"staff":    len(facts.Staff),
		},
	})
}

func (h *DashboardHandler) loadFacts(r *http.Request, pid string) (*compliance.ProviderFacts, models.StateCode, error) {
	ctx := r.Context()
	var p models.Provider
	var createdStr, updatedStr string
	if err := h.Pool.QueryRowContext(ctx, `
		SELECT id, COALESCE(name, legal_name, '') AS name,
		       COALESCE(state_code, state, '') AS state_code,
		       COALESCE(license_number,''), COALESCE(owner_email,''), capacity, timezone,
		       created_at, updated_at
		FROM providers WHERE id = ?`, pid).Scan(
		&p.ID, &p.Name, &p.StateCode, &p.LicenseNumber, &p.OwnerEmail, &p.Capacity, &p.Timezone,
		&createdStr, &updatedStr,
	); err != nil {
		return nil, "", err
	}
	p.CreatedAt = parseSQLiteTime(createdStr)
	p.UpdatedAt = parseSQLiteTime(updatedStr)

	children, err := h.loadChildren(r, pid)
	if err != nil {
		return nil, "", err
	}
	staff, err := h.loadStaff(r, pid)
	if err != nil {
		return nil, "", err
	}
	docs, err := h.loadDocs(r, pid)
	if err != nil {
		return nil, "", err
	}

	// Drill count in last 90d — exclude soft-deleted rows.
	var drills int
	_ = h.Pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM drill_logs
		 WHERE provider_id = ? AND deleted_at IS NULL
		   AND drill_date > datetime('now', '-90 days')`, pid).
		Scan(&drills)

	// RatioOK + PostingsComplete — stored on providers as 0/1 ints (SQLite has
	// no native bool). COALESCE defaults match the column defaults in 000009.
	var ratioOK, postingsOK bool
	_ = h.Pool.QueryRowContext(ctx,
		`SELECT COALESCE(ratio_ok, 1), COALESCE(postings_complete, 0) FROM providers WHERE id = ?`, pid).
		Scan(&ratioOK, &postingsOK)

	facts := &compliance.ProviderFacts{
		Provider:         p,
		Children:         children,
		Staff:            staff,
		Documents:        docs,
		RatioOK:          ratioOK,
		PostingsComplete: postingsOK,
		DrillsLast90d:    drills,
	}
	return facts, p.StateCode, nil
}

func (h *DashboardHandler) loadChildren(r *http.Request, pid string) ([]models.Child, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, provider_id, first_name, last_name, date_of_birth, enroll_date,
		       COALESCE(parent_email,''), COALESCE(parent_phone,''), COALESCE(classroom,''),
		       status, created_at, updated_at
		FROM children WHERE provider_id = ?`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Child
	for rows.Next() {
		var c models.Child
		if err := rows.Scan(&c.ID, &c.ProviderID, &c.FirstName, &c.LastName, &c.DOB, &c.EnrollDate,
			&c.ParentEmail, &c.ParentPhone, &c.Classroom, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (h *DashboardHandler) loadStaff(r *http.Request, pid string) ([]models.Staff, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, provider_id, first_name, last_name, role, email, COALESCE(phone,''),
		       hire_date, background_check_date, status, created_at, updated_at
		FROM staff WHERE provider_id = ?`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Staff
	for rows.Next() {
		var s models.Staff
		if err := rows.Scan(&s.ID, &s.ProviderID, &s.FirstName, &s.LastName, &s.Role, &s.Email, &s.Phone,
			&s.HireDate, &s.BackgroundCheck, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (h *DashboardHandler) loadDocs(r *http.Request, pid string) (map[string][]models.Document, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, provider_id, subject_kind, COALESCE(subject_id,''), kind, title,
		       storage_bucket, storage_key, mime_type, size_bytes,
		       issued_at, expires_at, COALESCE(ocr_confidence,0), COALESCE(ocr_source,''),
		       COALESCE(uploaded_by,''), COALESCE(uploaded_via,''), last_chase_sent_at,
		       created_at, updated_at, deleted_at
		FROM documents WHERE provider_id = ? AND deleted_at IS NULL`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]models.Document)
	for rows.Next() {
		var d models.Document
		var kind string
		if err := rows.Scan(&d.ID, &d.ProviderID, &d.SubjectKind, &d.SubjectID, &kind, &d.Title,
			&d.StorageBucket, &d.StorageKey, &d.MIMEType, &d.SizeBytes,
			&d.IssuedAt, &d.ExpiresAt, &d.OCRConfidence, &d.OCRSource,
			&d.UploadedBy, &d.UploadedVia, &d.LastChaseSentAt,
			&d.CreatedAt, &d.UpdatedAt, &d.DeletedAt); err != nil {
			return nil, err
		}
		d.Kind = models.DocumentKind(kind)
		key := d.SubjectKind + "|" + d.SubjectID + "|" + kind
		out[key] = append(out[key], d)
	}
	return out, rows.Err()
}
