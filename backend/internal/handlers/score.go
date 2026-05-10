package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/compliance"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// ScoreHandler implements REQ-013: GET /api/compliance/score and the daily
// snapshot history endpoint.
//
// Caching: in-process, 60s TTL keyed by provider_id. Bust the cache on any
// document upload, staff/child mutation, or drill-log write — those handlers
// call BustCache(providerID).  60s keeps the dashboard cheap on the cx22
// droplet without making freshly-uploaded docs feel stuck.
type ScoreHandler struct {
	Pool *sql.DB
	Log  *slog.Logger

	mu    sync.RWMutex
	cache map[string]scoreCacheEntry
}

type scoreCacheEntry struct {
	at    time.Time
	synth *compliance.Synth
}

const scoreCacheTTL = 60 * time.Second

// BustCache evicts the cached entry for a provider. Call from any write path
// that could move the score (document finalize, staff/child create, drill log).
func (h *ScoreHandler) BustCache(providerID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.cache, providerID)
	h.mu.Unlock()
}

// Get is GET /api/compliance/score.
//
// Returns the synthesised dashboard score, the four highest-impact gaps,
// and the per-category breakdown.
func (h *ScoreHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}

	if cached, ok := h.cached(pid); ok {
		httpx.RenderJSON(w, http.StatusOK, cached)
		return
	}

	synth, err := h.compute(r.Context(), pid)
	if err != nil {
		// Degrade gracefully — never show a 0 or "--". Try the most recent
		// snapshot from the database; if even that fails, return 503.
		if cached := h.lastSnapshotSynth(r.Context(), pid); cached != nil {
			h.Log.Warn("score: live compute failed, returning last snapshot", "err", err, "provider_id", pid)
			httpx.RenderJSON(w, http.StatusOK, cached)
			return
		}
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	h.store(pid, synth)
	httpx.RenderJSON(w, http.StatusOK, synth)
}

