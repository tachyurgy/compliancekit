package billing

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// applyMigrations walks backend/migrations/ from the test working dir and
// applies every *.up.sql in numeric order. Same approach as the integration
// harness — duplicated here so this unit test stays in the billing package
// without dragging in the integration fixtures.
func applyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	dir, err := findMigrationsDir()
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	files := []string{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
}

func findMigrationsDir() (string, error) {
	wd, _ := os.Getwd()
	for dir := wd; ; {
		c := filepath.Join(dir, "migrations")
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// TestStripeEvents_IdempotencyDedup confirms the schema's UNIQUE constraint
// on stripe_event_id rejects a second insert of the same Stripe event id.
// This is the cornerstone of REQ-002's "webhook is idempotent" promise:
// HandleWebhook attempts the insert first and bails on conflict. We don't
// drive HandleWebhook end-to-end here (that would need a Stripe-signed
// payload); we test the SQL-layer contract that makes it work.
func TestStripeEvents_IdempotencyDedup(t *testing.T) {
	t.Parallel()
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") + "?cache=shared&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	applyMigrations(t, db)

	ctx := context.Background()
	const eventID = "evt_test_idempotent"

	// Need a provider row so any FK fields don't trip; stripe_events has none
	// but the migrations enable foreign_keys, so we still want the schema clean.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO providers (id, legal_name, state, name, owner_email)
		VALUES ('p_idem', 'Test', 'FL', 'Test', 'idem@example.com')`); err != nil {
		t.Fatalf("insert provider: %v", err)
	}

	// First insert: succeeds.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO stripe_events (id, stripe_event_id, type, payload)
		VALUES ('e1', ?, 'checkout.session.completed', '{}')`, eventID); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Second insert with the SAME stripe_event_id: must fail on the UNIQUE.
	_, err = db.ExecContext(ctx, `
		INSERT INTO stripe_events (id, stripe_event_id, type, payload)
		VALUES ('e2', ?, 'checkout.session.completed', '{}')`, eventID)
	if err == nil {
		t.Fatalf("expected duplicate insert to fail on UNIQUE(stripe_event_id), got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("expected UNIQUE violation, got %v", err)
	}

	// Confirm exactly one row exists for the event id.
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM stripe_events WHERE stripe_event_id = ?`, eventID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 stripe_events row, got %d", count)
	}
}

// TestPlanFromPriceID covers the price-ID-to-plan-code mapping the webhook
// uses to mirror Stripe state onto providers.plan. A typo in either env var
// would silently leave plan as NULL, which the dashboard banner reads, so
// this is worth pinning.
func TestPlanFromPriceID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		priceID string
		monthly string
		annual  string
		want    string
	}{
		{"price_M", "price_M", "price_A", "monthly_99"},
		{"price_A", "price_M", "price_A", "annual_948"},
		{"price_unknown", "price_M", "price_A", ""},
		{"", "price_M", "price_A", ""},
	}
	for _, tc := range cases {
		got := planFromPriceID(tc.priceID, tc.monthly, tc.annual)
		if got != tc.want {
			t.Fatalf("planFromPriceID(%q,%q,%q) = %q, want %q",
				tc.priceID, tc.monthly, tc.annual, got, tc.want)
		}
	}
}
