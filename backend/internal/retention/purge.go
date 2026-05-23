// Package retention implements the 90-day data purge worker.
//
// When a provider cancels their subscription (Stripe webhook → subscriptions.status='canceled',
// providers.canceled_at set) OR explicitly requests account deletion (providers.deleted_at set),
// a 90-day grace period begins. During the grace period the tenant can re-subscribe and their
// data is restored instantly. Once the grace window elapses we:
//
//  1. Write a deletion manifest JSON to S3 at audit/{provider_id}/deletion-{unix}.json describing
//     everything we are about to delete (row counts by table, S3 prefixes, owner email, timestamps).
//     This satisfies a 7-year audit retention obligation even after the tenant itself is gone.
//  2. Delete all tenant objects in S3 under providers/{id}/. (See storage.Client.DeleteAllForProvider.)
//     The audit/ prefix containing the manifest is NOT cleared, so the manifest survives.
//  3. Delete the tenant's rows from the DB, in dependency order. Where foreign keys are
//     ON DELETE CASCADE we still issue explicit deletes first — the cascade depth in SQLite
//     can blow the stack for a dense tenant, and being explicit makes the per-table counts
//     in the manifest trustworthy.
//  4. Emit a retention.purged row into audit_log BEFORE the providers row itself is removed.
//     audit_log.provider_id has ON DELETE SET NULL (see migration 000008), so the row
//     survives — anonymized — for the 7-year legal hold.
//
// The worker is idempotent: a provider that is already purged simply has no rows to delete,
// so re-running RunOnce is safe.
package retention

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

// GracePeriod is how long a canceled or deleted provider's data sits in cold
// storage before the purge worker hard-deletes it. 90 days is the Stripe
// industry default and also the period promised in our Terms of Service.
const GracePeriod = 90 * 24 * time.Hour

// PurgeWorker runs the daily retention sweep.
type PurgeWorker struct {
	pool *sql.DB
	s3   *storage.Client
	log  *slog.Logger
}

// NewPurgeWorker constructs a worker. s3 may be nil in tests that only want to
// exercise the DB side; in that case S3 deletes and manifest uploads are skipped.
func NewPurgeWorker(pool *sql.DB, s3 *storage.Client, log *slog.Logger) *PurgeWorker {
	if log == nil {
		log = slog.Default()
	}
	return &PurgeWorker{pool: pool, s3: s3, log: log}
}

// RunDaily blocks until ctx is done, invoking RunOnce approximately every 24h.
// It also runs once immediately on start so a fresh deploy doesn't wait a day.
func (w *PurgeWorker) RunDaily(ctx context.Context) {
	if w == nil || w.pool == nil {
		return
	}
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	if err := w.RunOnce(ctx, time.Now()); err != nil {
		w.log.Error("retention: initial run failed", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if err := w.RunOnce(ctx, now); err != nil {
				w.log.Error("retention: run failed", "err", err)
			}
		}
	}
}

// candidate is a provider eligible for purge at the current tick. Timestamps
// come off SQLite as strings (ISO-8601) because modernc.org/sqlite scans TEXT
// columns into string, not time.Time — we preserve the raw string in the
// deletion manifest for auditability.
type candidate struct {
	ID         string
	OwnerEmail sql.NullString
	CanceledAt sql.NullString
	DeletedAt  sql.NullString
}

// RunOnce executes a single purge pass. Exposed so tests can drive it with a
// fixed `now`, and so RunDaily can share the same code path.
func (w *PurgeWorker) RunOnce(ctx context.Context, now time.Time) error {
	if w == nil || w.pool == nil {
		return errors.New("retention: not initialized")
	}
	cutoff := now.Add(-GracePeriod)

	// A provider is eligible if EITHER:
	//  - they have a canceled subscription AND canceled_at is more than 90 days ago, OR
	//  - their providers.deleted_at is more than 90 days ago (explicit deletion request).
	//
	// We LEFT JOIN subscriptions so providers without a subscription row (e.g. an
	// owner who signed up but never converted) can still be purged via deleted_at.
	rows, err := w.pool.QueryContext(ctx, `
SELECT DISTINCT p.id, COALESCE(p.owner_email, ''), p.canceled_at, p.deleted_at
  FROM providers p
  LEFT JOIN subscriptions s ON s.provider_id = p.id
 WHERE (s.status = 'canceled' AND p.canceled_at IS NOT NULL AND p.canceled_at < ?)
    OR (p.deleted_at IS NOT NULL AND p.deleted_at < ?)`, cutoff, cutoff)
	if err != nil {
		return fmt.Errorf("retention: query candidates: %w", err)
	}
	defer rows.Close()

	var batch []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.ID, &c.OwnerEmail, &c.CanceledAt, &c.DeletedAt); err != nil {
			return fmt.Errorf("retention: scan: %w", err)
		}
		batch = append(batch, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("retention: iterate: %w", err)
	}

	w.log.Info("retention: sweep", "candidates", len(batch), "cutoff", cutoff)

	for _, c := range batch {
		if err := w.purgeProvider(ctx, c, now); err != nil {
			w.log.Error("retention: purge failed", "provider_id", c.ID, "err", err)
			continue
		}
		w.log.Info("retention: purged", "provider_id", c.ID)
	}
	return nil
}

