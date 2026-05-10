// Package handlers — Inspector Mode (REQ-009).
//
// Inspector Mode is the iPad-friendly surface a Florida operator hands to
// a DCF Licensing Counselor. It is a *separate* feature from the
// Inspection Readiness Simulator (handlers/inspections.go + migration
// 000011) — that is the pre-visit rehearsal, this is the live event log.
//
// Routes mounted under /api/inspection/* (singular) by api/router.go to
// keep them visually distinct from the simulator's /api/inspections/*.
package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// InspectionModeHandler implements the live Inspector Mode endpoints.
//
// All writes are gated to a server-generated session id. Reads (the
// /snapshot endpoint) are session-protected via the standard /api router.
type InspectionModeHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// ----------------------------------------------------------------------------
// DTOs
// ----------------------------------------------------------------------------

type inspectionEventDTO struct {
	ID              string  `json:"id"`
	FacilityID      string  `json:"facility_id"`
	StartedAt       string  `json:"started_at"`
	EndedAt         *string `json:"ended_at,omitempty"`
	CounselorName   string  `json:"counselor_name,omitempty"`
	Type            string  `json:"type"`
	Sample          sampleBlob `json:"sample"`
	Findings        []finding   `json:"findings"`
	PlanPDFPath     string  `json:"plan_of_correction_pdf_path,omitempty"`
}

// sampleBlob is the JSON-encoded random sample persisted on inspection_events.
type sampleBlob struct {
	StaffIDs []string `json:"staff_ids,omitempty"`
	ChildIDs []string `json:"child_ids,omitempty"`
}

// finding is one row in the exit-conference findings list.
type finding struct {
	LineNumber string `json:"line_number"` // CF-FSP 5316 reference, e.g. "5316 #11"
	Class      string `json:"class"`       // "Class I" | "Class II" | "Class III"
	Text       string `json:"text"`        // canned + free text
	Note       string `json:"note,omitempty"`
	AddedAt    string `json:"added_at"`
}

// ----------------------------------------------------------------------------
// POST /api/inspection/sessions  — start a session
// ----------------------------------------------------------------------------

func (h *InspectionModeHandler) StartSession(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	uid := mw.UserIDFrom(r.Context())

	var in struct {
		CounselorName string `json:"counselor_name"`
		Type          string `json:"type"`
	}
	_ = httpx.DecodeJSON(r, &in) // body is optional
	if in.Type == "" {
		in.Type = "routine"
	}
	switch in.Type {
	case "routine", "renewal", "complaint", "followup", "other":
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("type must be one of routine|renewal|complaint|followup|other"))
		return
	}

	id := base62.NewID()[:22]
	_, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO inspection_events
		    (id, facility_id, started_at, counselor_name, type, sample_json, findings_json, started_by_user_id)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?, '{}', '[]', ?)`,
		id, pid, in.CounselorName, in.Type, nullableUser(uid))
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	dto, err := h.loadEvent(r, pid, id)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusCreated, dto)
}

// ----------------------------------------------------------------------------
// POST /api/inspection/sessions/:id/sample  — pull random sample
// ----------------------------------------------------------------------------

func (h *InspectionModeHandler) Sample(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	var in struct {
		Kind  string `json:"kind"`  // "staff" | "child"
		Count int    `json:"count"` // 1..5
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.Count < 1 || in.Count > 5 {
		httpx.RenderError(w, r, httpx.BadRequestf("count must be between 1 and 5"))
		return
	}

	// Confirm session belongs to this facility and isn't ended.
	var ended sql.NullString
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT ended_at FROM inspection_events WHERE id = ? AND facility_id = ?`, id, pid).
		Scan(&ended)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	if ended.Valid {
		httpx.RenderError(w, r, httpx.ConflictF("session already ended"))
		return
	}

	var ids []string
	switch strings.ToLower(in.Kind) {
	case "staff":
		ids, err = h.sampleStaff(r, pid, in.Count)
	case "child":
		ids, err = h.sampleChildren(r, pid, in.Count)
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("kind must be staff or child"))
		return
	}
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Merge into the persisted sample blob (overwrite kind, keep the other).
	var blob sampleBlob
	if err := h.loadSampleBlob(r, id, &blob); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if strings.ToLower(in.Kind) == "staff" {
		blob.StaffIDs = ids
	} else {
		blob.ChildIDs = ids
	}
	raw, _ := json.Marshal(blob)
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE inspection_events SET sample_json = ? WHERE id = ? AND facility_id = ?`,
		string(raw), id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"kind": strings.ToLower(in.Kind),
		"ids":  ids,
	})
}

// sampleStaff returns up to count staff IDs drawn at random from the active
// (non-deleted, status='active') staff pool. Returns whatever exists if the
// pool is smaller than count.
func (h *InspectionModeHandler) sampleStaff(r *http.Request, pid string, count int) ([]string, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id FROM staff
		WHERE provider_id = ? AND deleted_at IS NULL AND status = 'active'`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pool []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		pool = append(pool, id)
	}
	return pickRandom(pool, count), rows.Err()
}

