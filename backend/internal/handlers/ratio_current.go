package handlers

// REQ-040: replace the free-text ratio input with a per-room computed
// display.  Florida law sets staff:child ratios by age band (Handbook
// §3.4), not by operator preference. The Operations → Ratio tab used to
// ask "how many staff per child do you want?" — wrong, and not what
// inspectors check.
//
// This handler reads:
//   - The rooms table for the provider's classroom layout.
//   - Today's attendance events to determine who is signed in.
//   - The provider's state to pick the right ratio table.
//
// It returns a per-room snapshot: age band, state-mandated ratio,
// children present in that age band, total staff present, and a
// compliance status badge (Compliant / Tight / Out-of-ratio).
//
// We do NOT model child→room assignment yet (no column for it). For the
// FL launch we bucket children into rooms by age band: every signed-in
// child whose computed age falls in the same band as the room is counted
// there. Staff are pooled facility-wide; the badge uses the room with
// the most pressure.

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// RatioCurrentHandler answers GET /api/ratio/current — the room-by-room
// snapshot the new Ratio tab renders.
type RatioCurrentHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// FLBandSpec encodes one FL age band: the operator-visible label, the
// inclusive age range in months, and the maximum children per staff.
// Authoritative reference: F.S. §402.305(4) + Handbook §3.4.
type FLBandSpec struct {
	Value         string // matches frontend FL_AGE_BANDS .value
	Label         string
	MinMonths     int
	MaxMonthsExcl int // exclusive upper bound; 9999 = no upper limit
	PerStaff      int // ratio cap, "1 staff per N children"
}

// FLBands is the canonical ordered table. The same data lives in
// internal/compliance/ratio.go computeBands; we keep a second copy here
// keyed on the frontend's band value strings so the API can return a
// consistent identifier on the way out.
//
// NOTE: the "ones" band covers 1-year-olds (12-23 mo), not "1 staff" —
// the FL_AGE_BANDS value strings on the frontend collapse into these
// six rows. school_age is 5+ in FL (same ratio as fives).
var FLBands = []FLBandSpec{
	{"infant_0_11", "Infants (0–11 mo)", 0, 12, 4},
	{"ones", "1-year-olds", 12, 24, 6},
	{"twos", "2-year-olds", 24, 36, 11},
	{"threes", "3-year-olds", 36, 48, 15},
	{"fours", "4-year-olds", 48, 60, 20},
	{"fives", "5-year-olds", 60, 9999, 25},
	{"school_age", "School age", 60, 9999, 25},
}

// findBand resolves a frontend band-value string to a band spec, or nil
// if the room has no age_band set.
func findBand(value string) *FLBandSpec {
	for i := range FLBands {
		if FLBands[i].Value == value {
			return &FLBands[i]
		}
	}
	return nil
}

// findBandForAge returns the band spec a given age-in-months falls into.
// fives and school_age overlap; we always prefer "fives" for 60-71 mo
// and "school_age" for 72+. Returns nil if no band matches.
func findBandForAge(months int) *FLBandSpec {
	if months >= 72 {
		for i := range FLBands {
			if FLBands[i].Value == "school_age" {
				return &FLBands[i]
			}
		}
	}
	for i := range FLBands {
		b := FLBands[i]
		if b.Value == "school_age" {
			continue
		}
		if months >= b.MinMonths && months < b.MaxMonthsExcl {
			return &b
		}
	}
	return nil
}

// RoomRatioStatus is one row in the response — the dashboard renders
// these as a calm-colored card per room.
type RoomRatioStatus struct {
	RoomID          string `json:"room_id"`
	RoomName        string `json:"room_name"`
	AgeBand         string `json:"age_band"`
	AgeBandLabel    string `json:"age_band_label"`
	MandatedRatio   int    `json:"mandated_ratio"` // max children per single staff
	ChildrenPresent int    `json:"children_present"`
	StaffPresent    int    `json:"staff_present"`
	Status          string `json:"status"` // "compliant" | "tight" | "out_of_ratio" | "unknown"
	Explainer       string `json:"explainer"`
}

// RatioCurrentResponse is the top-level shape /api/ratio/current returns.
type RatioCurrentResponse struct {
	State         string            `json:"state"`
	AsOf          string            `json:"as_of"`
	StaffPresent  int               `json:"staff_present_total"`
	ChildrenTotal int               `json:"children_present_total"`
	Rooms         []RoomRatioStatus `json:"rooms"`
	AnyOutOfRatio bool              `json:"any_out_of_ratio"`
}