// purgeProvider runs the full deletion sequence for a single tenant.
func (w *PurgeWorker) purgeProvider(ctx context.Context, c candidate, now time.Time) error {
	// 1) Gather per-table row counts BEFORE deleting so the manifest is accurate.
	counts, err := w.gatherCounts(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("gather counts: %w", err)
	}

	manifest := DeletionManifest{
		ProviderID:        c.ID,
		OwnerEmail:        c.OwnerEmail.String,
		CanceledAt:        c.CanceledAt.String,
		DeletedAt:         c.DeletedAt.String,
		PurgedAt:          now.UTC().Format(time.RFC3339),
		RowCounts:         counts,
		DeletedS3Prefixes: []string{"providers/" + c.ID + "/"},
		Note:              "ComplianceKit 90-day retention: tenant data purged. Audit log rows retained (anonymized) per legal hold.",
	}

	// 2) Upload manifest to audit/{provider_id}/deletion-{ts}.json before anything
	// destructive happens, so even a mid-purge crash leaves forensic evidence.
	manifestKey := fmt.Sprintf("audit/%s/deletion-%d.json", c.ID, now.Unix())
	if w.s3 != nil {
		if err := w.s3.PutAuditJSON(ctx, manifestKey, manifest); err != nil {
			return fmt.Errorf("upload manifest: %w", err)
		}
	}

	// 3) Emit the audit_log row BEFORE deleting the provider, because after
	// the providers row is gone audit_log.provider_id will be SET NULL and
	// cross-referencing a purge by provider_id becomes impossible.
	if err := EmitProviderPurge(ctx, w.pool, c.ID, manifestKey, manifest); err != nil {
		return fmt.Errorf("emit audit: %w", err)
	}

	// 4) Delete S3 objects under providers/<id>/ across all buckets (documents,
	// signed, audit, raw uploads). The manifest we just wrote lives under
	// audit/<id>/, which does NOT match the prefix, so it is preserved.
	if w.s3 != nil {
		if err := w.s3.DeleteAllForProvider(ctx, c.ID); err != nil {
			return fmt.Errorf("delete s3 objects: %w", err)
		}
	}

	// 5) Delete DB rows in dependency order. Most are already ON DELETE CASCADE
	// from providers, but being explicit gives us predictable behavior on SQLite
	// (which has a finite recursive-trigger depth) and a truthful count pre-purge.
	if err := w.deleteRows(ctx, c.ID); err != nil {
		return fmt.Errorf("delete rows: %w", err)
	}
	return nil
}

// tablesWithProviderFK is the list of tables we sweep. Order matters: delete
// leaf tables (those that FK to other tenant rows) before their parents.
// Tables not listed either FK-cascade from providers directly (we let the
// `DELETE FROM providers` at the end clean them up) or don't carry a provider_id.
var tablesWithProviderFK = []struct {
	Name string
	SQL  string
}{
	// Inspection responses live under runs which live under the provider; delete
	// responses first so the run-id lookups are clean.
	{"inspection_responses", `DELETE FROM inspection_responses WHERE run_id IN (SELECT id FROM inspection_runs WHERE provider_id = ?)`},
	{"inspection_runs", `DELETE FROM inspection_runs WHERE provider_id = ?`},

	// Per-child / per-staff dependents.
	{"child_documents_required", `DELETE FROM child_documents_required WHERE child_id IN (SELECT id FROM children WHERE provider_id = ?)`},
	{"staff_certifications_required", `DELETE FROM staff_certifications_required WHERE staff_id IN (SELECT id FROM staff WHERE provider_id = ?)`},

	// Document dependents.
	{"document_ocr_results", `DELETE FROM document_ocr_results WHERE document_id IN (SELECT id FROM documents WHERE provider_id = ?)`},
	{"document_chase_sends", `DELETE FROM document_chase_sends WHERE document_id IN (SELECT id FROM documents WHERE provider_id = ?)`},
	{"signatures", `DELETE FROM signatures WHERE document_id IN (SELECT id FROM documents WHERE provider_id = ?)`},
	{"sign_sessions", `DELETE FROM sign_sessions WHERE document_id IN (SELECT id FROM documents WHERE provider_id = ?) OR document_template_id IN (SELECT id FROM document_templates WHERE provider_id = ?)`},
	{"document_unassigned_photos", `DELETE FROM document_unassigned_photos WHERE provider_id = ?`},
	{"document_templates", `DELETE FROM document_templates WHERE provider_id = ?`},
	{"documents", `DELETE FROM documents WHERE provider_id = ?`},

	// Top-level tenant rows.
	{"drill_logs", `DELETE FROM drill_logs WHERE provider_id = ?`},
	{"children", `DELETE FROM children WHERE provider_id = ?`},
	{"staff", `DELETE FROM staff WHERE provider_id = ?`},
	{"compliance_snapshots", `DELETE FROM compliance_snapshots WHERE provider_id = ?`},
	{"chase_events", `DELETE FROM chase_events WHERE provider_id = ?`},
	{"outgoing_messages", `DELETE FROM outgoing_messages WHERE provider_id = ?`},
	{"data_exports", `DELETE FROM data_exports WHERE provider_id = ?`},
	{"sessions", `DELETE FROM sessions WHERE provider_id = ?`},
	{"subscriptions", `DELETE FROM subscriptions WHERE provider_id = ?`},
	{"users", `DELETE FROM users WHERE provider_id = ?`},
	{"providers", `DELETE FROM providers WHERE id = ?`},
}

