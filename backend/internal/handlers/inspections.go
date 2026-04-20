package handlers

import (
	"bytes"
	"database/sql"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/inspection"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// InspectionHandler implements the Inspection Readiness Simulator endpoints.
// Runs are per-provider. Checklists live in internal/inspection (data-only)
// and are joined to stored responses by the immutable item.ID string so
// regulatory revisions never orphan historical runs.
type InspectionHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// --- DTOs returned to the frontend ---

type inspectionRunDTO struct {
	ID           string     `json:"id"`
	ProviderID   string     `json:"provider_id"`
	State        string     `json:"state"`
	FormRef      string     `json:"form_ref"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Score        *int       `json:"score,omitempty"`
	TotalItems   int        `json:"total_items"`
	ItemsPassed  int        `json:"items_passed"`
	ItemsFailed  int        `json:"items_failed"`
	ItemsNA      int        `json:"items_na"`
	ItemsAnswered int       `json:"items_answered"`
}

type responseDTO struct {
	ItemID             string    `json:"item_id"`
	Answer             string    `json:"answer"` // pass | fail | na
	Note               string    `json:"note,omitempty"`
	EvidenceDocumentID string    `json:"evidence_document_id,omitempty"`
	AnsweredAt         time.Time `json:"answered_at"`
}

type domainBreakdown struct {
	Name        string `json:"name"`
	Total       int    `json:"total"`
	Passed      int    `json:"passed"`
	Failed      int    `json:"failed"`
	NA          int    `json:"na"`
	Unanswered  int    `json:"unanswered"`
}

type citationRisk struct {
	ItemID    string              `json:"item_id"`
	Domain    string              `json:"domain"`
	Question  string              `json:"question"`
	Reference string              `json:"reference"`
	FormRef   string              `json:"form_ref"`
	Severity  inspection.Severity `json:"severity"`
	Note      string              `json:"note,omitempty"`
}

type runDetailDTO struct {
	Run        inspectionRunDTO      `json:"run"`
	Checklist  inspection.Checklist  `json:"checklist"`
	Responses  []responseDTO         `json:"responses"`
	Domains    []domainBreakdown     `json:"domain_breakdown,omitempty"`
	Citations  []citationRisk        `json:"predicted_citations,omitempty"`
}

// --- POST /api/inspections ---
// Starts a new run seeded with total_items for the provider's state. Returns
// the run and the full checklist so the frontend can render the wizard without
// a second round-trip.
func (h *InspectionHandler) Start(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	state, err := h.providerState(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	checklist := inspection.ChecklistFor(state)
	if len(checklist.Items) == 0 {
		httpx.RenderError(w, r, httpx.BadRequestf("no inspection checklist defined for state %s", state))
		return
	}

	runID := base62.NewID()[:22]
	_, err = h.Pool.ExecContext(r.Context(), `
		INSERT INTO inspection_runs (id, provider_id, state, total_items, started_at, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		runID, pid, string(state), len(checklist.Items))
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	run, err := h.loadRun(r, pid, runID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusCreated, runDetailDTO{
		Run:       run,
		Checklist: checklist,
		Responses: []responseDTO{},
	})
}

// --- GET /api/inspections ---
func (h *InspectionHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT ir.id, ir.provider_id, ir.state, ir.started_at, ir.completed_at,
		       ir.score, ir.total_items, ir.items_passed, ir.items_failed, ir.items_na,
		       (SELECT COUNT(*) FROM inspection_responses WHERE run_id = ir.id)
		FROM inspection_runs ir
		WHERE ir.provider_id = ?
		ORDER BY ir.started_at DESC`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()

	out := make([]inspectionRunDTO, 0)
	for rows.Next() {
		dto, err := scanRunRow(rows)
		if err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, dto)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// --- GET /api/inspections/:id ---
func (h *InspectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")

	run, err := h.loadRun(r, pid, id)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	responses, err := h.loadResponses(r, id)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	checklist := inspection.ChecklistFor(models.StateCode(run.State))

	payload := runDetailDTO{
		Run:       run,
		Checklist: checklist,
		Responses: responses,
	}
	if run.CompletedAt != nil {
		payload.Domains = buildDomainBreakdown(checklist.Items, responses)
		payload.Citations = buildCitationRisks(checklist.Items, responses)
	}
	httpx.RenderJSON(w, http.StatusOK, payload)
}

// --- PATCH /api/inspections/:id/items/:item_id ---
// Upserts a single response. Validates the item_id belongs to the run's state
// checklist and the answer is one of pass|fail|na.
func (h *InspectionHandler) UpsertResponse(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	runID := chi.URLParam(r, "id")
	itemID := chi.URLParam(r, "item_id")

	var in struct {
		Answer             string `json:"answer"`
		Note               string `json:"note"`
		EvidenceDocumentID string `json:"evidence_document_id"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	switch in.Answer {
	case "pass", "fail", "na":
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("answer must be pass, fail, or na"))
		return
	}

	// Confirm the run exists, belongs to this provider, and isn't finalized.
	var state string
	var completed sql.NullTime
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT state, completed_at FROM inspection_runs WHERE id = ? AND provider_id = ?`,
		runID, pid).Scan(&state, &completed)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	if completed.Valid {
		httpx.RenderError(w, r, httpx.ConflictF("inspection already finalized"))
		return
	}

	// Confirm the item exists in this state's checklist.
	if !itemBelongsTo(itemID, models.StateCode(state)) {
		httpx.RenderError(w, r, httpx.BadRequestf("item %s not part of %s checklist", itemID, state))
		return
	}

	// Sanitize optional fields.
	var evidenceID sql.NullString
	if in.EvidenceDocumentID != "" {
		evidenceID = sql.NullString{String: in.EvidenceDocumentID, Valid: true}
	}
	var note sql.NullString
	if in.Note != "" {
		note = sql.NullString{String: in.Note, Valid: true}
	}

	// Upsert keyed on (run_id, item_id).
	newID := base62.NewID()[:22]
	_, err = h.Pool.ExecContext(r.Context(), `
		INSERT INTO inspection_responses (id, run_id, item_id, answer, evidence_document_id, note, answered_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(run_id, item_id) DO UPDATE SET
		  answer = excluded.answer,
		  evidence_document_id = excluded.evidence_document_id,
		  note = excluded.note,
		  answered_at = CURRENT_TIMESTAMP`,
		newID, runID, itemID, in.Answer, evidenceID, note)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Keep the counters in sync so list endpoints render accurate totals.
	if err := h.recountRun(r, runID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	run, err := h.loadRun(r, pid, runID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"run": run,
		"response": responseDTO{
			ItemID:             itemID,
			Answer:             in.Answer,
			Note:               in.Note,
			EvidenceDocumentID: in.EvidenceDocumentID,
			AnsweredAt:         time.Now().UTC(),
		},
	})
}

// --- POST /api/inspections/:id/finalize ---
// Locks the run, computes a weighted score (critical=5, major=3, minor=1) and
// returns summary data: score, per-domain breakdown, and predicted-citation
// list (failed items sorted by severity desc, then domain).
func (h *InspectionHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	runID := chi.URLParam(r, "id")

	run, err := h.loadRun(r, pid, runID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	if run.CompletedAt != nil {
		httpx.RenderError(w, r, httpx.ConflictF("already finalized"))
		return
	}

	responses, err := h.loadResponses(r, runID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	checklist := inspection.ChecklistFor(models.StateCode(run.State))
	score := computeScore(checklist.Items, responses)
	domains := buildDomainBreakdown(checklist.Items, responses)
	citations := buildCitationRisks(checklist.Items, responses)

	_, err = h.Pool.ExecContext(r.Context(), `
		UPDATE inspection_runs SET
		  completed_at = CURRENT_TIMESTAMP,
		  score        = ?
		WHERE id = ? AND provider_id = ?`, score, runID, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Reload for updated completed_at / score.
	run, err = h.loadRun(r, pid, runID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, runDetailDTO{
		Run:       run,
		Checklist: checklist,
		Responses: responses,
		Domains:   domains,
		Citations: citations,
	})
}

// --- GET /api/inspections/:id/report.pdf ---
// No PDF library is vendored in this module, so we render the report as
// self-contained HTML and serve it with a Content-Disposition: attachment and
// .html extension. The filename keeps a `.pdf` extension because multiple
// browsers/print-to-PDF flows assume that name. The HTML is print-optimized so
// Ctrl-P produces a clean single-file PDF.
func (h *InspectionHandler) Report(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	runID := chi.URLParam(r, "id")

	run, err := h.loadRun(r, pid, runID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	responses, err := h.loadResponses(r, runID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	checklist := inspection.ChecklistFor(models.StateCode(run.State))

	// If the run isn't finalized yet we still show an interim report so the
	// owner can print progress mid-walk-through. Compute score in-flight.
	score := 0
	if run.CompletedAt != nil && run.Score != nil {
		score = *run.Score
	} else {
		score = computeScore(checklist.Items, responses)
	}
	domains := buildDomainBreakdown(checklist.Items, responses)
	citations := buildCitationRisks(checklist.Items, responses)

	providerName, _ := h.providerName(r, pid)
	html := renderReportHTML(providerName, run, checklist, responses, domains, citations, score)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="inspection-%s.html"`, runID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

// --- helpers ---

func (h *InspectionHandler) providerState(r *http.Request, pid string) (models.StateCode, error) {
	var s string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT state_code FROM providers WHERE id = ?`, pid).Scan(&s)
	if err != nil {
		return "", err
	}
	return models.StateCode(s), nil
}

func (h *InspectionHandler) providerName(r *http.Request, pid string) (string, error) {
	var n string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT name FROM providers WHERE id = ?`, pid).Scan(&n)
	return n, err
}

func (h *InspectionHandler) loadRun(r *http.Request, pid, id string) (inspectionRunDTO, error) {
	row := h.Pool.QueryRowContext(r.Context(), `
		SELECT ir.id, ir.provider_id, ir.state, ir.started_at, ir.completed_at,
		       ir.score, ir.total_items, ir.items_passed, ir.items_failed, ir.items_na,
		       (SELECT COUNT(*) FROM inspection_responses WHERE run_id = ir.id)
		FROM inspection_runs ir
		WHERE ir.id = ? AND ir.provider_id = ?`, id, pid)
	return scanRunRow(row)
}

// scanRow is satisfied by both *sql.Row and *sql.Rows.
type scanRow interface {
	Scan(dst ...any) error
}

func scanRunRow(row scanRow) (inspectionRunDTO, error) {
	var dto inspectionRunDTO
	var completed sql.NullTime
	var score sql.NullInt64
	if err := row.Scan(&dto.ID, &dto.ProviderID, &dto.State, &dto.StartedAt,
		&completed, &score, &dto.TotalItems, &dto.ItemsPassed, &dto.ItemsFailed, &dto.ItemsNA,
		&dto.ItemsAnswered); err != nil {
		return dto, err
	}
	if completed.Valid {
		t := completed.Time
		dto.CompletedAt = &t
	}
	if score.Valid {
		s := int(score.Int64)
		dto.Score = &s
	}
	dto.FormRef = inspection.FormRefFor(models.StateCode(dto.State))
	return dto, nil
}

func (h *InspectionHandler) loadResponses(r *http.Request, runID string) ([]responseDTO, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT item_id, answer, COALESCE(note, ''), COALESCE(evidence_document_id, ''), answered_at
		FROM inspection_responses WHERE run_id = ?
		ORDER BY answered_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]responseDTO, 0)
	for rows.Next() {
		var r responseDTO
		if err := rows.Scan(&r.ItemID, &r.Answer, &r.Note, &r.EvidenceDocumentID, &r.AnsweredAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (h *InspectionHandler) recountRun(r *http.Request, runID string) error {
	_, err := h.Pool.ExecContext(r.Context(), `
		UPDATE inspection_runs SET
		  items_passed = (SELECT COUNT(*) FROM inspection_responses WHERE run_id = ? AND answer = 'pass'),
		  items_failed = (SELECT COUNT(*) FROM inspection_responses WHERE run_id = ? AND answer = 'fail'),
		  items_na     = (SELECT COUNT(*) FROM inspection_responses WHERE run_id = ? AND answer = 'na')
		WHERE id = ?`,
		runID, runID, runID, runID)
	return err
}

// itemBelongsTo verifies an itemID exists in a state's checklist. Cheap — all
// checklists are small in-memory slices.
func itemBelongsTo(itemID string, state models.StateCode) bool {
	for _, it := range inspection.For(state) {
		if it.ID == itemID {
			return true
		}
	}
	return false
}

// computeScore produces a 0..100 weighted pass rate. Only answered items
// contribute. Unanswered items are treated as "fail" so owners can't game the
// score by skipping tough questions. "na" items are excluded entirely (they
// don't apply to this facility).
func computeScore(items []inspection.Item, responses []responseDTO) int {
	byID := map[string]string{}
	for _, r := range responses {
		byID[r.ItemID] = r.Answer
	}
	weighted := 0.0
	total := 0.0
	for _, it := range items {
		ans, ok := byID[it.ID]
		if ok && ans == "na" {
			continue
		}
		w := it.Severity.Weight()
		total += w
		if ok && ans == "pass" {
			weighted += w
		}
	}
	if total == 0 {
		return 100
	}
	return int((weighted / total) * 100)
}

func buildDomainBreakdown(items []inspection.Item, responses []responseDTO) []domainBreakdown {
	byID := map[string]string{}
	for _, r := range responses {
		byID[r.ItemID] = r.Answer
	}
	ordered := []string{}
	seen := map[string]*domainBreakdown{}
	for _, it := range items {
		d, ok := seen[it.Domain]
		if !ok {
			d = &domainBreakdown{Name: it.Domain}
			seen[it.Domain] = d
			ordered = append(ordered, it.Domain)
		}
		d.Total++
		ans, answered := byID[it.ID]
		if !answered {
			d.Unanswered++
			continue
		}
		switch ans {
		case "pass":
			d.Passed++
		case "fail":
			d.Failed++
		case "na":
			d.NA++
		}
	}
	out := make([]domainBreakdown, 0, len(ordered))
	for _, n := range ordered {
		out = append(out, *seen[n])
	}
	return out
}

// buildCitationRisks returns failed items sorted by severity weight desc, then
// domain order. This is the "predicted citation" list the owner sees after a
// finalize call — a prioritized fix list.
func buildCitationRisks(items []inspection.Item, responses []responseDTO) []citationRisk {
	type response struct {
		answer string
		note   string
	}
	byID := map[string]response{}
	for _, r := range responses {
		byID[r.ItemID] = response{answer: r.Answer, note: r.Note}
	}
	out := []citationRisk{}
	for _, it := range items {
		r, ok := byID[it.ID]
		if !ok || r.answer != "fail" {
			continue
		}
		out = append(out, citationRisk{
			ItemID:    it.ID,
			Domain:    it.Domain,
			Question:  it.Question,
			Reference: it.Reference,
			FormRef:   it.FormRef,
			Severity:  it.Severity,
			Note:      r.note,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Severity.Weight() > out[j].Severity.Weight()
	})
	return out
}

// renderReportHTML produces a printable, self-contained HTML document. No
// external assets, no JS. Designed to pass through browser "Print to PDF" with
// clean page breaks between domains.
func renderReportHTML(providerName string, run inspectionRunDTO,
	checklist inspection.Checklist, responses []responseDTO,
	domains []domainBreakdown, citations []citationRisk, score int) string {

	byID := map[string]responseDTO{}
	for _, r := range responses {
		byID[r.ItemID] = r
	}
	itemsByDomain := map[string][]inspection.Item{}
	domainOrder := []string{}
	for _, it := range checklist.Items {
		if _, ok := itemsByDomain[it.Domain]; !ok {
			domainOrder = append(domainOrder, it.Domain)
		}
		itemsByDomain[it.Domain] = append(itemsByDomain[it.Domain], it)
	}

	var buf bytes.Buffer
	fmt.Fprintln(&buf, `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<title>Mock Inspection Report</title>
<style>
 @page { margin: 0.6in; }
 * { box-sizing: border-box; }
 body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
   color: #111; font-size: 12px; line-height: 1.4; margin: 0; padding: 24px; }
 h1 { font-size: 22px; margin: 0 0 4px; }
 h2 { font-size: 16px; margin: 24px 0 8px; border-bottom: 1px solid #ddd; padding-bottom: 4px;
      page-break-after: avoid; }
 .meta { color: #555; font-size: 11px; margin-bottom: 24px; }
 .score-card { display: flex; align-items: center; gap: 24px;
   padding: 16px; border: 1px solid #ddd; border-radius: 8px; margin-bottom: 16px; }
 .score-num { font-size: 44px; font-weight: 700; line-height: 1; }
 .score-label { color: #555; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; }
 .counts { display: flex; gap: 18px; flex-wrap: wrap; }
 .count { background: #f8f8f8; padding: 6px 10px; border-radius: 4px; font-size: 11px; }
 .count strong { font-size: 14px; display: block; }
 table { width: 100%; border-collapse: collapse; margin-top: 8px; }
 th, td { text-align: left; padding: 6px 8px; border-bottom: 1px solid #eee; vertical-align: top; }
 th { font-size: 10px; text-transform: uppercase; color: #555; letter-spacing: 0.4px; }
 .pill { display: inline-block; font-size: 10px; padding: 2px 6px; border-radius: 999px;
   font-weight: 600; text-transform: uppercase; letter-spacing: 0.4px; }
 .pass { background: #e6f4ea; color: #137333; }
 .fail { background: #fce8e6; color: #b3261e; }
 .na   { background: #e8eaed; color: #555; }
 .unanswered { background: #fff4e5; color: #a5590a; }
 .sev-critical { color: #b3261e; font-weight: 700; }
 .sev-major { color: #a5590a; font-weight: 600; }
 .sev-minor { color: #555; }
 .domain { page-break-inside: avoid; }
 .note { color: #555; font-size: 11px; margin-top: 4px; font-style: italic; }
 ul.citations { padding-left: 18px; }
 ul.citations li { margin-bottom: 6px; }
</style></head><body>`)

	fmt.Fprintf(&buf, `<h1>Mock Inspection Report</h1>
<div class="meta">%s &middot; %s &middot; Run %s &middot; Started %s`,
		html.EscapeString(providerName),
		html.EscapeString(checklist.FormRef),
		html.EscapeString(run.ID),
		run.StartedAt.Format("Jan 2, 2006 3:04 PM"))
	if run.CompletedAt != nil {
		fmt.Fprintf(&buf, " &middot; Completed %s", run.CompletedAt.Format("Jan 2, 2006 3:04 PM"))
	}
	fmt.Fprintln(&buf, "</div>")

	fmt.Fprintf(&buf, `<div class="score-card">
 <div><div class="score-label">Inspection Score</div><div class="score-num">%d</div></div>
 <div class="counts">
  <div class="count"><strong>%d</strong>Total items</div>
  <div class="count"><strong>%d</strong>Passed</div>
  <div class="count"><strong>%d</strong>Failed</div>
  <div class="count"><strong>%d</strong>N/A</div>
 </div></div>`,
		score, run.TotalItems, run.ItemsPassed, run.ItemsFailed, run.ItemsNA)

	// Predicted citations.
	if len(citations) > 0 {
		fmt.Fprintln(&buf, "<h2>Predicted citations (fix these before the inspector arrives)</h2>")
		fmt.Fprintln(&buf, `<ul class="citations">`)
		for _, c := range citations {
			sevClass := "sev-" + string(c.Severity)
			fmt.Fprintf(&buf,
				`<li><span class="%s">[%s]</span> <strong>%s</strong> &mdash; %s <span style="color:#999">(%s, %s)</span>`,
				html.EscapeString(sevClass),
				html.EscapeString(string(c.Severity)),
				html.EscapeString(c.Domain),
				html.EscapeString(c.Question),
				html.EscapeString(c.Reference),
				html.EscapeString(c.FormRef),
			)
			if c.Note != "" {
				fmt.Fprintf(&buf, `<div class="note">Note: %s</div>`, html.EscapeString(c.Note))
			}
			fmt.Fprintln(&buf, "</li>")
		}
		fmt.Fprintln(&buf, "</ul>")
	}

	// Per-domain breakdown.
	fmt.Fprintln(&buf, "<h2>Domain breakdown</h2>")
	fmt.Fprintln(&buf, `<table><thead><tr><th>Domain</th><th>Pass</th><th>Fail</th><th>N/A</th><th>Unanswered</th></tr></thead><tbody>`)
	for _, d := range domains {
		fmt.Fprintf(&buf, `<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td></tr>`,
			html.EscapeString(d.Name), d.Passed, d.Failed, d.NA, d.Unanswered)
	}
	fmt.Fprintln(&buf, "</tbody></table>")

	// Full item walk-through by domain.
	for _, dn := range domainOrder {
		fmt.Fprintf(&buf, `<h2 class="domain">%s</h2>`, html.EscapeString(dn))
		fmt.Fprintln(&buf, `<table><thead><tr><th>Item</th><th>Question</th><th>Severity</th><th>Answer</th></tr></thead><tbody>`)
		for _, it := range itemsByDomain[dn] {
			r, ok := byID[it.ID]
			ansCell := `<span class="pill unanswered">Unanswered</span>`
			if ok {
				ansCell = fmt.Sprintf(`<span class="pill %s">%s</span>`,
					html.EscapeString(r.Answer), html.EscapeString(strings.ToUpper(r.Answer)))
			}
			sevClass := "sev-" + string(it.Severity)
			fmt.Fprintf(&buf, `<tr><td>%s</td><td>%s<div class="note">%s &middot; %s</div>`,
				html.EscapeString(it.FormRef),
				html.EscapeString(it.Question),
				html.EscapeString(it.Reference),
				html.EscapeString(it.ID),
			)
			if ok && r.Note != "" {
				fmt.Fprintf(&buf, `<div class="note">Note: %s</div>`, html.EscapeString(r.Note))
			}
			if ok && r.EvidenceDocumentID != "" {
				fmt.Fprintf(&buf, `<div class="note">Evidence document: %s</div>`, html.EscapeString(r.EvidenceDocumentID))
			}
			fmt.Fprintf(&buf, `</td><td><span class="%s">%s</span></td><td>%s</td></tr>`,
				html.EscapeString(sevClass),
				html.EscapeString(string(it.Severity)),
				ansCell,
			)
		}
		fmt.Fprintln(&buf, "</tbody></table>")
	}

	fmt.Fprintln(&buf, `</body></html>`)
	return buf.String()
}