// Get handles GET /api/ratio/current. Read-only — no DB writes.
func (h *RatioCurrentHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}

	// Provider state — we only support FL ratios today, but we surface
	// the state so the UI can render a "your state's ratios aren't loaded
	// yet" hint for TX/CA centers as soon as they sign up.
	var state string
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(state_code, state, '') FROM providers WHERE id = ?`, pid).Scan(&state); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	stateCode := models.StateCode(strings.ToUpper(state))

	rooms, err := h.loadRooms(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Today's roster — children with computed age in months + total staff
	// signed in. Pool staff facility-wide (no per-room mapping yet).
	kidAges, staffCount, err := h.todayRoster(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Build a per-band head count. Each room shares the staff pool, so
	// the "staff present" field on every room is the same total.
	countByBand := map[string]int{}
	for _, months := range kidAges {
		b := findBandForAge(months)
		if b == nil {
			continue
		}
		countByBand[b.Value]++
	}

	// Special case: a "school_age" room should pull in any child >= 60mo
	// who isn't already assigned to a "fives" room. We approximate by
	// counting >=72 mo separately and bucketing into school_age. Since
	// findBandForAge already routes >=72 to school_age, this is fine.

	out := RatioCurrentResponse{
		State:         string(stateCode),
		AsOf:          time.Now().UTC().Format(time.RFC3339),
		StaffPresent:  staffCount,
		ChildrenTotal: len(kidAges),
		Rooms:         make([]RoomRatioStatus, 0, len(rooms)),
	}

	for _, rm := range rooms {
		row := RoomRatioStatus{
			RoomID:       rm.ID,
			RoomName:     rm.Name,
			AgeBand:      rm.AgeBand,
			StaffPresent: staffCount,
		}
		spec := findBand(rm.AgeBand)
		if spec == nil {
			row.AgeBandLabel = "Age band not set"
			row.Status = "unknown"
			row.Explainer = "Pick an age band for this room on the Settings → Facility page so we can show its required ratio."
			out.Rooms = append(out.Rooms, row)
			continue
		}
		row.AgeBandLabel = spec.Label
		row.MandatedRatio = spec.PerStaff
		row.ChildrenPresent = countByBand[spec.Value]
		row.Status, row.Explainer = classifyRoom(row.ChildrenPresent, staffCount, spec.PerStaff)
		if row.Status == "out_of_ratio" {
			out.AnyOutOfRatio = true
		}
		out.Rooms = append(out.Rooms, row)
	}

	httpx.RenderJSON(w, http.StatusOK, out)
}

// classifyRoom returns (status, explainer) for one room. Pure function,
// safe to unit-test.
//
// Rules (FL launch — calm copy, no scary language):
//   - 0 children present: "compliant" — "No children in this band right now."
//   - 0 staff but children present: "out_of_ratio" — "You need at least one
//     staff member with these children."
//   - children > staff * mandatedRatio: "out_of_ratio".
//   - children > staff * (mandatedRatio - 1): "tight" — one more child and
//     you'd need to add staff. Skipped when mandatedRatio == 1.
//   - otherwise: "compliant".
func classifyRoom(children, staff, mandated int) (string, string) {
	if children == 0 {
		return "compliant", "No children in this band right now. You're good."
	}
	if staff == 0 {
		return "out_of_ratio", "You need at least one staff member with these children."
	}
	cap := staff * mandated
	if children > cap {
		need := (children + mandated - 1) / mandated // ceil
		more := need - staff
		if more < 1 {
			more = 1
		}
		return "out_of_ratio", calmPlural(more,
			"Add one more staff member to this band or stop adding kids.",
			"You need "+itoa(more)+" more staff in this band, or stop adding kids.")
	}
	// "Tight" is at-or-very-near the cap.
	if mandated > 1 && children > staff*(mandated-1) {
		return "tight", "You're at the edge of ratio. One more child and you'd need another staff member."
	}
	return "compliant", "You're within the state ratio."
}

func calmPlural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// roomRow is the minimal projection we need out of the rooms table.
type roomRow struct {
	ID, Name, AgeBand string
}

// loadRooms reads the room list for a provider, filtered to non-deleted.
func (h *RatioCurrentHandler) loadRooms(r *http.Request, pid string) ([]roomRow, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, name, COALESCE(age_band, '')
		  FROM rooms
		 WHERE provider_id = ? AND deleted_at IS NULL
		 ORDER BY created_at ASC`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []roomRow{}
	for rows.Next() {
		var rm roomRow
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.AgeBand); err != nil {
			return nil, err
		}
		out = append(out, rm)
	}
	return out, nil
}

// todayRoster returns the age-in-months of every child whose most-recent
// attendance event today is a _in, plus the count of staff in the same
// state. Mirrors AttendanceHandler.currentRoster but only pulls the
// fields ratio compute needs (no parent names, no allergies).
func (h *RatioCurrentHandler) todayRoster(r *http.Request, pid string) ([]int, int, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		WITH latest AS (
			SELECT subject_kind, subject_id,
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
		SELECT subject_kind, subject_id FROM latest WHERE last_kind LIKE '%_in'`,
		pid)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var childIDs []string
	staffCount := 0
	for rows.Next() {
		var kind, id string
		if err := rows.Scan(&kind, &id); err != nil {
			return nil, 0, err
		}
		switch kind {
		case "child":
			childIDs = append(childIDs, id)
		case "staff":
			staffCount++
		}
	}

	ages := make([]int, 0, len(childIDs))
	for _, cid := range childIDs {
		var months int
		err := h.Pool.QueryRowContext(r.Context(),
			`SELECT CAST((julianday('now') - julianday(date_of_birth)) / 30.4375 AS INTEGER)
			 FROM children WHERE id = ? AND provider_id = ?`, cid, pid).Scan(&months)
		if err != nil {
			continue // child row missing — skip silently
		}
		if months < 0 {
			months = 0
		}
		ages = append(ages, months)
	}
	return ages, staffCount, nil
}