// History is GET /api/compliance/score/history?days=30.
//
// Returns the daily snapshots for the trailing N days. Used by the (future)
// sparkline; ship the data path now so we have history when the UI lands.
func (h *ScoreHandler) History(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}

	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT score, band, calculated_at
		FROM compliance_score_snapshots
		WHERE provider_id = ?
		  AND calculated_at >= datetime('now', ?)
		ORDER BY calculated_at ASC`, pid, "-"+strconv.Itoa(days)+" days")
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()

	type point struct {
		Score        int    `json:"score"`
		Band         string `json:"band"`
		CalculatedAt string `json:"calculated_at"`
	}
	out := []point{}
	for rows.Next() {
		var p point
		if err := rows.Scan(&p.Score, &p.Band, &p.CalculatedAt); err == nil {
			out = append(out, p)
		}
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"history": out, "days": days})
}

// ---- internals ----

func (h *ScoreHandler) cached(providerID string) (*compliance.Synth, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.cache == nil {
		return nil, false
	}
	e, ok := h.cache[providerID]
	if !ok {
		return nil, false
	}
	if time.Since(e.at) > scoreCacheTTL {
		return nil, false
	}
	return e.synth, true
}

func (h *ScoreHandler) store(providerID string, synth *compliance.Synth) {
	h.mu.Lock()
	if h.cache == nil {
		h.cache = make(map[string]scoreCacheEntry)
	}
	h.cache[providerID] = scoreCacheEntry{at: time.Now(), synth: synth}
	h.mu.Unlock()
}

// compute runs the engine against current DB state and returns the synth.
// It re-uses the dashboard's loadFacts shape via a small bespoke loader so we
// don't couple ScoreHandler to DashboardHandler.
func (h *ScoreHandler) compute(ctx context.Context, providerID string) (*compliance.Synth, error) {
	facts, state, err := loadProviderFacts(ctx, h.Pool, providerID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	report := compliance.EvaluateAt(state, facts, now)
	return compliance.SynthesizeScore(report, facts, now), nil
}

// lastSnapshotSynth returns the most recent persisted score as a Synth.
// Used as a graceful-degradation fallback when live compute fails.
func (h *ScoreHandler) lastSnapshotSynth(ctx context.Context, providerID string) *compliance.Synth {
	var score int
	var band, calcAt, breakdown string
	err := h.Pool.QueryRowContext(ctx, `
		SELECT score, band, breakdown_json, calculated_at
		FROM compliance_score_snapshots
		WHERE provider_id = ?
		ORDER BY calculated_at DESC LIMIT 1`, providerID).Scan(&score, &band, &breakdown, &calcAt)
	if err != nil {
		return nil
	}
	t, _ := time.Parse("2006-01-02 15:04:05", calcAt)
	rows := []compliance.BreakdownRow{}
	_ = json.Unmarshal([]byte(breakdown), &rows)
	return &compliance.Synth{
		Score:            score,
		Band:             band,
		LastCalculatedAt: t,
		TopGaps:          nil,
		Breakdown:        rows,
	}
}

// loadProviderFacts is a slim sibling of DashboardHandler.loadFacts. Kept
// here so ScoreHandler can compute without importing handlers ←→ handlers.
// Returns the same shape: facts + state.
func loadProviderFacts(ctx context.Context, pool *sql.DB, pid string) (*compliance.ProviderFacts, models.StateCode, error) {
	var p models.Provider
	var stateStr string
	if err := pool.QueryRowContext(ctx, `
		SELECT id, COALESCE(name, legal_name, '') AS name,
		       COALESCE(state_code, state, '') AS state_code,
		       COALESCE(license_number,''), COALESCE(owner_email,''),
		       COALESCE(capacity, 0), COALESCE(timezone,'')
		FROM providers WHERE id = ?`, pid).Scan(
		&p.ID, &p.Name, &stateStr, &p.LicenseNumber, &p.OwnerEmail, &p.Capacity, &p.Timezone,
	); err != nil {
		return nil, "", err
	}
	p.StateCode = models.StateCode(stateStr)

	facts := &compliance.ProviderFacts{Provider: p, Documents: map[string][]models.Document{}}

	// Children
	crows, err := pool.QueryContext(ctx, `SELECT id, COALESCE(status,'enrolled') FROM children WHERE provider_id = ? AND deleted_at IS NULL`, pid)
	if err == nil {
		for crows.Next() {
			var c models.Child
			_ = crows.Scan(&c.ID, &c.Status)
			c.ProviderID = pid
			facts.Children = append(facts.Children, c)
		}
		_ = crows.Close()
	}

	// Staff
	srows, err := pool.QueryContext(ctx, `SELECT id, COALESCE(status,'active'), COALESCE(role,'') FROM staff WHERE provider_id = ? AND deleted_at IS NULL`, pid)
	if err == nil {
		for srows.Next() {
			var s models.Staff
			_ = srows.Scan(&s.ID, &s.Status, &s.Role)
			s.ProviderID = pid
			facts.Staff = append(facts.Staff, s)
		}
		_ = srows.Close()
	}

	// Documents — only the columns the engine helpers read.
	drows, err := pool.QueryContext(ctx, `
		SELECT id,
		       COALESCE(subject_kind, owner_kind, '') AS subject_kind,
		       COALESCE(subject_id, owner_id, '') AS subject_id,
		       COALESCE(kind, doc_type, '') AS kind,
		       expiration_date AS expires_at
		FROM documents WHERE provider_id = ? AND deleted_at IS NULL`, pid)
	if err == nil {
		for drows.Next() {
			var d models.Document
			var kind string
			var exp sql.NullTime
			_ = drows.Scan(&d.ID, &d.SubjectKind, &d.SubjectID, &kind, &exp)
			d.Kind = models.DocumentKind(kind)
			d.ProviderID = pid
			if exp.Valid {
				d.ExpiresAt = &exp.Time
			}
			key := d.SubjectKind + "|" + d.SubjectID + "|" + kind
			facts.Documents[key] = append(facts.Documents[key], d)
		}
		_ = drows.Close()
	}

	// Drill count + facility flags.
	var ratioOK, postingsOK sql.NullBool
	_ = pool.QueryRowContext(ctx,
		`SELECT COALESCE(ratio_ok,1), COALESCE(postings_complete,0) FROM providers WHERE id = ?`, pid).Scan(&ratioOK, &postingsOK)
	facts.RatioOK = ratioOK.Valid && ratioOK.Bool
	facts.PostingsComplete = postingsOK.Valid && postingsOK.Bool
	var drills int
	_ = pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM drill_logs WHERE provider_id = ? AND deleted_at IS NULL AND drill_date > datetime('now','-90 days')`, pid).Scan(&drills)
	facts.DrillsLast90d = drills

	return facts, p.StateCode, nil
}
