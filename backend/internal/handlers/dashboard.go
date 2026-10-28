package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"database/sql"

	"github.com/tachyurgy/compliancekit/backend/internal/compliance"
	"github.com/tachyurgy/compliancekit/backend/internal/dbtime"
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
	now := time.Now()
	report := compliance.EvaluateAt(state, facts, now)
	// Same score the /api/compliance/score hero shows, so the two never disagree.
	synth := compliance.SynthesizeScore(report, facts, now)

	alerts := make([]map[string]any, 0, len(report.Violations))
	critical, warning := 0, 0
	for _, v := range report.Violations {
		sev := alertSeverity(v.Severity)
		switch sev {
		case "critical":
			critical++
		case "warning":
			warning++
		}
		alerts = append(alerts, map[string]any{
			"id":          v.RuleID,
			"severity":    sev,
			"title":       v.Title,
			"description": v.Description,
			"href":        alertHref(v.RuleID, v.Category),
			"dueDate":     nil,
			"category":    alertCategory(v.RuleID, v.Category),
		})
	}
	timeline := make([]map[string]any, 0, len(report.UpcomingDeadlines90d))
	for _, d := range report.UpcomingDeadlines90d {
		timeline = append(timeline, map[string]any{
			"id":       d.RuleID + "|" + d.DueAt.Format("2006-01-02"),
			"date":     d.DueAt.Format(time.RFC3339),
			"label":    d.Title,
			"category": alertCategory(d.RuleID, d.Category),
			"severity": alertSeverity(d.Severity),
			"href":     alertHref(d.RuleID, d.Category),
		})
	}
	docCount := 0
	for _, ds := range facts.Documents {
		docCount += len(ds)
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		// Shape the SPA's DashboardSchema (frontend/src/api/dashboard.ts) parses.
		"complianceScore": synth.Score,
		"scoreDelta":      h.scoreDelta(r, pid, synth.Score),
		"updatedAt":       now.UTC().Format(time.RFC3339),
		"alerts":          alerts,
		"timeline":        timeline,
		// Raw engine output, kept for API clients and the integration tests.
		"score":              report.Score,
		"violations":         report.Violations,
		"upcoming_deadlines": report.UpcomingDeadlines90d,
		"rules_evaluated":    report.RulesEvaluated,
		"state":              state,
		"counts": map[string]int{
			"children":       len(facts.Children),
			"staff":          len(facts.Staff),
			"documents":      docCount,
			"criticalAlerts": critical,
			"warningAlerts":  warning,
		},
	})
}

