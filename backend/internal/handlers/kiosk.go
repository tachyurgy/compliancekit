package handlers

// REQ-016a: Kiosk endpoints for the iPad mounted at the front door.
//
// Auth model:
//   * IssueSession is admin-authenticated (under /api with the user-session
//     middleware). It mints a 4-hour kiosk_session row and returns the raw
//     ID. The operator copies that ID into the iPad once.
//   * SignIn / SignOut / Roster validate a kiosk session bearer themselves
//     (header `X-Kiosk-Session: <id>` or query param `?ks=<id>`). They run
//     OUTSIDE the user-session group so the iPad doesn't need a logged-in
//     manager.
//   * The PIN typed by the parent / staff is the second factor (bcrypt
//     compare). 5 wrong attempts inside 60s lock the kiosk for 5 minutes
//     (in-process map; we restart cheap on the cx22).

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

type KioskHandler struct {
	Pool *sql.DB
	Log  *slog.Logger

	mu       sync.Mutex
	attempts map[string]*pinAttempt
}

type pinAttempt struct {
	count    int
	first    time.Time
	lockedAt time.Time
}

const (
	kioskSessionTTL    = 4 * time.Hour
	kioskMaxAttempts   = 5
	kioskAttemptWindow = 60 * time.Second
	kioskLockoutTTL    = 5 * time.Minute
)