// countQueries is the set of per-tenant row-count queries used to build the
// deletion manifest. Using the same predicates as tablesWithProviderFK keeps
// the counts consistent with what we're about to delete.
var countQueries = []struct {
	Key string
	SQL string
}{
	{"children", `SELECT COUNT(*) FROM children WHERE provider_id = ?`},
	{"staff", `SELECT COUNT(*) FROM staff WHERE provider_id = ?`},
	{"documents", `SELECT COUNT(*) FROM documents WHERE provider_id = ?`},
	{"drill_logs", `SELECT COUNT(*) FROM drill_logs WHERE provider_id = ?`},
	{"inspection_runs", `SELECT COUNT(*) FROM inspection_runs WHERE provider_id = ?`},
	{"compliance_snapshots", `SELECT COUNT(*) FROM compliance_snapshots WHERE provider_id = ?`},
	{"chase_events", `SELECT COUNT(*) FROM chase_events WHERE provider_id = ?`},
	{"signatures", `SELECT COUNT(*) FROM signatures WHERE document_id IN (SELECT id FROM documents WHERE provider_id = ?)`},
	{"sign_sessions", `SELECT COUNT(*) FROM sign_sessions WHERE document_id IN (SELECT id FROM documents WHERE provider_id = ?) OR document_template_id IN (SELECT id FROM document_templates WHERE provider_id = ?)`},
	{"users", `SELECT COUNT(*) FROM users WHERE provider_id = ?`},
}

func (w *PurgeWorker) gatherCounts(ctx context.Context, pid string) (map[string]int, error) {
	out := make(map[string]int, len(countQueries))
	for _, q := range countQueries {
		var n int
		var err error
		// sign_sessions predicate has two ? placeholders bound to the same pid.
		if q.Key == "sign_sessions" {
			err = w.pool.QueryRowContext(ctx, q.SQL, pid, pid).Scan(&n)
		} else {
			err = w.pool.QueryRowContext(ctx, q.SQL, pid).Scan(&n)
		}
		if err != nil {
			return nil, fmt.Errorf("count %s: %w", q.Key, err)
		}
		out[q.Key] = n
	}
	return out, nil
}

func (w *PurgeWorker) deleteRows(ctx context.Context, pid string) error {
	for _, t := range tablesWithProviderFK {
		var err error
		switch t.Name {
		case "sign_sessions":
			_, err = w.pool.ExecContext(ctx, t.SQL, pid, pid)
		case "providers":
			_, err = w.pool.ExecContext(ctx, t.SQL, pid)
		default:
			_, err = w.pool.ExecContext(ctx, t.SQL, pid)
		}
		if err != nil {
			return fmt.Errorf("delete %s: %w", t.Name, err)
		}
	}
	return nil
}

// DeletionManifest is the JSON blob written to S3 as the tombstone for a purged tenant.
type DeletionManifest struct {
	ProviderID        string         `json:"provider_id"`
	OwnerEmail        string         `json:"owner_email,omitempty"`
	CanceledAt        string         `json:"canceled_at,omitempty"`
	DeletedAt         string         `json:"deleted_at,omitempty"`
	PurgedAt          string         `json:"purged_at"`
	RowCounts         map[string]int `json:"row_counts"`
	DeletedS3Prefixes []string       `json:"deleted_s3_prefixes"`
	Note              string         `json:"note,omitempty"`
}

// EmitProviderPurge writes a single retention.purged row to audit_log via the
// shared auditlog package. Called by the worker BEFORE the providers row is
// removed so the FK link is live at insert time; after the provider delete
// the provider_id column becomes NULL via ON DELETE SET NULL and the row
// stays for the 7-year legal hold.
func EmitProviderPurge(ctx context.Context, pool *sql.DB, providerID, manifestKey string, manifest DeletionManifest) error {
	meta := map[string]any{
		"manifest_s3_key":     manifestKey,
		"row_counts":          manifest.RowCounts,
		"deleted_s3_prefixes": manifest.DeletedS3Prefixes,
		"purged_at":           manifest.PurgedAt,
	}
	return auditlog.Emit(ctx, pool, auditlog.Entry{
		ProviderID: providerID,
		ActorKind:  auditlog.ActorKindSystem,
		ActorID:    "retention_worker",
		Action:     "retention.purged",
		TargetKind: auditlog.TargetKindProvider,
		TargetID:   providerID,
		Metadata:   meta,
	})
}
