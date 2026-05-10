package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/compliance"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// ScoreSnapshotWorker is REQ-013's once-per-day persister. It writes one row
// per active provider into compliance_score_snapshots so we have the
// time-series data the future sparkline UI will read.  Distinct from the
// existing SnapshotWorker which writes the *raw* engine Report into
// compliance_snapshots — this writer captures the synthesized 0-100 score
// after class-based weighting (see internal/compliance/score.go).
//
// Cadence: every 24h. The REQ asks for 03:00 UTC; we run "as soon as the
// process boots, then every 24h" because cron-style alignment isn't worth
// adding a scheduler dep for. The first-of-day write lands sometime in the
// 24h window; the calculated_at column is the source of truth for sparklines.
type ScoreSnapshotWorker struct {
	Pool *sql.DB
	Log  *slog.Logger
}

func (s *ScoreSnapshotWorker) RunDaily(ctx context.Context) {
	s.runOnce(ctx)
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runOnce(ctx)
		}
	}
}

func (s *ScoreSnapshotWorker) runOnce(ctx context.Context) {
	rows, err := s.Pool.QueryContext(ctx, `SELECT id, COALESCE(state_code, state, '') FROM providers WHERE deleted_at IS NULL`)
	if err != nil {
		s.Log.Warn("score snapshot: list providers failed", "err", err)
		return
	}
	type row struct{ id, state string }
	var providers []row
	for rows.Next() {
		var r row
		_ = rows.Scan(&r.id, &r.state)
		providers = append(providers, r)
	}
	_ = rows.Close()

	now := time.Now()
	wrote := 0
	for _, p := range providers {
		if ctx.Err() != nil {
			return
		}
		facts, err := loadFactsForSnapshot(ctx, s.Pool, p.id)
		if err != nil {
			s.Log.Warn("score snapshot: facts failed", "provider_id", p.id, "err", err)
			continue
		}
		report := compliance.EvaluateAt(models.StateCode(p.state), facts, now)
		synth := compliance.SynthesizeScore(report, facts, now)
		bd, _ := json.Marshal(synth.Breakdown)
		if _, err := s.Pool.ExecContext(ctx, `
			INSERT INTO compliance_score_snapshots (provider_id, score, band, breakdown_json, calculated_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`, p.id, synth.Score, synth.Band, string(bd)); err != nil {
			s.Log.Warn("score snapshot: insert failed", "provider_id", p.id, "err", err)
			continue
		}
		wrote++
	}
	s.Log.Info("score snapshot complete", "providers", len(providers), "written", wrote)
}

// loadFactsForSnapshot mirrors the slim loadFacts shape the SnapshotWorker
// uses.  Kept package-internal so we don't import handlers from workers.
func loadFactsForSnapshot(ctx context.Context, pool *sql.DB, pid string) (*compliance.ProviderFacts, error) {
	f := &compliance.ProviderFacts{
		Provider:  models.Provider{ID: pid},
		Documents: map[string][]models.Document{},
	}

	crows, err := pool.QueryContext(ctx, `SELECT id, COALESCE(status,'enrolled') FROM children WHERE provider_id = ? AND deleted_at IS NULL`, pid)
	if err == nil {
		for crows.Next() {
			var c models.Child
			_ = crows.Scan(&c.ID, &c.Status)
			f.Children = append(f.Children, c)
		}
		_ = crows.Close()
	}

	srows, err := pool.QueryContext(ctx, `SELECT id, COALESCE(status,'active'), COALESCE(role,'') FROM staff WHERE provider_id = ? AND deleted_at IS NULL`, pid)
	if err == nil {
		for srows.Next() {
			var s models.Staff
			_ = srows.Scan(&s.ID, &s.Status, &s.Role)
			f.Staff = append(f.Staff, s)
		}
		_ = srows.Close()
	}

	drows, err := pool.QueryContext(ctx, `
		SELECT COALESCE(subject_kind, owner_kind, ''),
		       COALESCE(subject_id, owner_id, ''),
		       COALESCE(kind, doc_type, ''),
		       expiration_date
		FROM documents WHERE provider_id = ? AND deleted_at IS NULL`, pid)
	if err == nil {
		for drows.Next() {
			var d models.Document
			var kind string
			var exp sql.NullTime
			_ = drows.Scan(&d.SubjectKind, &d.SubjectID, &kind, &exp)
			d.Kind = models.DocumentKind(kind)
			if exp.Valid {
				d.ExpiresAt = &exp.Time
			}
			key := d.SubjectKind + "|" + d.SubjectID + "|" + kind
			f.Documents[key] = append(f.Documents[key], d)
		}
		_ = drows.Close()
	}

	var ratioOK, postingsOK sql.NullBool
	_ = pool.QueryRowContext(ctx,
		`SELECT COALESCE(ratio_ok,1), COALESCE(postings_complete,0) FROM providers WHERE id = ?`, pid).
		Scan(&ratioOK, &postingsOK)
	f.RatioOK = ratioOK.Valid && ratioOK.Bool
	f.PostingsComplete = postingsOK.Valid && postingsOK.Bool
	var d int
	_ = pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM drill_logs WHERE provider_id = ? AND deleted_at IS NULL AND drill_date > datetime('now','-90 days')`, pid).Scan(&d)
	f.DrillsLast90d = d
	return f, nil
}