// IssueSession mints a kiosk session for the caller's facility.
// POST /api/kiosk/session  body: { "label": "front door iPad" }
func (h *KioskHandler) IssueSession(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in struct {
		Label string `json:"label"`
	}
	_ = httpx.DecodeJSON(r, &in)

	id := base62.NewID()
	expires := time.Now().Add(kioskSessionTTL).UTC().Format(time.RFC3339)
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO kiosk_sessions (id, provider_id, label, expires_at)
		VALUES (?, ?, ?, ?)`, id, pid, strings.TrimSpace(in.Label), expires); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"kiosk_session": id,
		"expires_at":    expires,
		"hint":          "Open /kiosk on the iPad and paste this code. It rotates every 4 hours.",
	})
}

// SignIn handles a child or staff sign-in.
// POST /kiosk/sign-in
//
// Body: { "kind":"child_in"|"staff_in", "subject_id":"...", "pin":"1234",
//
//	"dropped_off_by":"name (children only)", "room_id":"...","notes":"..." }
func (h *KioskHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	h.handleEvent(w, r, true)
}

// SignOut mirrors SignIn for the out direction.
func (h *KioskHandler) SignOut(w http.ResponseWriter, r *http.Request) {
	h.handleEvent(w, r, false)
}

// Roster is a lightweight read for the kiosk welcome screen — counts of who's
// currently in. Trades nothing for nothing; the operator dashboard uses
// /api/attendance/today instead.
func (h *KioskHandler) Roster(w http.ResponseWriter, r *http.Request) {
	pid, _, err := h.resolveKiosk(r)
	if err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	kids, staff := h.headcounts(r, pid)
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"children_present": kids,
		"staff_present":    staff,
	})
}

func (h *KioskHandler) handleEvent(w http.ResponseWriter, r *http.Request, in bool) {
	pid, sessID, err := h.resolveKiosk(r)
	if err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	var body struct {
		Kind         string `json:"kind"`
		SubjectID    string `json:"subject_id"`
		PIN          string `json:"pin"`
		DroppedOffBy string `json:"dropped_off_by"`
		PickedUpBy   string `json:"picked_up_by"`
		Notes        string `json:"notes"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if body.SubjectID == "" || body.PIN == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("subject_id and pin required"))
		return
	}

	// Lockout check.
	if locked, retry := h.lockedOut(body.SubjectID); locked {
		w.Header().Set("Retry-After", retry.UTC().Format(time.RFC3339))
		httpx.RenderError(w, r, httpx.BadRequestf("Too many wrong PINs. Try again in a few minutes."))
		return
	}

	subjectKind := "child"
	if strings.HasPrefix(body.Kind, "staff_") {
		subjectKind = "staff"
	}

	// Verify PIN.
	if !h.verifyPIN(r, pid, subjectKind, body.SubjectID, body.PIN) {
		h.recordFailure(body.SubjectID)
		httpx.RenderError(w, r, httpx.BadRequestf("That PIN doesn't match what we have on file."))
		return
	}
	h.clearFailures(body.SubjectID)

	kind := body.Kind
	if kind == "" {
		if subjectKind == "child" && in {
			kind = "child_in"
		} else if subjectKind == "child" {
			kind = "child_out"
		} else if in {
			kind = "staff_in"
		} else {
			kind = "staff_out"
		}
	}

	// Children require a named adult on entry/exit.
	dropped := strings.TrimSpace(body.DroppedOffBy)
	picked := strings.TrimSpace(body.PickedUpBy)
	if subjectKind == "child" && in && dropped == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("Please type the name of the person dropping off."))
		return
	}
	if subjectKind == "child" && !in && picked == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("Please type the name of the person picking up."))
		return
	}

	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO attendance_events
			(provider_id, kind, subject_kind, subject_id, dropped_off_by, picked_up_by,
			 kiosk_session_id, notes)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''))`,
		pid, kind, subjectKind, body.SubjectID, dropped, picked, sessID, body.Notes,
	); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	_, _ = h.Pool.ExecContext(r.Context(), `UPDATE kiosk_sessions SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`, sessID)

	// Friendly response — shown briefly on the kiosk.
	name := h.lookupName(r, pid, subjectKind, body.SubjectID)
	verb := "Welcome back"
	if !in {
		verb = "See you tomorrow"
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"message":      verb + ", " + name + "!",
		"recorded_at":  time.Now().UTC().Format(time.RFC3339),
		"subject_kind": subjectKind,
	})
}

// RotateStaffPIN issues a new 4-digit PIN for a staff member. Returned in
// the response body once; never persisted in plaintext.
// POST /api/kiosk/pins/staff/{id}
func (h *KioskHandler) RotateStaffPIN(w http.ResponseWriter, r *http.Request) {
	h.rotatePIN(w, r, "staff")
}

// BulkRotatePINs is the Settings → Kiosk "regenerate one or all" surface.
// POST /api/settings/kiosk/rotate-pins  body: { "scope": "child"|"staff", "ids": ["..."]?  }
//
// If `ids` is empty/missing, ALL active subjects of that scope are rotated.
// Returns a map of subject_id -> new PIN. The caller is expected to render
// these to the operator once (so they can print the parent letters); we do
// not store the plaintext anywhere.
func (h *KioskHandler) BulkRotatePINs(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	var in struct {
		Scope string   `json:"scope"`
		IDs   []string `json:"ids"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	col := "kiosk_pin_hash"
	table := "staff"
	statusCol := "status"
	statusVal := "active"
	if in.Scope == "child" {
		col = "parent_pin_hash"
		table = "children"
		statusVal = "enrolled"
	} else if in.Scope != "staff" {
		httpx.RenderError(w, r, httpx.BadRequestf("scope must be child or staff"))
		return
	}

	// Collect target IDs.
	var ids []string
	if len(in.IDs) > 0 {
		ids = in.IDs
	} else {
		rows, err := h.Pool.QueryContext(r.Context(),
			"SELECT id FROM "+table+" WHERE provider_id = ? AND "+statusCol+" = ? AND deleted_at IS NULL", pid, statusVal)
		if err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		_ = rows.Close()
	}

	out := map[string]string{}
	for _, id := range ids {
		pin := generate4DigitPIN()
		hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
		if err != nil {
			continue
		}
		if _, err := h.Pool.ExecContext(r.Context(),
			"UPDATE "+table+" SET "+col+" = ? WHERE id = ? AND provider_id = ?",
			string(hash), id, pid); err == nil {
			out[id] = pin
		}
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"pins":  out,
		"hint":  "Show these once — Levelbrook does not store the digits, only a hash.",
		"count": len(out),
	})
}

// RotateChildPIN issues a new 4-digit parent PIN.
// POST /api/kiosk/pins/child/{id}
func (h *KioskHandler) RotateChildPIN(w http.ResponseWriter, r *http.Request) {
	h.rotatePIN(w, r, "child")
}

func (h *KioskHandler) rotatePIN(w http.ResponseWriter, r *http.Request, kind string) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if id == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("id required"))
		return
	}
	pin := generate4DigitPIN()
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	col := "kiosk_pin_hash"
	table := "staff"
	if kind == "child" {
		col = "parent_pin_hash"
		table = "children"
	}
	res, err := h.Pool.ExecContext(r.Context(),
		"UPDATE "+table+" SET "+col+" = ? WHERE id = ? AND provider_id = ?",
		string(hash), id, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"pin":  pin,
		"hint": "Show this once. We don't store the digits — only a hash.",
	})
}

// ---- internals ----