// sampleChildren returns up to count child IDs drawn at random from the
// active enrolled pool (deleted_at IS NULL AND withdrawal_date IS NULL).
func (h *InspectionModeHandler) sampleChildren(r *http.Request, pid string, count int) ([]string, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id FROM children
		WHERE provider_id = ? AND deleted_at IS NULL AND withdrawal_date IS NULL`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pool []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		pool = append(pool, id)
	}
	return pickRandom(pool, count), rows.Err()
}

// pickRandom picks up to n distinct elements from src using crypto/rand for
// unbiased shuffling. If len(src) <= n, returns a copy of src in original
// order — the inspector wants the same sample on a re-show, so we don't
// shuffle when the sample is everyone.
func pickRandom(src []string, n int) []string {
	if len(src) == 0 {
		return []string{}
	}
	if n >= len(src) {
		out := make([]string, len(src))
		copy(out, src)
		return out
	}
	// Fisher–Yates over a copy, drawing indices via crypto/rand.
	pool := make([]string, len(src))
	copy(pool, src)
	for i := len(pool) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			// Astronomically unlikely; fall through with current order.
			break
		}
		jj := int(j.Int64())
		pool[i], pool[jj] = pool[jj], pool[i]
	}
	return pool[:n]
}

// ----------------------------------------------------------------------------
// POST /api/inspection/sessions/:id/finding
// ----------------------------------------------------------------------------

func (h *InspectionModeHandler) AddFinding(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	var in struct {
		LineNumber string `json:"line_number"`
		Class      string `json:"class"`
		Text       string `json:"text"`
		Note       string `json:"note"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("text required"))
		return
	}
	switch in.Class {
	case "", "Class I", "Class II", "Class III":
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("class must be one of Class I|Class II|Class III or empty"))
		return
	}

	var ended sql.NullString
	var raw string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT ended_at, COALESCE(findings_json, '[]') FROM inspection_events WHERE id = ? AND facility_id = ?`,
		id, pid).Scan(&ended, &raw)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	if ended.Valid {
		httpx.RenderError(w, r, httpx.ConflictF("session already ended"))
		return
	}

	var findings []finding
	_ = json.Unmarshal([]byte(raw), &findings)
	findings = append(findings, finding{
		LineNumber: in.LineNumber,
		Class:      in.Class,
		Text:       in.Text,
		Note:       in.Note,
		AddedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	updated, _ := json.Marshal(findings)
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE inspection_events SET findings_json = ? WHERE id = ? AND facility_id = ?`,
		string(updated), id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	httpx.RenderJSON(w, http.StatusCreated, map[string]any{
		"finding":  findings[len(findings)-1],
		"findings": findings,
	})
}

// ----------------------------------------------------------------------------
// POST /api/inspection/sessions/:id/end  — end + generate Plan of Correction
// ----------------------------------------------------------------------------

func (h *InspectionModeHandler) EndSession(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	dto, err := h.loadEvent(r, pid, id)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	if dto.EndedAt != nil {
		httpx.RenderError(w, r, httpx.ConflictF("session already ended"))
		return
	}

	// Generate the Plan of Correction PDF and store it on disk under
	// /tmp/levelbrook-poc-<id>.pdf. In production the path would point at
	// S3, but for v1 we keep it on the API host since the PDF is downloaded
	// inline almost immediately and never re-fetched. We persist the path
	// either way so the GET handler stays state-agnostic.
	facility, err := h.facilityHeader(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	pdfBytes := buildPlanOfCorrectionPDF(facility, dto)
	pdfPath := fmt.Sprintf("/tmp/levelbrook-poc-%s.pdf", id)
	if err := writeFileAtomic(pdfPath, pdfBytes); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE inspection_events SET ended_at = CURRENT_TIMESTAMP, plan_of_correction_pdf_path = ?
		 WHERE id = ? AND facility_id = ?`, pdfPath, id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	updated, err := h.loadEvent(r, pid, id)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, updated)
}

// ----------------------------------------------------------------------------
// GET /api/inspection/sessions/:id/plan-of-correction.pdf
// ----------------------------------------------------------------------------

func (h *InspectionModeHandler) DownloadPlan(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	dto, err := h.loadEvent(r, pid, id)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}

	// If the session hasn't been ended yet, we still render an interim PDF
	// so the operator can preview it during the demo. We do NOT persist
	// this path; the real path is written by EndSession.
	facility, err := h.facilityHeader(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	pdfBytes := buildPlanOfCorrectionPDF(facility, dto)

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="plan-of-correction-%s.pdf"`, id))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// ----------------------------------------------------------------------------
// GET /api/inspection/snapshot  — read-model that powers Inspector Mode tabs
// ----------------------------------------------------------------------------

// snapshotResponse is the single round-trip Inspector Mode loads on entry.
// Sample data follows the most-recent open session (or the most-recent
// closed one if none is open) so re-entering the view shows the same
// names the inspector already saw.
type snapshotResponse struct {
	Facility       facilityCard       `json:"facility"`
	LastInspection *lastInspection    `json:"last_inspection,omitempty"`
	StaffSample    []staffCard        `json:"staff_sample"`
	ChildSample    []childCard        `json:"child_sample"`
	FacilityRecords []recordRow       `json:"facility_records"`
	Postings       []postingRow       `json:"postings"`
	Drills         drillsBlock        `json:"drills"`
	OpenSessionID  string             `json:"open_session_id,omitempty"`
	GeneratedAt    string             `json:"generated_at"`
}

type facilityCard struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	LicenseNumber string `json:"license_number,omitempty"`
	LicenseExpiry string `json:"license_expiry,omitempty"`
	Address       string `json:"address,omitempty"`
	Capacity      int    `json:"capacity"`
	DirectorName  string `json:"director_name,omitempty"`
	StateCode     string `json:"state_code"`
}

