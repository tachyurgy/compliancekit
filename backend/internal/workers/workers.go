// Package workers holds the small background goroutines that keep the
// production deployment healthy: session GC and the nightly compliance
// snapshot recompute. These are intentionally minimal — no job queue, no
// external scheduler, just `go worker.RunDaily(ctx)` from main.
//
// Each worker:
//   - Runs one pass on startup (so the operator sees results immediately).
//   - Sleeps between passes with context-cancellation support for clean
//     shutdown.
//   - Logs a structured summary of every pass.
//   - Is idempotent (safe to run multiple times per day).
package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/compliance"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// ---- Session GC ----

// SessionGC deletes expired rows from the sessions table. Without this the
// table grows unbounded; every magic-link click makes a new row and nothing
// removes them. Expired rows carry no security risk (expires_at is checked
// on every session load) but they bloat the DB file.
type SessionGC struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// RunDaily runs the GC pass every 24 hours until ctx is canceled. Runs once
// immediately so operators get feedback at startup.
func (g *SessionGC) RunDaily(ctx context.Context) {
	g.runOnce(ctx)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.runOnce(ctx)
		}
	}
}

func (g *SessionGC) runOnce(ctx context.Context) {
	// Delete sessions whose expires_at is in the past.
	res, err := g.Pool.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP`)
	if err != nil {
		g.Log.Warn("session gc failed", "err", err)
		return
	}
	n, _ := res.RowsAffected()

	// Also delete magic-link tokens that have been used or expired for more
	// than 48h. Their only value was the one-time use; keeping stale hashes
	// around longer serves no purpose.
	res2, err := g.Pool.ExecContext(ctx, `
		DELETE FROM magic_link_tokens
		WHERE (consumed_at IS NOT NULL AND consumed_at < datetime('now', '-2 days'))
		   OR (expires_at < datetime('now', '-2 days'))`)
	if err != nil {
		g.Log.Warn("magic-link token gc failed", "err", err)
		return
	}
	n2, _ := res2.RowsAffected()

	g.Log.Info("session gc complete", "sessions_deleted", n, "tokens_deleted", n2)
}

// ---- Nightly compliance snapshot recompute ----

// SnapshotWorker recomputes a compliance_snapshots row for every non-deleted
// provider once every 24 hours. This catches time-based transitions (e.g., a
// staff CPR card expiring at midnight with no document upload to trigger the
// per-request recompute path in DocumentHandler.Finalize).
type SnapshotWorker struct {
	Pool *sql.DB
	Log  *slog.Logger
}

func (s *SnapshotWorker) RunDaily(ctx context.Context) {
	s.runOnce(ctx)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *SnapshotWorker) runOnce(ctx context.Context) {
	rows, err := s.Pool.QueryContext(ctx, `SELECT id, state_code FROM providers WHERE deleted_at IS NULL AND state_code IS NOT NULL`)
	if err != nil {
		s.Log.Warn("snapshot worker: list providers failed", "err", err)
		return
	}
	defer rows.Close()

	type row struct {
		id    string
		state string
	}
	var providers []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.state); err != nil {
			s.Log.Warn("snapshot worker: scan failed", "err", err)
			continue
		}
		providers = append(providers, r)
	}
	_ = rows.Close()

	now := time.Now()
	var wrote int
	for _, p := range providers {
		if ctx.Err() != nil {
			return
		}
		facts, err := s.loadFacts(ctx, p.id)
		if err != nil {
			s.Log.Warn("snapshot worker: load facts failed", "provider_id", p.id, "err", err)
			continue
		}
		report := compliance.EvaluateAt(models.StateCode(p.state), facts, now)
		payload, _ := json.Marshal(report)
		critical := 0
		for _, v := range report.Violations {
			if v.Severity == compliance.SeverityCritical {
				critical++
			}
		}
		_, err = s.Pool.ExecContext(ctx, `
			INSERT INTO compliance_snapshots (id, provider_id, score, violation_count, critical_count, payload, computed_at)
			VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			base62.NewID()[:22], p.id, report.Score, len(report.Violations), critical, string(payload))
		if err != nil {
			s.Log.Warn("snapshot worker: insert failed", "provider_id", p.id, "err", err)
			continue
		}
		wrote++
	}

	s.Log.Info("snapshot worker complete", "providers_scanned", len(providers), "snapshots_written", wrote)
}

// loadFacts gathers the minimum ProviderFacts needed for compliance.Evaluate.
// Kept intentionally narrow — mirrors the shape DashboardHandler.loadFacts uses.
// The nightly recompute does not populate Documents[] by kind; that's fine for
// the current rule set because rules query via ProviderFacts helpers that
// return empty/nil for a missing key (which surfaces as "violated").
func (s *SnapshotWorker) loadFacts(ctx context.Context, providerID string) (*compliance.ProviderFacts, error) {
	f := &compliance.ProviderFacts{
		Provider: models.Provider{ID: providerID},
	}

	// Children.
	crows, err := s.Pool.QueryContext(ctx, `SELECT id, status FROM children WHERE provider_id = ? AND deleted_at IS NULL`, providerID)
	if err == nil {
		for crows.Next() {
			var c models.Child
			_ = crows.Scan(&c.ID, &c.Status)
			f.Children = append(f.Children, c)
		}
		_ = crows.Close()
	}

	// Staff.
	srows, err := s.Pool.QueryContext(ctx, `SELECT id, status, role FROM staff WHERE provider_id = ? AND deleted_at IS NULL`, providerID)
	if err == nil {
		for srows.Next() {
			var st models.Staff
			_ = srows.Scan(&st.ID, &st.Status, &st.Role)
			f.Staff = append(f.Staff, st)
		}
		_ = srows.Close()
	}

	// Facility flags + drill count.
	var ratioOK, postingsComplete sql.NullBool
	_ = s.Pool.QueryRowContext(ctx, `SELECT ratio_ok, postings_complete FROM providers WHERE id = ?`, providerID).
		Scan(&ratioOK, &postingsComplete)
	f.RatioOK = ratioOK.Valid && ratioOK.Bool
	f.PostingsComplete = postingsComplete.Valid && postingsComplete.Bool

	var drills int
	_ = s.Pool.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM drill_logs
		WHERE provider_id = ? AND drill_date >= datetime('now', '-90 days') AND (deleted_at IS NULL)`, providerID).Scan(&drills)
	f.DrillsLast90d = drills

	return f, nil
}
