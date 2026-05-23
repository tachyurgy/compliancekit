package demoseed_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/db"
	"github.com/tachyurgy/compliancekit/backend/internal/demoseed"
)

// applyMigrations is a thin copy of the integration-test helper — kept local
// so this package can ship its own unit-style tests without depending on
// test/integration's fixtures (which would create a cycle).
func applyMigrations(t *testing.T, pool *sql.DB) {
	t.Helper()
	cwd, _ := os.Getwd()
	dir := cwd
	var migrationsDir string
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "migrations")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			migrationsDir = candidate
			break
		}
		dir = filepath.Dir(dir)
	}
	if migrationsDir == "" {
		t.Fatalf("could not locate migrations directory from %s", cwd)
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var ups []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, filepath.Join(migrationsDir, e.Name()))
		}
	}
	sort.Strings(ups)
	for _, p := range ups {
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if _, err := pool.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(p), err)
		}
	}
}

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ck.db")
	pool, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	applyMigrations(t, pool)
	return pool
}

func insertRealUser(t *testing.T, pool *sql.DB) (userID, realProviderID string) {
	t.Helper()
	realProviderID = base62.NewID()[:22]
	userID = base62.NewID()[:22]
	if _, err := pool.Exec(`
		INSERT INTO providers (id, legal_name, state, state_code, timezone, created_at, updated_at)
		VALUES (?, 'Real Facility', 'FL', 'FL', 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		realProviderID); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if _, err := pool.Exec(`
		INSERT INTO users (id, provider_id, email, full_name, role, created_at, updated_at)
		VALUES (?, ?, 'owner@example.com', 'Owner', 'provider_admin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		userID, realProviderID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(`
		INSERT INTO provider_memberships (id, user_id, provider_id, role, created_at)
		VALUES (?, ?, ?, 'provider_admin', CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], userID, realProviderID); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
	return userID, realProviderID
}

func TestSeedDemoFacility_CreatesIsolatedProvider(t *testing.T) {
	pool := newDB(t)
	uid, realID := insertRealUser(t, pool)

	demoID, err := demoseed.SeedDemoFacility(context.Background(), pool, uid, realID)
	if err != nil {
		t.Fatalf("SeedDemoFacility: %v", err)
	}
	if demoID == "" {
		t.Fatal("empty demoID")
	}
	if demoID == realID {
		t.Fatalf("demoID should differ from realID; got both = %s", demoID)
	}

	// providers.is_demo should be 1 on the new row, 0 on the real one.
	var demoFlag, realFlag int
	if err := pool.QueryRow(`SELECT is_demo FROM providers WHERE id = ?`, demoID).Scan(&demoFlag); err != nil {
		t.Fatalf("demo is_demo: %v", err)
	}
	if demoFlag != 1 {
		t.Fatalf("demo provider is_demo = %d, want 1", demoFlag)
	}
	if err := pool.QueryRow(`SELECT is_demo FROM providers WHERE id = ?`, realID).Scan(&realFlag); err != nil {
		t.Fatalf("real is_demo: %v", err)
	}
	if realFlag != 0 {
		t.Fatalf("real provider is_demo = %d, want 0", realFlag)
	}

	// Real provider must remain empty.
	var realStaff, realKids int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM staff WHERE provider_id = ?`, realID).Scan(&realStaff)
	_ = pool.QueryRow(`SELECT COUNT(*) FROM children WHERE provider_id = ?`, realID).Scan(&realKids)
	if realStaff != 0 || realKids != 0 {
		t.Fatalf("real provider got seed rows: staff=%d kids=%d", realStaff, realKids)
	}

	// Demo provider must be populated.
	var demoStaff, demoKids int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM staff WHERE provider_id = ?`, demoID).Scan(&demoStaff)
	_ = pool.QueryRow(`SELECT COUNT(*) FROM children WHERE provider_id = ?`, demoID).Scan(&demoKids)
	if demoStaff < 3 || demoKids < 3 {
		t.Fatalf("demo provider underseeded: staff=%d kids=%d", demoStaff, demoKids)
	}

	// Membership row for the demo provider.
	var mc int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM provider_memberships WHERE user_id = ? AND provider_id = ?`, uid, demoID).Scan(&mc)
	if mc != 1 {
		t.Fatalf("demo membership count = %d, want 1", mc)
	}
}

func TestSeedDemoFacility_Idempotent(t *testing.T) {
	pool := newDB(t)
	uid, realID := insertRealUser(t, pool)

	first, err := demoseed.SeedDemoFacility(context.Background(), pool, uid, realID)
	if err != nil {
		t.Fatalf("first SeedDemoFacility: %v", err)
	}
	second, err := demoseed.SeedDemoFacility(context.Background(), pool, uid, realID)
	if err != nil {
		t.Fatalf("second SeedDemoFacility: %v", err)
	}
	if first != second {
		t.Fatalf("idempotency broken: first=%s second=%s", first, second)
	}
	// Only one demo provider per user.
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM providers WHERE is_demo = 1`).Scan(&n)
	if n != 1 {
		t.Fatalf("expected exactly one demo provider, got %d", n)
	}
}

func TestSeedDemoFacility_RejectsEmptyUserID(t *testing.T) {
	pool := newDB(t)
	if _, err := demoseed.SeedDemoFacility(context.Background(), pool, "", "anything"); err == nil {
		t.Fatal("expected error for empty userID")
	}
}