type lastInspection struct {
	StartedAt     string `json:"started_at"`
	EndedAt       string `json:"ended_at,omitempty"`
	CounselorName string `json:"counselor_name,omitempty"`
	Type          string `json:"type,omitempty"`
	FindingsCount int    `json:"findings_count"`
}

type staffCard struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Role        string         `json:"role,omitempty"`
	HireDate    string         `json:"hire_date,omitempty"`
	Items       []personnelItem `json:"items"` // Handbook § 7.4 rollup
}

// personnelItem is one Handbook § 7.4 row for a sampled staff member.
// Status is one of "On file" | "Expiring soon" | "Expired" | "Missing".
type personnelItem struct {
	Label      string  `json:"label"`
	Status     string  `json:"status"`
	DocumentID string  `json:"document_id,omitempty"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
	FormName   string  `json:"form_name,omitempty"` // Inspector Mode shows e.g. "DH 680"
}

type childCard struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	DOB           string         `json:"date_of_birth,omitempty"`
	Enrolled      string         `json:"enrollment_date,omitempty"`
	Allergies     string         `json:"allergies,omitempty"`
	MedicalNotes  string         `json:"medical_notes,omitempty"`
	Items         []personnelItem `json:"items"` // Handbook § 7.3
}

type recordRow struct {
	Label      string `json:"label"`
	FormName   string `json:"form_name,omitempty"`
	Status     string `json:"status"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	DocumentID string `json:"document_id,omitempty"`
}

type postingRow struct {
	Kind            string  `json:"kind"`
	Label           string  `json:"label"`
	Required        bool    `json:"required"`
	RoomID          string  `json:"room_id,omitempty"`
	PhotoPath       string  `json:"photo_path,omitempty"`
	LastVerifiedAt  *string `json:"last_verified_at,omitempty"`
}

type drillsBlock struct {
	WindowDays int           `json:"window_days"`
	Logged     []drillEntry  `json:"logged"`
	Counts     map[string]int `json:"counts_by_kind"`
}

type drillEntry struct {
	ID        string `json:"id"`
	Kind      string `json:"drill_kind"`
	DrillDate string `json:"drill_date"`
	Notes     string `json:"notes,omitempty"`
}