// scoreDelta compares today's score with the newest daily snapshot that is at
// least a week old. No history yet means no movement to report.
func (h *DashboardHandler) scoreDelta(r *http.Request, pid string, score int) int {
	var prev int
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT score FROM compliance_score_snapshots
		WHERE provider_id = ? AND calculated_at <= datetime('now', '-7 days')
		ORDER BY calculated_at DESC LIMIT 1`, pid).Scan(&prev)
	if err != nil {
		return 0
	}
	return score - prev
}

// alertSeverity maps engine severities onto the UI's critical/warning/info.
func alertSeverity(s compliance.Severity) string {
	switch s {
	case compliance.SeverityCritical:
		return "critical"
	case compliance.SeverityMajor:
		return "warning"
	default:
		return "info"
	}
}

// alertCategory maps a rule onto the UI's alert category enum.
func alertCategory(ruleID, category string) string {
	switch {
	case strings.Contains(ruleID, "-IMM"):
		return "immunization"
	case strings.Contains(ruleID, "DRILLS"):
		return "drill"
	case strings.Contains(ruleID, "RATIOS"):
		return "ratio"
	case strings.Contains(ruleID, "POSTINGS"):
		return "posting"
	case strings.Contains(ruleID, "CPR"), strings.Contains(ruleID, "CREDENTIAL"),
		strings.Contains(ruleID, "TRAINING"), strings.Contains(ruleID, "LICENSE"):
		return "certification"
	}
	switch category {
	case "child_files", "staff_files", "facility":
		return "document"
	}
	return "other"
}

// alertHref points an alert at the page where it gets fixed.
func alertHref(ruleID, category string) any {
	switch {
	case strings.Contains(ruleID, "DRILLS"):
		return "/drill-roster"
	case strings.Contains(ruleID, "RATIOS"), strings.Contains(ruleID, "POSTINGS"):
		return "/operations"
	}
	switch category {
	case "child_files":
		return "/children"
	case "staff_files":
		return "/staff"
	case "facility":
		return "/documents"
	}
	return nil
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
		// TEXT date columns: scan via dbtime (see loadDocs) — modernc will
		// not auto-parse them into time.Time.
		var dob, enroll, created, updated dbtime.NullTime
		if err := rows.Scan(&c.ID, &c.ProviderID, &c.FirstName, &c.LastName, &dob, &enroll,
			&c.ParentEmail, &c.ParentPhone, &c.Classroom, &c.Status, &created, &updated); err != nil {
			return nil, err
		}
		c.DOB, c.EnrollDate = dob.ValueOrZero(), enroll.ValueOrZero()
		c.CreatedAt, c.UpdatedAt = created.ValueOrZero(), updated.ValueOrZero()
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
		var hire, bg, created, updated dbtime.NullTime
		if err := rows.Scan(&s.ID, &s.ProviderID, &s.FirstName, &s.LastName, &s.Role, &s.Email, &s.Phone,
			&hire, &bg, &s.Status, &created, &updated); err != nil {
			return nil, err
		}
		s.HireDate = hire.ValueOrZero()
		if bg.Valid {
			t := bg.Time
			s.BackgroundCheck = &t
		}
		s.CreatedAt, s.UpdatedAt = created.ValueOrZero(), updated.ValueOrZero()
		out = append(out, s)
	}
	return out, rows.Err()
}

func (h *DashboardHandler) loadDocs(r *http.Request, pid string) (map[string][]models.Document, error) {
	// NOTE: the `documents` table carries two sets of columns — the canonical
	// ones from migration 000004 (owner_kind/owner_id/doc_type/s3_key/
	// expiration_date) and a later handler-era alias set (subject_kind/
	// subject_id/kind/storage_*). We map both into the handler's expected
	// shape. When the alias columns are NULL we fall back to the canonical
	// values so compliance evaluation always sees a populated struct.
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
		       COALESCE(ocr_confidence, 0) AS ocr_confidence,
		       '' AS ocr_source,
		       '' AS uploaded_by,
		       COALESCE(uploaded_via, '') AS uploaded_via,
		       NULL AS last_chase_sent_at,
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
		// SQLite TEXT columns: scan timestamps into dbtime.NullTime to
		// tolerate the CURRENT_TIMESTAMP "YYYY-MM-DD HH:MM:SS" format the
		// modernc.org/sqlite driver refuses to auto-parse into time.Time.
		var issued, expires, lastChase, created, updated, deleted dbtime.NullTime
		if err := rows.Scan(&d.ID, &d.ProviderID, &d.SubjectKind, &d.SubjectID, &kind, &d.Title,
			&d.StorageBucket, &d.StorageKey, &d.MIMEType, &d.SizeBytes,
			&issued, &expires, &d.OCRConfidence, &d.OCRSource,
			&d.UploadedBy, &d.UploadedVia, &lastChase,
			&created, &updated, &deleted); err != nil {
			return nil, err
		}
		if issued.Valid {
			t := issued.Time
			d.IssuedAt = &t
		}
		if expires.Valid {
			t := expires.Time
			d.ExpiresAt = &t
		}
		if lastChase.Valid {
			t := lastChase.Time
			d.LastChaseSentAt = &t
		}
		d.CreatedAt = created.ValueOrZero()
		d.UpdatedAt = updated.ValueOrZero()
		if deleted.Valid {
			t := deleted.Time
			d.DeletedAt = &t
		}
		d.Kind = models.DocumentKind(kind)
		key := d.SubjectKind + "|" + d.SubjectID + "|" + kind
		out[key] = append(out[key], d)
	}
	return out, rows.Err()
}
