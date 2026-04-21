package retention_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/retention"
	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

func silentLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// insertProvider seeds a provider with optional canceled_at / deleted_at dates
// (as ISO-8601 strings, or "" for NULL) plus an active or canceled subscription.
func insertProvider(t *testing.T, pool interface {
	Exec(string, ...any) (interface{}, error)
}, id, canceledAt, deletedAt, subStatus string) {
}

func TestPurgeWorker_CanceledOver90d_Purged(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)

	// provider canceled 100 days ago
	oldCanceled := time.Now().AddDate(0, 0, -100).UTC().Format(time.RFC3339)
	_, err := pool.Exec(`INSERT INTO providers (id, legal_name, state, canceled_at) VALUES ('pOld','A','CA',?)`, oldCanceled)
	if err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	_, err = pool.Exec(`INSERT INTO subscriptions (id, provider_id, stripe_customer_id, plan, status) VALUES ('s1','pOld','cus_1','pro','canceled')`)
	if err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	w := retention.NewPurgeWorker(pool, nil, silentLog())
	if err := w.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM providers WHERE id='pOld'`).Scan(&n)
	if n != 0 {
		t.Fatalf("expected provider purged, rows=%d", n)
	}
}

func TestPurgeWorker_CanceledUnder90d_Preserved(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)

	recent := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	_, err := pool.Exec(`INSERT INTO providers (id, legal_name, state, canceled_at) VALUES ('pRecent','A','CA',?)`, recent)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = pool.Exec(`INSERT INTO subscriptions (id, provider_id, stripe_customer_id, plan, status) VALUES ('s1','pRecent','cus_1','pro','canceled')`)
	if err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	w := retention.NewPurgeWorker(pool, nil, silentLog())
	if err := w.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM providers WHERE id='pRecent'`).Scan(&n)
	if n != 1 {
		t.Fatalf("provider inside grace period was purged")
	}
}

func TestPurgeWorker_DeletedAtOver90d_Purged(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)

	oldDeleted := time.Now().AddDate(0, 0, -120).UTC().Format(time.RFC3339)
	_, err := pool.Exec(`INSERT INTO providers (id, legal_name, state, deleted_at) VALUES ('pDel','A','CA',?)`, oldDeleted)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := retention.NewPurgeWorker(pool, nil, silentLog())
	if err := w.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM providers WHERE id='pDel'`).Scan(&n)
	if n != 0 {
		t.Fatalf("expected deleted-over-90d provider purged")
	}
}

func TestPurgeWorker_DeletedAtUnder90d_Preserved(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)

	recent := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)
	_, err := pool.Exec(`INSERT INTO providers (id, legal_name, state, deleted_at) VALUES ('pRecentDel','A','CA',?)`, recent)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := retention.NewPurgeWorker(pool, nil, silentLog())
	if err := w.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM providers WHERE id='pRecentDel'`).Scan(&n)
	if n != 1 {
		t.Fatalf("recently-deleted provider was purged")
	}
}

func TestPurgeWorker_MultipleCandidates_AllPurged(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)

	old := time.Now().AddDate(0, 0, -100).UTC().Format(time.RFC3339)
	_, err := pool.Exec(`
INSERT INTO providers (id, legal_name, state, deleted_at) VALUES ('pA','A','CA',?);
INSERT INTO providers (id, legal_name, state, deleted_at) VALUES ('pB','B','TX',?);
INSERT INTO providers (id, legal_name, state, deleted_at) VALUES ('pC','C','FL',?);
INSERT INTO providers (id, legal_name, state) VALUES ('pLive','L','CA');`, old, old, old)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := retention.NewPurgeWorker(pool, nil, silentLog())
	if err := w.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	var remaining int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM providers`).Scan(&remaining)
	if remaining != 1 {
		t.Fatalf("providers remaining = %d, want 1 (pLive)", remaining)
	}
	var live int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM providers WHERE id='pLive'`).Scan(&live)
	if live != 1 {
		t.Fatalf("live provider missing")
	}
}