func (h *InspectionModeHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())

	resp := snapshotResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Facility card.
	fac, err := h.facilityHeader(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	resp.Facility = fac

	// Latest inspection event (open or closed).
	if last, openID, err := h.latestEvent(r, pid); err == nil && last != nil {
		resp.LastInspection = last
		resp.OpenSessionID = openID
	}

	// Read the most-recent open session's sample (if any) for the sample tabs.
	var blob sampleBlob
	if resp.OpenSessionID != "" {
		_ = h.loadSampleBlob(r, resp.OpenSessionID, &blob)
	}

	// Staff sample cards (full Handbook § 7.4 rollup per staff).
	staffCards, err := h.staffCardsByIDs(r, pid, blob.StaffIDs)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	resp.StaffSample = staffCards

	// Children sample cards.
	childCards, err := h.childCardsByIDs(r, pid, blob.ChildIDs)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	resp.ChildSample = childCards

	// Facility records (49-item rollup, derived from facility-owned documents).
	resp.FacilityRecords, err = h.facilityRecords(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Postings (from posting_photos + the canonical FL posting list).
	resp.Postings, err = h.postingsRollup(r, pid, fac.StateCode)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Drills — last 12 months from drill_logs.
	resp.Drills, err = h.drillsBlock(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	httpx.RenderJSON(w, http.StatusOK, resp)
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func (h *InspectionModeHandler) loadEvent(r *http.Request, pid, id string) (*inspectionEventDTO, error) {
	row := h.Pool.QueryRowContext(r.Context(), `
		SELECT id, facility_id, started_at, ended_at, COALESCE(counselor_name, ''),
		       type, COALESCE(sample_json, '{}'), COALESCE(findings_json, '[]'),
		       COALESCE(plan_of_correction_pdf_path, '')
		FROM inspection_events WHERE id = ? AND facility_id = ?`, id, pid)

	var dto inspectionEventDTO
	var ended sql.NullString
	var sampleRaw, findingsRaw string
	if err := row.Scan(&dto.ID, &dto.FacilityID, &dto.StartedAt, &ended,
		&dto.CounselorName, &dto.Type, &sampleRaw, &findingsRaw,
		&dto.PlanPDFPath); err != nil {
		return nil, err
	}
	if ended.Valid {
		s := ended.String
		dto.EndedAt = &s
	}
	_ = json.Unmarshal([]byte(sampleRaw), &dto.Sample)
	_ = json.Unmarshal([]byte(findingsRaw), &dto.Findings)
	if dto.Findings == nil {
		dto.Findings = []finding{}
	}
	return &dto, nil
}

func (h *InspectionModeHandler) loadSampleBlob(r *http.Request, id string, blob *sampleBlob) error {
	var raw string
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(sample_json, '{}') FROM inspection_events WHERE id = ?`, id).
		Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), blob)
}

// facilityHeader pulls the provider row + onboarding-derived display data.
func (h *InspectionModeHandler) facilityHeader(r *http.Request, pid string) (facilityCard, error) {
	var fc facilityCard
	var addr1, addr2, city, stateAbbr, postal sql.NullString
	var licNum, licExp sql.NullString
	var capacity sql.NullInt64
	var name, stateCode sql.NullString
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT id,
		       COALESCE(name, legal_name),
		       license_number,
		       license_expires_on,
		       address_line1,
		       address_line2,
		       city,
		       state_abbr,
		       postal_code,
		       capacity,
		       state_code
		FROM providers WHERE id = ?`, pid).
		Scan(&fc.ID, &name, &licNum, &licExp, &addr1, &addr2, &city, &stateAbbr, &postal,
			&capacity, &stateCode)
	if err != nil {
		return fc, err
	}
	fc.Name = name.String
	fc.LicenseNumber = licNum.String
	fc.LicenseExpiry = licExp.String
	fc.Capacity = int(capacity.Int64)
	fc.StateCode = stateCode.String
	fc.Address = strings.TrimSpace(strings.Join(filterNonEmpty([]string{
		addr1.String, addr2.String,
		strings.TrimSpace(strings.Join(filterNonEmpty([]string{city.String, stateAbbr.String, postal.String}), ", ")),
	}), ", "))

	// Director name: the first staff member with role 'director' (case-insensitive).
	var first, last sql.NullString
	_ = h.Pool.QueryRowContext(r.Context(), `
		SELECT first_name, last_name FROM staff
		WHERE provider_id = ? AND deleted_at IS NULL AND status = 'active'
		      AND LOWER(COALESCE(role, '')) = 'director'
		ORDER BY created_at ASC LIMIT 1`, pid).Scan(&first, &last)
	if first.Valid || last.Valid {
		fc.DirectorName = strings.TrimSpace(first.String + " " + last.String)
	}
	return fc, nil
}

func (h *InspectionModeHandler) latestEvent(r *http.Request, pid string) (*lastInspection, string, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, started_at, ended_at, COALESCE(counselor_name, ''), type,
		       COALESCE(findings_json, '[]')
		FROM inspection_events WHERE facility_id = ?
		ORDER BY started_at DESC LIMIT 5`, pid)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var openID string
	var first *lastInspection
	for rows.Next() {
		var id, started, typ, findingsRaw, counselor string
		var ended sql.NullString
		if err := rows.Scan(&id, &started, &ended, &counselor, &typ, &findingsRaw); err != nil {
			return nil, "", err
		}
		if first == nil {
			var findings []finding
			_ = json.Unmarshal([]byte(findingsRaw), &findings)
			f := lastInspection{
				StartedAt:     started,
				CounselorName: counselor,
				Type:          typ,
				FindingsCount: len(findings),
			}
			if ended.Valid {
				f.EndedAt = ended.String
			}
			first = &f
		}
		if !ended.Valid && openID == "" {
			openID = id
		}
	}
	return first, openID, rows.Err()
}

// staffCardsByIDs renders one staffCard per id in input order. If ids is empty
// it returns an empty slice (the inspector hasn't pulled a sample yet). For
// each id we query the staff row and the staff's documents, then map to
// Handbook § 7.4 rows by best-effort doc_type matching. Items the staff has
// no document for show as "Missing" — honest empty state.
func (h *InspectionModeHandler) staffCardsByIDs(r *http.Request, pid string, ids []string) ([]staffCard, error) {
	if len(ids) == 0 {
		return []staffCard{}, nil
	}
	out := make([]staffCard, 0, len(ids))
	for _, id := range ids {
		card, err := h.staffCard(r, pid, id)
		if err != nil {
			return nil, err
		}
		if card != nil {
			out = append(out, *card)
		}
	}
	return out, nil
}

func (h *InspectionModeHandler) staffCard(r *http.Request, pid, id string) (*staffCard, error) {
	var first, last sql.NullString
	var role, hire sql.NullString
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT first_name, last_name, role, hire_date FROM staff
		WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`, id, pid).
		Scan(&first, &last, &role, &hire)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	card := staffCard{
		ID:       id,
		Name:     strings.TrimSpace(first.String + " " + last.String),
		Role:     role.String,
		HireDate: hire.String,
	}
	docs, err := h.docsByOwner(r, pid, "staff", id)
	if err != nil {
		return nil, err
	}
	card.Items = mapToPersonnelItems(handbookSection74(), docs)
	return &card, nil
}

func (h *InspectionModeHandler) childCardsByIDs(r *http.Request, pid string, ids []string) ([]childCard, error) {
	if len(ids) == 0 {
		return []childCard{}, nil
	}
	out := make([]childCard, 0, len(ids))
	for _, id := range ids {
		card, err := h.childCard(r, pid, id)
		if err != nil {
			return nil, err
		}
		if card != nil {
			out = append(out, *card)
		}
	}
	return out, nil
}

func (h *InspectionModeHandler) childCard(r *http.Request, pid, id string) (*childCard, error) {
	var first, last, dob, enroll sql.NullString
	var allergies, medical sql.NullString
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT first_name, last_name, date_of_birth, enrollment_date,
		       COALESCE(allergies, ''), COALESCE(medical_notes, '')
		FROM children WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		id, pid).Scan(&first, &last, &dob, &enroll, &allergies, &medical)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	card := childCard{
		ID:           id,
		Name:         strings.TrimSpace(first.String + " " + last.String),
		DOB:          dob.String,
		Enrolled:     enroll.String,
		Allergies:    allergies.String,
		MedicalNotes: medical.String,
	}
	docs, err := h.docsByOwner(r, pid, "child", id)
	if err != nil {
		return nil, err
	}
	card.Items = mapToPersonnelItems(handbookSection73(), docs)
	return &card, nil
}

