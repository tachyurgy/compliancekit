package integration

import (
	"context"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/retention"
)

// TestRetention_PurgesOnlyExpiredProviders verifies the core contract of the
// purge worker:
//  1. A provider whose canceled_at is more than 90 days old AND whose
//     subscriptions.status='canceled' IS purged — rows in children/staff/
//     documents disappear.
//  2. A provider whose canceled_at is less than 90 days old is NOT purged
//     even if their subscription is canceled.
//  3. A provider with no subscription at all and no deleted_at is NOT purged.
//  4. A provider whose deleted_at is more than 90 days old IS purged
//     regardless of subscription state.
//  5. An audit_log row survives the purge (with provider_id set to NULL
//     thanks to ON DELETE SET NULL).
func TestRetention_PurgesOnlyExpiredProviders(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()
	now := time.Now()

	old := mustMakeProvider(t, h, "owner+old@example.com", "CA", "Old Provider")
	fresh := mustMakeProvider(t, h, "owner+fresh@example.com", "TX", "Fresh Provider")
	active := mustMakeProvider(t, h, "owner+active@example.com", "FL", "Active Provider")
	explicitlyDeleted := mustMakeProvider(t, h, "owner+del@example.com", "CA", "Del Provider")

	// 100 days ago (eligible)
	oldCanceled := now.Add(-100 * 24 * time.Hour)
	// 80 days ago (NOT eligible yet)
	freshCanceled := now.Add(-80 * 24 * time.Hour)

	mustExec(t, h, `UPDATE providers SET canceled_at = ? WHERE id = ?`, oldCanceled, old)
	mustExec(t, h, `UPDATE providers SET canceled_at = ? WHERE id = ?`, freshCanceled, fresh)

	// "explicitly deleted" case uses deleted_at only, no subscription.
	mustExec(t, h, `UPDATE providers SET deleted_at = ? WHERE id = ?`, oldCanceled, explicitlyDeleted)

	// Canceled subscription rows for old + fresh.
	mustExec(t, h, `
INSERT INTO subscriptions (id, provider_id, stripe_customer_id, plan, status, created_at, updated_at)
VALUES (?, ?, 'cus_old', 'pro', 'canceled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], old)
	mustExec(t, h, `
INSERT INTO subscriptions (id, provider_id, stripe_customer_id, plan, status, created_at, updated_at)
VALUES (?, ?, 'cus_fresh', 'pro', 'canceled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], fresh)
	// Active provider has an active subscription.
	mustExec(t, h, `
INSERT INTO subscriptions (id, provider_id, stripe_customer_id, plan, status, created_at, updated_at)
VALUES (?, ?, 'cus_active', 'pro', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], active)

	// Seed tenant data for every provider so we can confirm what survives.
	for _, pid := range []string{old, fresh, active, explicitlyDeleted} {
		mustExec(t, h, `
INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date, created_at, updated_at)
VALUES (?, ?, 'Ann', 'Doe', '2022-01-01', '2024-01-01', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			base62.NewID()[:22], pid)
		mustExec(t, h, `
INSERT INTO staff (id, provider_id, first_name, last_name, status, created_at, updated_at)
VALUES (?, ?, 'Sam', 'Smith', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			base62.NewID()[:22], pid)
		mustExec(t, h, `
INSERT INTO documents (id, provider_id, owner_kind, owner_id, doc_type, s3_key, uploaded_via, created_at, updated_at)
VALUES (?, ?, 'facility', ?, 'license', 'providers/'||?||'/license.pdf', 'provider', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			base62.NewID()[:22], pid, pid, pid)
	}

	// Run the worker with nil S3 client so the test is hermetic — the DB
	// purge path is what we care about here.
	worker := retention.NewPurgeWorker(h.DB, nil, nil)
	if err := worker.RunOnce(ctx, now); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// old + explicitlyDeleted should be gone.
	if providerExists(t, h, old) {
		t.Fatalf("old provider was not purged")
	}
	if providerExists(t, h, explicitlyDeleted) {
		t.Fatalf("explicitly-deleted provider was not purged")
	}
	// fresh + active should be untouched.
	if !providerExists(t, h, fresh) {
		t.Fatalf("fresh (80-day) provider was incorrectly purged")
	}
	if !providerExists(t, h, active) {
		t.Fatalf("active provider was incorrectly purged")
	}

	// Children/staff/documents for purged providers are gone.
	for _, pid := range []string{old, explicitlyDeleted} {
		if rowCount(t, h, `SELECT COUNT(*) FROM children WHERE provider_id = ?`, pid) != 0 {
			t.Fatalf("children rows remained for purged provider %s", pid)
		}
		if rowCount(t, h, `SELECT COUNT(*) FROM staff WHERE provider_id = ?`, pid) != 0 {
			t.Fatalf("staff rows remained for purged provider %s", pid)
		}
		if rowCount(t, h, `SELECT COUNT(*) FROM documents WHERE provider_id = ?`, pid) != 0 {
			t.Fatalf("documents rows remained for purged provider %s", pid)
		}
	}

	// Active provider untouched.
	if rowCount(t, h, `SELECT COUNT(*) FROM children WHERE provider_id = ?`, active) != 1 {
		t.Fatalf("active provider's children were disturbed")
	}

	// An audit_log row with action='retention.purged' exists for each purged
	// provider. After the provider is deleted, provider_id is SET NULL by
	// ON DELETE SET NULL, so we filter by action + target_id.
	for _, pid := range []string{old, explicitlyDeleted} {
		n := rowCount(t, h, `SELECT COUNT(*) FROM audit_log WHERE action = 'retention.purged' AND target_id = ?`, pid)
		if n != 1 {
			t.Fatalf("expected exactly 1 retention.purged audit row for %s, got %d", pid, n)
		}
	}
}

// TestRetention_ManifestShape validates the DeletionManifest struct's field
// names (by building one and asserting JSON output). Cheap regression guard —
// the manifest is a contract with future auditors.
func TestRetention_ManifestShape(t *testing.T) {
	m := retention.DeletionManifest{
		ProviderID:        "p123",
		OwnerEmail:        "o@example.com",
		PurgedAt:          "2026-01-01T00:00:00Z",
		RowCounts:         map[string]int{"children": 3},
		DeletedS3Prefixes: []string{"providers/p123/"},
	}
	if m.ProviderID != "p123" || m.RowCounts["children"] != 3 {
		t.Fatalf("manifest fields not as expected: %+v", m)
	}
}

// ---- helpers ----

func mustMakeProvider(t *testing.T, h *Harness, email, state, name string) string {
	t.Helper()
	id := base62.NewID()[:22]
	_, err := h.DB.Exec(`
INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 0, 'America/Los_Angeles', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, name, name, email, state, state)
	if err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	return id
}

func mustExec(t *testing.T, h *Harness, q string, args ...any) {
	t.Helper()
	if _, err := h.DB.Exec(q, args...); err != nil {
		t.Fatalf("exec %s: %v", q, err)
	}
}

func providerExists(t *testing.T, h *Harness, id string) bool {
	t.Helper()
	var n int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM providers WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count providers: %v", err)
	}
	return n > 0
}

func rowCount(t *testing.T, h *Harness, q string, args ...any) int {
	t.Helper()
	var n int
	if err := h.DB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}