func (h *KioskHandler) resolveKiosk(r *http.Request) (providerID, sessionID string, err error) {
	id := r.Header.Get("X-Kiosk-Session")
	if id == "" {
		id = r.URL.Query().Get("ks")
	}
	if id == "" {
		return "", "", httpx.ErrUnauthorized
	}
	var pid string
	var exp string
	row := h.Pool.QueryRowContext(r.Context(), `
		SELECT provider_id, expires_at FROM kiosk_sessions WHERE id = ?`, id)
	if err := row.Scan(&pid, &exp); err != nil {
		return "", "", httpx.ErrUnauthorized
	}
	t, _ := time.Parse(time.RFC3339, exp)
	if t.IsZero() {
		t, _ = time.Parse("2006-01-02 15:04:05", exp)
	}
	if !t.IsZero() && time.Now().After(t) {
		return "", "", httpx.ErrUnauthorized
	}
	return pid, id, nil
}

func (h *KioskHandler) verifyPIN(r *http.Request, pid, kind, subjectID, pin string) bool {
	col := "kiosk_pin_hash"
	table := "staff"
	if kind == "child" {
		col = "parent_pin_hash"
		table = "children"
	}
	var hash sql.NullString
	if err := h.Pool.QueryRowContext(r.Context(),
		"SELECT "+col+" FROM "+table+" WHERE id = ? AND provider_id = ?",
		subjectID, pid).Scan(&hash); err != nil {
		return false
	}
	if !hash.Valid || hash.String == "" {
		// No PIN issued yet — allow ANY value during the rollout grace period
		// so the operator isn't locked out before they've issued PINs. The
		// dashboard nags them to issue PINs; once issued, this branch closes.
		return true
	}
	return bcrypt.CompareHashAndPassword([]byte(hash.String), []byte(pin)) == nil
}

func (h *KioskHandler) lookupName(r *http.Request, pid, kind, subjectID string) string {
	table := "staff"
	if kind == "child" {
		table = "children"
	}
	var first string
	_ = h.Pool.QueryRowContext(r.Context(),
		"SELECT COALESCE(first_name,'') FROM "+table+" WHERE id = ? AND provider_id = ?",
		subjectID, pid).Scan(&first)
	if first == "" {
		return "friend"
	}
	return first
}

func (h *KioskHandler) headcounts(r *http.Request, pid string) (kids, staff int) {
	// "Currently signed in" = most recent attendance event today is an `_in`.
	// The aggregate query joins on the latest event per subject.
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
		SELECT subject_kind, COUNT(*) FROM latest
		WHERE last_kind LIKE '%_in'
		GROUP BY subject_kind`, pid)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		_ = rows.Scan(&k, &n)
		if k == "child" {
			kids = n
		} else {
			staff = n
		}
	}
	return
}

func (h *KioskHandler) lockedOut(subjectID string) (bool, time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.attempts == nil {
		return false, time.Time{}
	}
	a, ok := h.attempts[subjectID]
	if !ok {
		return false, time.Time{}
	}
	if !a.lockedAt.IsZero() {
		until := a.lockedAt.Add(kioskLockoutTTL)
		if time.Now().Before(until) {
			return true, until
		}
		delete(h.attempts, subjectID)
	}
	return false, time.Time{}
}

func (h *KioskHandler) recordFailure(subjectID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.attempts == nil {
		h.attempts = map[string]*pinAttempt{}
	}
	a, ok := h.attempts[subjectID]
	now := time.Now()
	if !ok || now.Sub(a.first) > kioskAttemptWindow {
		h.attempts[subjectID] = &pinAttempt{count: 1, first: now}
		return
	}
	a.count++
	if a.count >= kioskMaxAttempts {
		a.lockedAt = now
	}
}

func (h *KioskHandler) clearFailures(subjectID string) {
	h.mu.Lock()
	delete(h.attempts, subjectID)
	h.mu.Unlock()
}

func generate4DigitPIN() string {
	id := base62.NewID()
	// Map base62 ID to 4 digits — deterministic but uniform enough for a 4-digit PIN.
	// Sum the byte values modulo 10000.
	sum := 0
	for i := 0; i < len(id); i++ {
		sum = (sum*31 + int(id[i])) & 0x7fffffff
	}
	d := sum % 10000
	out := []byte{'0', '0', '0', '0'}
	for i := 3; i >= 0; i-- {
		out[i] = byte('0' + d%10)
		d /= 10
	}
	return string(out)
}

// json import quiet — used by some tools when extending the handler.
var _ = json.Marshal