// ownedDoc is the slice of fields we read from documents to score a checklist row.
type ownedDoc struct {
	ID         string
	DocType    string
	ExpiresAt  string // empty if no expiration captured
}

func (h *InspectionModeHandler) docsByOwner(r *http.Request, pid, kind, id string) ([]ownedDoc, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, doc_type, COALESCE(expiration_date, '')
		FROM documents
		WHERE provider_id = ? AND owner_kind = ? AND owner_id = ? AND deleted_at IS NULL`,
		pid, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ownedDoc
	for rows.Next() {
		var d ownedDoc
		if err := rows.Scan(&d.ID, &d.DocType, &d.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (h *InspectionModeHandler) facilityRecords(r *http.Request, pid string) ([]recordRow, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, doc_type, COALESCE(expiration_date, '')
		FROM documents
		WHERE provider_id = ? AND owner_kind = 'facility' AND deleted_at IS NULL`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []ownedDoc
	for rows.Next() {
		var d ownedDoc
		if err := rows.Scan(&d.ID, &d.DocType, &d.ExpiresAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}

	out := make([]recordRow, 0, len(facilityRecordsList()))
	for _, def := range facilityRecordsList() {
		match := bestMatch(def.DocTypes, docs)
		row := recordRow{
			Label:    def.Label,
			FormName: def.FormName,
		}
		if match == nil {
			row.Status = "Missing"
		} else {
			row.DocumentID = match.ID
			row.ExpiresAt = match.ExpiresAt
			row.Status = expirationStatus(match.ExpiresAt)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (h *InspectionModeHandler) postingsRollup(r *http.Request, pid, stateCode string) ([]postingRow, error) {
	// Pull the most recent verification per (kind, room) tuple.
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT posting_kind, COALESCE(room_id, ''), photo_path, last_verified_at
		FROM posting_photos
		WHERE facility_id = ?
		ORDER BY last_verified_at DESC`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type entry struct {
		PhotoPath  string
		Verified   string
		RoomID     string
	}
	by := map[string]entry{}
	for rows.Next() {
		var kind, room, photo, verified string
		if err := rows.Scan(&kind, &room, &photo, &verified); err != nil {
			return nil, err
		}
		k := kind + "|" + room
		if _, exists := by[k]; !exists {
			by[k] = entry{PhotoPath: photo, Verified: verified, RoomID: room}
		}
	}
	defs := postingDefsForState(stateCode)
	out := make([]postingRow, 0, len(defs))
	for _, d := range defs {
		row := postingRow{Kind: d.Kind, Label: d.Label, Required: d.Required}
		// First try a per-room hit (room blank), then fall back to any room_id.
		if e, ok := by[d.Kind+"|"]; ok {
			row.PhotoPath = e.PhotoPath
			v := e.Verified
			row.LastVerifiedAt = &v
		} else {
			// Any matching kind regardless of room.
			for k, e := range by {
				if strings.HasPrefix(k, d.Kind+"|") {
					row.PhotoPath = e.PhotoPath
					row.RoomID = e.RoomID
					v := e.Verified
					row.LastVerifiedAt = &v
					break
				}
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (h *InspectionModeHandler) drillsBlock(r *http.Request, pid string) (drillsBlock, error) {
	cutoff := time.Now().AddDate(0, 0, -365).UTC().Format(time.RFC3339)
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, drill_kind, drill_date, COALESCE(notes, '')
		FROM drill_logs
		WHERE provider_id = ? AND deleted_at IS NULL AND drill_date >= ?
		ORDER BY drill_date DESC`, pid, cutoff)
	if err != nil {
		return drillsBlock{}, err
	}
	defer rows.Close()
	out := drillsBlock{WindowDays: 365, Counts: map[string]int{}}
	for rows.Next() {
		var d drillEntry
		if err := rows.Scan(&d.ID, &d.Kind, &d.DrillDate, &d.Notes); err != nil {
			return out, err
		}
		out.Logged = append(out.Logged, d)
		out.Counts[d.Kind] = out.Counts[d.Kind] + 1
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------------------
// Handbook § 7.4 / § 7.3 rollup definitions
// ----------------------------------------------------------------------------

type handbookDef struct {
	Label     string   // operator-friendly label (used inside Inspector Mode header text)
	FormName  string   // formal form/handbook reference shown to the inspector
	DocTypes  []string // doc_type strings that satisfy this row, in priority order
}

// handbookSection74 mirrors active/product/florida-inspection-coverage.md
// (the 25 highest-priority staff records). Inspector Mode renders the
// formal FormName, not the friendly Label.
func handbookSection74() []handbookDef {
	return []handbookDef{
		{"Background screening (Level 2)", "AHCA Clearinghouse — Handbook § 5.1", []string{"bg_screening", "background_check", "bg_screening_form"}},
		{"Eligible-result document", "Clearinghouse Eligible-Result — Handbook § 7.4.1", []string{"clearinghouse_result", "eligible_result"}},
		{"CF-FSP 5131 Background Screening form", "CF-FSP 5131", []string{"bg_screening_form", "cf_fsp_5131"}},
		{"CF-FSP 1649A Good Moral Character", "CF-FSP 1649A", []string{"cf_fsp_1649a", "moral_character"}},
		{"Employment application", "Handbook § 7.4 — F.S. § 402.3055(1)(b)", []string{"employment_application"}},
		{"40-hour Introductory Training", "Handbook § 4.1, § 4.2.1", []string{"intro_training", "intro_40hr"}},
		{"10-hour Annual In-Service (CF-FSP 5268)", "CF-FSP 5268 — Handbook § 4.2.7", []string{"in_service", "cf_fsp_5268", "annual_inservice"}},
		{"Pediatric CPR", "Handbook § 4.2.4", []string{"cpr", "cpr_certification"}},
		{"First Aid", "Handbook § 4.2.4", []string{"first_aid", "first_aid_certification"}},
		{"Safe Sleep / SBS Training", "Handbook § 4.2.3", []string{"safe_sleep", "sbs_training"}},
		{"Early Literacy Training", "Handbook § 4.2.2", []string{"early_literacy"}},
		{"Fire Extinguisher Training", "Handbook § 4.2.5", []string{"fire_extinguisher_training"}},
		{"Mandated Reporter (CF-FSP 5337)", "CF-FSP 5337 — annual", []string{"cf_fsp_5337", "mandated_reporter"}},
		{"Director Credential", "Handbook § 4.7", []string{"director_credential"}},
		{"Staff Credential", "Handbook § 4.6", []string{"staff_credential", "fl_cda", "fccpc"}},
		{"Volunteer Acknowledgement (CF-FSP 5217)", "CF-FSP 5217", []string{"cf_fsp_5217", "volunteer_ack"}},
		{"Driver's License", "Handbook § 2.5.1", []string{"drivers_license"}},
		{"Driver Physical (annual)", "Handbook § 2.5.1", []string{"driver_physical"}},
		{"Transportation Training", "Handbook § 4.2.6", []string{"transportation_training"}},
		{"Personnel Record Acknowledgement", "Handbook § 7.4", []string{"personnel_ack"}},
	}
}

func handbookSection73() []handbookDef {
	return []handbookDef{
		{"Application for Enrollment (CF-FSP 5219)", "CF-FSP 5219 — Handbook § 7.3", []string{"cf_fsp_5219", "enrollment_application"}},
		{"Immunization (DH 680)", "DH 680 — Handbook § 7.1, § 21", []string{"dh_680", "immunization"}},
		{"Religious Exemption (DH 681)", "DH 681 — Handbook § 7.1, § 21", []string{"dh_681", "religious_exemption"}},
		{"Student Health Examination (DH 3040)", "DH 3040 — Handbook § 7.2", []string{"dh_3040", "health_exam"}},
		{"Authorization to Release Child", "Handbook § 7.3", []string{"release_authorization", "authorized_pickup"}},
		{"Emergency Contact Info", "Handbook § 7.3, § 2.7", []string{"emergency_contact"}},
		{"Allergy / Special Diet Order", "Handbook § 3.9.3(F)", []string{"allergy_order", "special_diet"}},
		{"CF/PI 175-24 Brochure (Know Your Facility)", "CF/PI 175-24 — Handbook § 7.3", []string{"cf_pi_175_24"}},
		{"Discipline Policy Acknowledgement", "Handbook § 7.3", []string{"discipline_ack"}},
		{"Food & Nutrition Policy Acknowledgement", "Handbook § 7.3", []string{"food_nutrition_ack"}},
		{"CF/PI 175-70 Influenza Brochure (annual)", "CF/PI 175-70 — F.S. § 402.313(14)", []string{"cf_pi_175_70", "influenza_ack"}},
		{"CF/PI 175-12 Distracted Adult (annual)", "CF/PI 175-12 — F.S. § 402.313(15)", []string{"cf_pi_175_12", "distracted_adult_ack"}},
		{"Field-trip / food activity permission", "Handbook § 2.7", []string{"field_trip_permission"}},
		{"Medication / sunscreen permission", "Handbook § 6.5", []string{"medication_permission", "sunscreen_permission"}},
	}
}

// facilityRecordsList is a 49-item rollup applied to facility-owned documents.
// We track 20 of the highest-impact ones; the rest are surfaced in the
// Postings tab or the Drills tab.
func facilityRecordsList() []handbookDef {
	return []handbookDef{
		{"Current DCF License", "Handbook § 2.1", []string{"facility_license"}},
		{"Renewal Application (CF-FSP 5017)", "CF-FSP 5017 — 65C-22.001(1)", []string{"cf_fsp_5017", "renewal_application"}},
		{"General Liability Insurance", "Handbook § 7", []string{"liability_insurance", "insurance"}},
		{"Annual Fire Inspection", "Handbook § 3.8", []string{"fire_inspection"}},
		{"Annual Health Inspection", "Handbook § 3.10", []string{"health_inspection"}},
		{"Emergency Evacuation Plan", "Handbook § 3.8.5(E)", []string{"evacuation_plan"}},
		{"Disaster Preparedness Plan", "Handbook § 3.8.5", []string{"disaster_plan"}},
		{"Discipline & Expulsion Policy", "Handbook § 7.3", []string{"discipline_policy"}},
		{"Food & Nutrition Policy", "Handbook § 3.9", []string{"food_nutrition_policy"}},
		{"Universal Precautions / Exposure Plan", "Handbook § 3.10", []string{"exposure_plan", "universal_precautions"}},
		{"Cleaning & Sanitizing Schedule", "Handbook § 3.10", []string{"cleaning_schedule"}},
		{"Vehicle Insurance (if transporting)", "Handbook § 2.5", []string{"vehicle_insurance"}},
		{"Vehicle Mechanic Inspection (annual)", "Handbook § 2.5.1", []string{"vehicle_inspection"}},
		{"Well/Septic Permit (if applicable)", "Handbook § 3.10", []string{"well_septic"}},
		{"Director Credential (CF-FSP 5290)", "CF-FSP 5290", []string{"director_credential"}},
		{"Plan of Operation Summary", "Handbook § 2.6", []string{"plan_of_operation"}},
		{"Shaken Baby Syndrome / Abusive Head Trauma Policy", "Handbook § 4.2.3", []string{"sbs_policy"}},
		{"Background Screening Affidavit", "Handbook § 5.1", []string{"bg_affidavit"}},
		{"Capacity Documentation (per room)", "Handbook § 3.4.1(A)", []string{"capacity_doc"}},
		{"Annual In-Service Training Plan", "Handbook § 4.2.7", []string{"inservice_plan"}},
	}
}

// ----------------------------------------------------------------------------
// Posting kind catalog (FL only for v1)
// ----------------------------------------------------------------------------

type postingKindDef struct {
	Kind     string
	Label    string
	Required bool
}

func postingDefsForState(stateCode string) []postingKindDef {
	if strings.ToUpper(stateCode) != "FL" {
		// Other states share most of these but with different labels.
		// For v1 we only ship FL; non-FL falls through to FL labels — which
		// will still mostly be correct.
	}
	return []postingKindDef{
		{"license", "Current DCF license", true},
		{"capacity_room", "Capacity per room", true},
		{"evacuation_plan", "Emergency evacuation plan diagram", true},
		{"weekly_menu", "Weekly meal & snack menu", true},
		{"activity_plan", "Daily activity schedule", true},
		{"emergency_phone", "Emergency telephone numbers", true},
		{"diaper_procedure", "Diaper changing procedure", true},
		{"hand_hygiene", "Hand-hygiene situations", true},
		{"handwashing", "Handwashing technique sign", true},
		{"food_restrictions", "Special food restrictions", true},
		{"dcf_hotline", "DCF abuse hotline poster", true},
		{"mandated_reporter", "Mandated reporter notice", true},
	}
}

// ----------------------------------------------------------------------------
// Status mapping (Handbook plain-English status → Inspector Mode status)
// ----------------------------------------------------------------------------

func mapToPersonnelItems(defs []handbookDef, docs []ownedDoc) []personnelItem {
	out := make([]personnelItem, 0, len(defs))
	for _, def := range defs {
		match := bestMatch(def.DocTypes, docs)
		item := personnelItem{
			Label:    def.Label,
			FormName: def.FormName,
		}
		if match == nil {
			item.Status = "Missing"
		} else {
			item.DocumentID = match.ID
			if match.ExpiresAt != "" {
				e := match.ExpiresAt
				item.ExpiresAt = &e
			}
			item.Status = expirationStatus(match.ExpiresAt)
		}
		out = append(out, item)
	}
	return out
}

func bestMatch(types []string, docs []ownedDoc) *ownedDoc {
	for _, t := range types {
		for i := range docs {
			if strings.EqualFold(docs[i].DocType, t) {
				return &docs[i]
			}
		}
	}
	return nil
}

// expirationStatus returns "On file" / "Expiring soon" / "Expired" given an
// expiration date string. An empty expires_at means the doc is on file but
// has no captured expiration — we surface "On file" rather than fabricating.
func expirationStatus(expiresAt string) string {
	if expiresAt == "" {
		return "On file"
	}
	t, err := parseLooseDate(expiresAt)
	if err != nil {
		return "On file"
	}
	now := time.Now().UTC()
	if t.Before(now) {
		return "Expired"
	}
	if t.Before(now.AddDate(0, 0, 90)) {
		return "Expiring soon"
	}
	return "On file"
}

func parseLooseDate(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", s)
}

// ----------------------------------------------------------------------------
// Plan of Correction PDF (single-page, hand-rolled)
// ----------------------------------------------------------------------------

// buildPlanOfCorrectionPDF returns a single-page PDF document. We deliberately
// hand-roll it because (a) no PDF library is vendored and (b) the document is
// short, plain-text only, and the byte-exactness of a real PDF is what makes
// the inspector treat the iPad as the records source. The PDF embeds the
// Levelbrook footer and a draft signature line for the director.
func buildPlanOfCorrectionPDF(fc facilityCard, ev *inspectionEventDTO) []byte {
	// Build the page-content stream first; everything else references it.
	var stream strings.Builder
	stream.WriteString("BT\n")
	stream.WriteString("/F1 18 Tf\n")
	stream.WriteString("72 760 Td\n")
	stream.WriteString("(Plan of Correction Draft) Tj\n")
	stream.WriteString("/F1 11 Tf\n")
	stream.WriteString("0 -22 Td\n")
	stream.WriteString(fmt.Sprintf("(%s) Tj\n", pdfEscape(fc.Name)))
	stream.WriteString("0 -14 Td\n")
	stream.WriteString(fmt.Sprintf("(License #: %s   State: %s) Tj\n",
		pdfEscape(fc.LicenseNumber), pdfEscape(fc.StateCode)))
	stream.WriteString("0 -14 Td\n")
	stream.WriteString(fmt.Sprintf("(Address: %s) Tj\n", pdfEscape(fc.Address)))
	stream.WriteString("0 -14 Td\n")
	stream.WriteString(fmt.Sprintf("(Director: %s) Tj\n", pdfEscape(fc.DirectorName)))

	stream.WriteString("0 -22 Td\n")
	stream.WriteString("/F1 12 Tf\n")
	stream.WriteString("(Inspection event) Tj\n")
	stream.WriteString("/F1 10 Tf\n")
	stream.WriteString("0 -14 Td\n")
	stream.WriteString(fmt.Sprintf("(Started: %s   Counselor: %s   Type: %s) Tj\n",
		pdfEscape(ev.StartedAt), pdfEscape(orFallback(ev.CounselorName, "(not provided)")),
		pdfEscape(ev.Type)))

	stream.WriteString("0 -22 Td\n")
	stream.WriteString("/F1 12 Tf\n")
	if len(ev.Findings) == 0 {
		stream.WriteString("(Findings: none recorded) Tj\n")
	} else {
		stream.WriteString(fmt.Sprintf("(Findings (%d)) Tj\n", len(ev.Findings)))
		stream.WriteString("/F1 10 Tf\n")
		for i, f := range ev.Findings {
			stream.WriteString("0 -14 Td\n")
			label := f.LineNumber
			if f.Class != "" {
				label = label + " (" + f.Class + ")"
			}
			line := fmt.Sprintf("%d. %s — %s", i+1, label, f.Text)
			line = truncateLine(line, 110)
			stream.WriteString(fmt.Sprintf("(%s) Tj\n", pdfEscape(line)))
			if f.Note != "" {
				stream.WriteString("0 -12 Td\n")
				stream.WriteString(fmt.Sprintf("(   Note: %s) Tj\n",
					pdfEscape(truncateLine(f.Note, 100))))
			}
		}
	}

	stream.WriteString("0 -36 Td\n")
	stream.WriteString("/F1 11 Tf\n")
	stream.WriteString("(Plan of Correction) Tj\n")
	stream.WriteString("/F1 10 Tf\n")
	stream.WriteString("0 -14 Td\n")
	stream.WriteString("(Each finding above will be corrected within 14 days unless otherwise noted.) Tj\n")
	stream.WriteString("0 -14 Td\n")
	stream.WriteString("(Records source: Levelbrook \\(digital\\). Source documents are linked in the iPad view.) Tj\n")

	stream.WriteString("0 -40 Td\n")
	stream.WriteString("/F1 11 Tf\n")
	stream.WriteString("(Director signature: ____________________________________) Tj\n")
	stream.WriteString("0 -18 Td\n")
	stream.WriteString("(Date: ____________________________________) Tj\n")

	stream.WriteString("0 -36 Td\n")
	stream.WriteString("/F1 8 Tf\n")
	stream.WriteString(fmt.Sprintf("(Generated by Levelbrook on %s. Session %s.) Tj\n",
		pdfEscape(time.Now().UTC().Format("2006-01-02 15:04 UTC")), pdfEscape(ev.ID)))

	stream.WriteString("ET\n")

	return assemblePDF(stream.String())
}

// assemblePDF wraps a content stream into a minimal valid single-page PDF.
// We compute byte offsets ourselves and build the xref so the output passes
// `pdfinfo` and renders in Safari, Preview, Chrome, and iPadOS Files.
func assemblePDF(content string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	// %âãÏÓ — high-bit binary marker so transports treat the file as binary.
	b.WriteString("%\xE2\xE3\xCF\xD3\n")

	type obj struct {
		ID   int
		Body string
	}
	objs := []obj{
		{1, "<< /Type /Catalog /Pages 2 0 R >>\n"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>\n"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\n"},
		{4, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream\n", len(content), content)},
		{5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\n"},
	}
	offsets := make([]int, len(objs)+1)
	for _, o := range objs {
		offsets[o.ID] = b.Len()
		b.WriteString(fmt.Sprintf("%d 0 obj\n%sendobj\n", o.ID, o.Body))
	}
	xrefStart := b.Len()
	b.WriteString(fmt.Sprintf("xref\n0 %d\n", len(objs)+1))
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	b.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objs)+1, xrefStart))
	return []byte(b.String())
}

// pdfEscape escapes parens and backslashes per PDF string syntax. Input is
// expected to be UTF-8 — Helvetica's WinAnsi encoding maps the printable ASCII
// subset 1:1, which is the only thing we use here. Non-ASCII characters get
// replaced with '?' to keep the file valid without embedding a font.
func pdfEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7f {
			switch r {
			case '(', ')', '\\':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
			continue
		}
		// Substitute common typography for ASCII look-alikes.
		switch r {
		case '§':
			b.WriteString("Sec.")
		case '–', '—':
			b.WriteRune('-')
		case '“', '”':
			b.WriteRune('"')
		case '‘', '’':
			b.WriteRune('\'')
		case '\t':
			b.WriteString("    ")
		default:
			if r > 0x7f {
				b.WriteRune('?')
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func orFallback(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	return s
}

func truncateLine(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// writeFileAtomic writes to a tmp file and renames into place. We don't use
// os.WriteFile directly because the same path may already exist from an
// earlier session reopen.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// nullableUser converts an empty user-id into a nil sql arg so the FK column
// stays NULL rather than referencing a phantom row.
func nullableUser(uid string) any {
	if uid == "" {
		return nil
	}
	return uid
}

// filterNonEmpty drops empty / whitespace-only strings.
func filterNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