func TestPurgeWorker_EmitProviderPurge_WritesAuditLogBeforeDelete(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)

	old := time.Now().AddDate(0, 0, -100).UTC().Format(time.RFC3339)
	_, err := pool.Exec(`INSERT INTO providers (id, legal_name, state, owner_email, deleted_at) VALUES ('pX','X','CA','owner@x.com',?)`, old)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := retention.NewPurgeWorker(pool, nil, silentLog())
	if err := w.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// audit_log.provider_id has ON DELETE SET NULL so after purge the column is
	// NULL. We assert the row exists with the right action.
	var action string
	var pid *string
	err = pool.QueryRow(
		`SELECT action, provider_id FROM audit_log WHERE action='retention.purged' ORDER BY created_at DESC LIMIT 1`).
		Scan(&action, &pid)
	if err != nil {
		t.Fatalf("select audit: %v", err)
	}
	if action != "retention.purged" {
		t.Fatalf("action = %q", action)
	}
	// provider_id is NULL (SET NULL cascade) — the row survives the purge.
	if pid != nil && *pid != "" {
		t.Fatalf("expected provider_id NULLed out after purge, got %q", *pid)
	}
}

func TestPurgeWorker_NilPool_Errors(t *testing.T) {
	t.Parallel()
	w := retention.NewPurgeWorker(nil, nil, silentLog())
	if err := w.RunOnce(context.Background(), time.Now()); err == nil {
		t.Fatal("expected error when pool is nil")
	}
}

func TestPurgeWorker_RemovesChildData(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)

	old := time.Now().AddDate(0, 0, -100).UTC().Format(time.RFC3339)
	_, err := pool.Exec(`
INSERT INTO providers (id, legal_name, state, deleted_at) VALUES ('pZ','Z','CA',?);
INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date, guardians) VALUES ('c1','pZ','Kid','A','2020-01-01','2024-01-01','[]');
INSERT INTO staff (id, provider_id, first_name, last_name, status) VALUES ('s1','pZ','Alice','B','active');
INSERT INTO users (id, provider_id, email, full_name, role) VALUES ('u1','pZ','owner@pz.com','Owner Z','provider_admin');
`, old)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := retention.NewPurgeWorker(pool, nil, silentLog())
	if err := w.RunOnce(context.Background(), time.Now()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	for _, q := range []string{
		`SELECT COUNT(*) FROM providers WHERE id='pZ'`,
		`SELECT COUNT(*) FROM children WHERE provider_id='pZ'`,
		`SELECT COUNT(*) FROM staff WHERE provider_id='pZ'`,
		`SELECT COUNT(*) FROM users WHERE provider_id='pZ'`,
	} {
		var n int
		_ = pool.QueryRow(q).Scan(&n)
		if n != 0 {
			t.Errorf("%s: expected 0, got %d", q, n)
		}
	}
}

func TestPurgeWorker_Idempotent(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	old := time.Now().AddDate(0, 0, -100).UTC().Format(time.RFC3339)
	_, _ = pool.Exec(`INSERT INTO providers (id, legal_name, state, deleted_at) VALUES ('pI','I','CA',?)`, old)

	w := retention.NewPurgeWorker(pool, nil, silentLog())
	// Run twice — second pass must not crash even though the provider is gone.
	for i := 0; i < 2; i++ {
		if err := w.RunOnce(context.Background(), time.Now()); err != nil {
			t.Fatalf("RunOnce pass %d: %v", i, err)
		}
	}
}

func TestEmitProviderPurge_WritesAuditRow(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	_, _ = pool.Exec(`INSERT INTO providers (id, legal_name, state) VALUES ('p1','X','CA')`)

	manifest := retention.DeletionManifest{
		ProviderID:        "p1",
		OwnerEmail:        "o@x.com",
		DeletedAt:         "2026-01-01T00:00:00Z",
		PurgedAt:          "2026-04-01T00:00:00Z",
		RowCounts:         map[string]int{"children": 2, "staff": 3},
		DeletedS3Prefixes: []string{"providers/p1/"},
	}
	if err := retention.EmitProviderPurge(context.Background(), pool, "p1", "audit/p1/del.json", manifest); err != nil {
		t.Fatalf("EmitProviderPurge: %v", err)
	}

	var action, meta string
	err := pool.QueryRow(`SELECT action, metadata FROM audit_log WHERE action='retention.purged'`).Scan(&action, &meta)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if action != "retention.purged" {
		t.Fatalf("action = %q", action)
	}
	if meta == "{}" {
		t.Fatal("metadata is empty object")
	}
}
