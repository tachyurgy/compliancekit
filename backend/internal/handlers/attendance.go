package handlers

// REQ-016: attendance reads + the printable Drill-Ready Roster PDF.
//
// /api/attendance/today        — current head-count, age-band breakdown,
//                                required-floor, and the warnings list.
// /api/attendance/history      — calendar drill-down for Inspector Mode.
// /api/drill-roster.pdf        — letter-size printable roster (no-store).
//
// The PDF is intentionally minimal — we hand-roll a tiny PDF stream rather
// than pull in a library. Letter-size, plain text, no images.

import (
	"bytes"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/compliance"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

type AttendanceHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

type rosterChild struct {
	ID         string `json:"id"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	AgeMonths  int    `json:"age_months"`
	ParentName string `json:"parent_name,omitempty"`
	Phone      string `json:"parent_phone,omitempty"`
	Allergies  string `json:"allergies,omitempty"`
	SignedInAt string `json:"signed_in_at"`
}

type rosterStaff struct {
	ID         string `json:"id"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Role       string `json:"role"`
	SignedInAt string `json:"signed_in_at"`
}

func (h *AttendanceHandler) Today(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	kids, staff, err := h.currentRoster(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Compute the floor + warnings via the pure compliance helper.
	cps := make([]compliance.ChildPresent, 0, len(kids))
	for _, c := range kids {
		cps = append(cps, compliance.ChildPresent{ID: c.ID, AgeMonths: c.AgeMonths})
	}
	sps := make([]compliance.StaffPresent, 0, len(staff))
	for _, s := range staff {
		sps = append(sps, compliance.StaffPresent{ID: s.ID, Role: s.Role})
	}
	res := compliance.CalcRatio(cps, sps, time.Now())

	// Absence flags — children on the day's expected roster who haven't
	// signed in by 9:30 AM. Surface as a parallel list so the dashboard's
	// "call this parent" card has a tel: link to render.
	absences := h.absencesAt(r, pid, time.Now())

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"children_present": kids,
		"staff_present":    staff,
		"by_band":          res.ByBand,
		"required_floor":   res.RequiredFloor,
		"warnings":         res.Warnings,
		"absences":         absences,
		"as_of":            time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *AttendanceHandler) History(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	q := r.URL.Query()
	from := q.Get("from")
	to := q.Get("to")
	if from == "" {
		from = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	subjectKind := q.Get("subject_kind")
	subjectID := q.Get("subject_id")

	args := []any{pid, from, to}
	where := "provider_id = ? AND date(timestamp_utc) BETWEEN ? AND ?"
	if subjectKind != "" {
		where += " AND subject_kind = ?"
		args = append(args, subjectKind)
	}
	if subjectID != "" {
		where += " AND subject_id = ?"
		args = append(args, subjectID)
	}

	rows, err := h.Pool.QueryContext(r.Context(),
		"SELECT timestamp_utc, kind, subject_kind, subject_id, COALESCE(dropped_off_by,''), COALESCE(picked_up_by,''), COALESCE(notes,'') FROM attendance_events WHERE "+where+" ORDER BY timestamp_utc DESC LIMIT 5000",
		args...)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	type event struct {
		At           string `json:"at"`
		Kind         string `json:"kind"`
		SubjectKind  string `json:"subject_kind"`
		SubjectID    string `json:"subject_id"`
		DroppedOffBy string `json:"dropped_off_by,omitempty"`
		PickedUpBy   string `json:"picked_up_by,omitempty"`
		Notes        string `json:"notes,omitempty"`
	}
	out := []event{}
	for rows.Next() {
		var e event
		_ = rows.Scan(&e.At, &e.Kind, &e.SubjectKind, &e.SubjectID, &e.DroppedOffBy, &e.PickedUpBy, &e.Notes)
		out = append(out, e)
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"events": out, "from": from, "to": to})
}

// DrillRosterPDF — letter-size, prints in one tap. Plain-text PDF stream.
func (h *AttendanceHandler) DrillRosterPDF(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var facility, addr string
	_ = h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(name, legal_name, ''), COALESCE(address_line1,'') FROM providers WHERE id = ?`, pid).Scan(&facility, &addr)

	kids, staff, err := h.currentRoster(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	pdf := buildDrillRosterPDF(facility, addr, kids, staff, time.Now())
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `inline; filename="drill-roster.pdf"`)
	_, _ = w.Write(pdf)
}

// ---- shared loaders ----

// currentRoster returns every subject whose latest attendance event today is
// an `_in`. Computes age-in-months for each child from DOB so the band math
// in compliance/ratio.go works without a second round-trip.
func (h *AttendanceHandler) currentRoster(r *http.Request, pid string) (kids []rosterChild, staff []rosterStaff, err error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		WITH latest AS (
			SELECT subject_kind, subject_id,
			       MAX(timestamp_utc) AS at,
			       (SELECT kind FROM attendance_events e2
			        WHERE e2.provider_id = e.provider_id
			          AND e2.subject_kind = e.subject_kind
			          AND e2.subject_id = e.subject_id
			          AND date(e2.timestamp_utc) = date('now')
			        ORDER BY e2.timestamp_utc DESC LIMIT 1) AS last_kind
			FROM attendance_events e
			WHERE e.provider_id = ? AND date(e.timestamp_utc) = date('now')
			GROUP BY subject_kind, subject_id
		)
		SELECT subject_kind, subject_id, at FROM latest
		WHERE last_kind LIKE '%_in'
		ORDER BY at DESC`, pid)
	if err != nil {
		return nil, nil, err
	}
	type sin struct{ kind, id, at string }
	var sins []sin
	for rows.Next() {
		var s sin
		_ = rows.Scan(&s.kind, &s.id, &s.at)
		sins = append(sins, s)
	}
	_ = rows.Close()

	for _, s := range sins {
		if s.kind == "child" {
			c := rosterChild{ID: s.id, SignedInAt: s.at}
			var months int
			_ = h.Pool.QueryRowContext(r.Context(),
				`SELECT first_name, last_name, COALESCE(parent_name,''), COALESCE(parent_phone,''), COALESCE(allergies,''),
				        CAST((julianday('now') - julianday(date_of_birth)) / 30.4375 AS INTEGER) AS age_months
				 FROM children WHERE id = ? AND provider_id = ?`,
				s.id, pid).Scan(&c.FirstName, &c.LastName, &c.ParentName, &c.Phone, &c.Allergies, &months)
			c.AgeMonths = months
			kids = append(kids, c)
		} else {
			st := rosterStaff{ID: s.id, SignedInAt: s.at}
			_ = h.Pool.QueryRowContext(r.Context(),
				`SELECT first_name, last_name, COALESCE(role,'staff') FROM staff WHERE id = ? AND provider_id = ?`,
				s.id, pid).Scan(&st.FirstName, &st.LastName, &st.Role)
			staff = append(staff, st)
		}
	}
	return kids, staff, nil
}

// absencesAt returns children on today's expected roster who have not been
// signed in by 9:30 AM local. Returns nothing before 9:30. The expected_days
// column is a JSON array of weekday strings ("mon","tue",...); we treat a
// NULL/missing value as "all weekdays" so a freshly-onboarded center
// surfaces sensible defaults until the operator edits it.
func (h *AttendanceHandler) absencesAt(r *http.Request, pid string, now time.Time) []map[string]any {
	if now.Hour() < 9 || (now.Hour() == 9 && now.Minute() < 30) {
		return []map[string]any{}
	}
	weekday := strings.ToLower(now.Format("Mon"))[:3] // "mon","tue",...

	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, first_name, last_name, COALESCE(parent_name,''), COALESCE(parent_phone,''),
		       COALESCE(expected_days, '["mon","tue","wed","thu","fri"]')
		FROM children
		WHERE provider_id = ? AND status = 'enrolled' AND deleted_at IS NULL`, pid)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, first, last, pname, phone, expected string
		_ = rows.Scan(&id, &first, &last, &pname, &phone, &expected)
		if !strings.Contains(strings.ToLower(expected), weekday) {
			continue
		}
		// Was this child signed in today?
		var signedIn int
		_ = h.Pool.QueryRowContext(r.Context(),
			`SELECT 1 FROM attendance_events
			 WHERE provider_id = ? AND subject_kind = 'child' AND subject_id = ?
			   AND kind = 'child_in' AND date(timestamp_utc) = date('now') LIMIT 1`,
			pid, id).Scan(&signedIn)
		if signedIn == 1 {
			continue
		}
		out = append(out, map[string]any{
			"child_id":     id,
			"first_name":   first,
			"last_name":    last,
			"parent_name":  pname,
			"parent_phone": phone,
		})
	}
	return out
}

// ---- the PDF builder ----
//
// Hand-rolled PDF — keeps us off another dependency. Letter-size, paginates
// every ~28 lines.  Header: facility name, address, generation timestamp.
// Body: child rows then staff rows. Allergies tagged so the PDF reader can
// see them inline (we cannot color text in a basic PDF stream without
// embedding a graphics state, so we prefix with "ALLERGIES:" — the
// front-end render uses real coral type).

func buildDrillRosterPDF(facility, addr string, kids []rosterChild, staff []rosterStaff, now time.Time) []byte {
	const left = 54.0
	const top = 740.0
	const bottom = 60.0
	const lineH = 13.0

	// Build a slice of lines first, then split into pages.
	lines := []string{}
	add := func(s string) { lines = append(lines, s) }
	add(facility)
	if addr != "" {
		add(addr)
	}
	add("Drill-Ready Roster — current as of " + now.Format("Mon Jan 2, 2006  3:04 PM"))
	add("")
	add(fmt.Sprintf("Children present (%d)", len(kids)))
	for _, c := range kids {
		text := fmt.Sprintf("- %s %s   Parent: %s %s", c.FirstName, c.LastName, c.ParentName, c.Phone)
		if c.Allergies != "" {
			text += "   ALLERGIES: " + c.Allergies
		}
		add(text)
	}
	add("")
	add(fmt.Sprintf("Staff present (%d)", len(staff)))
	for _, s := range staff {
		add(fmt.Sprintf("- %s %s   %s", s.FirstName, s.LastName, s.Role))
	}
	add("")
	add("Generated by Levelbrook · This roster is current as of " + now.Format("3:04 PM Mon Jan 2"))

	// Paginate.
	pages := [][]string{}
	cur := []string{}
	maxPerPage := 52 // (740 - 60) / 13, computed once and inlined.
	for _, l := range lines {
		cur = append(cur, l)
		if len(cur) >= maxPerPage {
			pages = append(pages, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		pages = append(pages, cur)
	}

	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	write := func(s string) {
		offsets = append(offsets, b.Len())
		b.WriteString(s)
	}

	// Object 1 = Catalog, 2 = Pages, 3..N = each Page object.
	write("1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n")

	// We need the Pages object to enumerate Kids. Build it after we know N.
	pagesPlaceholderIdx := len(offsets) // remember where to write
	write("PLACEHOLDER PAGES\n")

	var fontObjID int

	pageObjIDs := []int{}
	for i, page := range pages {
		// Page header line size 18 if the first line includes the facility name.
		// To keep things simple: line 0 = size 16, line 1 = size 11, others 10.
		var content bytes.Buffer
		y := top
		for j, l := range page {
			size := 10.0
			if i == 0 && j == 0 {
				size = 16
			} else if i == 0 && j == 1 {
				size = 11
			}
			fmt.Fprintf(&content, "BT /F1 %.1f Tf %.1f %.1f Td (%s) Tj ET\n", size, left, y, rosterPDFEscape(l))
			y -= lineH
			if y < bottom {
				break
			}
		}

		stream := content.Bytes()
		contentObjNum := len(offsets) + 1
		pageObjNum := contentObjNum + 1
		// Write content stream first.
		write(fmt.Sprintf("%d 0 obj << /Length %d >> stream\n%s\nendstream endobj\n", contentObjNum, len(stream), stream))
		// Then page object referencing it. Font ref is computed after the loop;
		// for now we use a temporary placeholder we'll fix up via ID 999.
		pageObjIDs = append(pageObjIDs, pageObjNum)
		write(fmt.Sprintf("%d 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 999 0 R >> >> /Contents %d 0 R >> endobj\n", pageObjNum, contentObjNum))
	}

	// Font object — last.
	fontObjID = len(offsets)
	write(fmt.Sprintf("%d 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n", fontObjID))

	// Now patch the placeholder Pages object and the 999-references in each page.
	out := b.String()
	out = strings.Replace(out, "PLACEHOLDER PAGES\n",
		fmt.Sprintf("2 0 obj << /Type /Pages /Kids [%s] /Count %d >> endobj\n",
			joinPageRefs(pageObjIDs), len(pageObjIDs)), 1)
	out = strings.ReplaceAll(out, "999 0 R", fmt.Sprintf("%d 0 R", fontObjID))

	// Recompute offsets — easiest is to re-scan the patched string for each "N 0 obj" header.
	patched := []byte(out)
	xrefOffset := len(patched)
	// Build xref table by scanning for each object header.
	objectCount := 1 + len(pageObjIDs)*2 + 1 // pages + (content+page)*N + font + catalog
	_ = objectCount
	_ = pagesPlaceholderIdx

	var xref bytes.Buffer
	highest := 1 + len(pageObjIDs)*2 + 1 // catalog,pages,(content+page)*N,font
	xref.WriteString(fmt.Sprintf("xref\n0 %d\n", highest+1))
	xref.WriteString("0000000000 65535 f \n")
	for i := 1; i <= highest; i++ {
		needle := []byte(fmt.Sprintf("%d 0 obj", i))
		idx := bytes.Index(patched, needle)
		if idx < 0 {
			xref.WriteString("0000000000 00000 n \n")
		} else {
			xref.WriteString(fmt.Sprintf("%010d 00000 n \n", idx))
		}
	}
	xref.WriteString(fmt.Sprintf("trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", highest+1, xrefOffset))

	final := append(patched, xref.Bytes()...)
	return final
}

func joinPageRefs(ids []int) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d 0 R", id))
	}
	return strings.Join(parts, " ")
}

// rosterPDFEscape escapes the three characters that have meaning inside a
// PDF literal-string: '(' ')' '\\'. (handlers/inspection_mode.go has its
// own pdfEscape helper for the Plan-of-Correction PDF; we duplicate the
// trivial helper rather than introduce a tiny shared package.)
func rosterPDFEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)")
	return r.Replace(s)
}
