package workers

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// ---- test harness ----

// openTestDB opens an in-memory SQLite DB and applies every migration in
// backend/migrations/ in order. Returns the pool + a silent logger suitable
// for worker tests.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dsn := filepath.Join(tmpDir, "workers_test.db") + "?_pragma=foreign_keys(ON)"
	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	// Apply every *.up.sql in ../../migrations/.
	migDir := findMigrations(t)
	entries, err := os.ReadDir(migDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var ups []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, filepath.Join(migDir, e.Name()))
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
	return pool
}

// findMigrations walks up from the test package to find the migrations dir.
// Tests run in backend/internal/workers/; migrations live at backend/migrations/.
func findMigrations(t *testing.T) string {
	t.Helper()
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "migrations")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("migrations directory not found from %s", cwd)
	return ""
}

// silentLog is a logger that drops everything — keeps test output clean.
func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// seedProvider inserts a minimal provider row and returns its ID.
// State defaults to "CA"; deleted = false by default.
func seedProvider(t *testing.T, pool *sql.DB, stateCode string, deleted bool) string {
	t.Helper()
	id := base62.NewID()[:22]
	var deletedExpr any
	if deleted {
		deletedExpr = "2026-01-01 00:00:00"
	}
	_, err := pool.Exec(`
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, deleted_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'America/Los_Angeles', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, "Test Daycare "+id[:4], "Test Daycare "+id[:4], "owner-"+id[:6]+"@ck.local",
		stateCode, stateCode, deletedExpr)
	if err != nil {
		t.Fatalf("seedProvider: %v", err)
	}
	return id
}

// seedUser inserts a user row tied to a provider and returns the user ID.
func seedUser(t *testing.T, pool *sql.DB, providerID string) string {
	t.Helper()
	id := base62.NewID()[:22]
	_, err := pool.Exec(`
		INSERT INTO users (id, provider_id, email, full_name, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'provider_admin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, providerID, "u-"+id[:6]+"@ck.local", "Test User "+id[:4])
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	return id
}

// seedSession inserts a session row for the given provider + user.
func seedSession(t *testing.T, pool *sql.DB, providerID string, expiresIn time.Duration) string {
	t.Helper()
	userID := seedUser(t, pool, providerID)
	id := base62.NewID()
	expiresAt := time.Now().Add(expiresIn).UTC().Format("2006-01-02 15:04:05")
	_, err := pool.Exec(`
		INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		id, providerID, userID, expiresAt)
	if err != nil {
		t.Fatalf("seedSession: %v", err)
	}
	return id
}

// seedMagicLinkToken inserts a magic-link token with explicit expires_at and
// optional consumed_at.
func seedMagicLinkToken(t *testing.T, pool *sql.DB, providerID string, expiresIn time.Duration, consumed bool) string {
	t.Helper()
	id := base62.NewID()
	expiresAt := time.Now().Add(expiresIn).UTC().Format("2006-01-02 15:04:05")
	var consumedAt any
	if consumed {
		consumedAt = time.Now().Add(-(expiresIn + 72*time.Hour)).UTC().Format("2006-01-02 15:04:05")
	}
	_, err := pool.Exec(`
		INSERT INTO magic_link_tokens (id, provider_id, subject_id, kind, token_hash, expires_at, consumed_at, created_at)
		VALUES (?, ?, ?, 'provider_signin', ?, ?, ?, CURRENT_TIMESTAMP)`,
		id, providerID, providerID, []byte("fakehash"+id), expiresAt, consumedAt)
	if err != nil {
		t.Fatalf("seedMagicLinkToken: %v", err)
	}
	return id
}

// ---- SessionGC tests ----

func TestSessionGC_DeletesExpiredSessions(t *testing.T) {
	pool := openTestDB(t)
	provider := seedProvider(t, pool, "CA", false)
	expired := seedSession(t, pool, provider, -1*time.Hour)
	live := seedSession(t, pool, provider, 24*time.Hour)

	gc := &SessionGC{Pool: pool, Log: silentLog()}
	gc.runOnce(context.Background())

	// Expired session must be gone.
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, expired).Scan(&n)
	if n != 0 {
		t.Fatalf("expected expired session deleted, got %d rows", n)
	}
	// Live session must still exist.
	_ = pool.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, live).Scan(&n)
	if n != 1 {
		t.Fatalf("expected live session preserved, got %d rows", n)
	}
}

func TestSessionGC_DeletesOldConsumedTokens(t *testing.T) {
	pool := openTestDB(t)
	provider := seedProvider(t, pool, "TX", false)
	// Consumed > 2 days ago → should be deleted.
	oldConsumed := seedMagicLinkToken(t, pool, provider, 15*time.Minute, true)
	// Expired > 2 days ago but never consumed → should be deleted.
	oldExpired := seedMagicLinkToken(t, pool, provider, -5*24*time.Hour, false)
	// Still-live, unconsumed → keep.
	live := seedMagicLinkToken(t, pool, provider, 10*time.Minute, false)

	gc := &SessionGC{Pool: pool, Log: silentLog()}
	gc.runOnce(context.Background())

	for _, tc := range []struct {
		id       string
		wantKeep bool
	}{
		{oldConsumed, false},
		{oldExpired, false},
		{live, true},
	} {
		var n int
		_ = pool.QueryRow(`SELECT COUNT(*) FROM magic_link_tokens WHERE id = ?`, tc.id).Scan(&n)
		if tc.wantKeep && n == 0 {
			t.Errorf("token %s: expected kept, was deleted", tc.id)
		}
		if !tc.wantKeep && n != 0 {
			t.Errorf("token %s: expected deleted, still present", tc.id)
		}
	}
}

func TestSessionGC_HandlesEmptyDB(t *testing.T) {
	// Zero providers, zero sessions, zero tokens — runOnce must not panic.
	pool := openTestDB(t)
	gc := &SessionGC{Pool: pool, Log: silentLog()}
	gc.runOnce(context.Background()) // should just log "0 0" and return
}

func TestSessionGC_RespectsCanceledContext(t *testing.T) {
	pool := openTestDB(t)
	provider := seedProvider(t, pool, "FL", false)
	_ = seedSession(t, pool, provider, -1*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before invocation

	gc := &SessionGC{Pool: pool, Log: silentLog()}
	gc.runOnce(ctx) // should handle gracefully; sqlite ExecContext honors ctx
}

// ---- SnapshotWorker tests ----

func TestSnapshotWorker_WritesSnapshotPerLiveProvider(t *testing.T) {
	pool := openTestDB(t)
	active1 := seedProvider(t, pool, "CA", false)
	active2 := seedProvider(t, pool, "TX", false)
	_ = seedProvider(t, pool, "FL", true) // deleted — should be skipped

	w := &SnapshotWorker{Pool: pool, Log: silentLog()}
	w.runOnce(context.Background())

	// Expect exactly 2 snapshot rows (one per active provider).
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM compliance_snapshots`).Scan(&n)
	if n != 2 {
		t.Fatalf("expected 2 snapshot rows, got %d", n)
	}
	// Confirm both active providers got a snapshot.
	for _, id := range []string{active1, active2} {
		var m int
		_ = pool.QueryRow(`SELECT COUNT(*) FROM compliance_snapshots WHERE provider_id = ?`, id).Scan(&m)
		if m != 1 {
			t.Errorf("expected 1 snapshot for provider %s, got %d", id, m)
		}
	}
}

func TestSnapshotWorker_IncludesDrillCount(t *testing.T) {
	pool := openTestDB(t)
	provider := seedProvider(t, pool, "CA", false)

	// Insert 2 fire drills in the last 90d + 1 drill 100 days ago (should NOT count).
	for _, age := range []int{5, 30, 100} {
		_, err := pool.Exec(`
			INSERT INTO drill_logs (id, provider_id, drill_kind, drill_date, created_at, updated_at)
			VALUES (?, ?, 'fire', datetime('now', ?), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			base62.NewID()[:22], provider, "-"+strings.TrimPrefix(strings.Replace(string(rune(age+'0')), "", "", 0), "")+"-"+time.Duration(age).String())
		_ = err // SQLite datetime param is janky above; use an explicit INSERT below
	}
	// Cleaner: explicit dates.
	pool.Exec(`DELETE FROM drill_logs`)
	pool.Exec(`
		INSERT INTO drill_logs (id, provider_id, drill_kind, drill_date, created_at, updated_at) VALUES
		(?, ?, 'fire', datetime('now','-5 days'), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(?, ?, 'fire', datetime('now','-30 days'), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(?, ?, 'fire', datetime('now','-100 days'), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], provider,
		base62.NewID()[:22], provider,
		base62.NewID()[:22], provider)

	w := &SnapshotWorker{Pool: pool, Log: silentLog()}
	facts, err := w.loadFacts(context.Background(), provider)
	if err != nil {
		t.Fatalf("loadFacts: %v", err)
	}
	if facts.DrillsLast90d != 2 {
		t.Errorf("expected DrillsLast90d=2 (5d + 30d), got %d", facts.DrillsLast90d)
	}
}

func TestSnapshotWorker_UnsupportedStateSurfacesViolation(t *testing.T) {
	pool := openTestDB(t)
	// Directly write a provider with an unsupported state code (bypassing signup
	// validation — simulates a legacy/imported row).
	id := base62.NewID()[:22]
	_, err := pool.Exec(`
		INSERT INTO providers (id, legal_name, name, state, state_code, timezone, capacity, owner_email, created_at, updated_at)
		VALUES (?, 'Oregon Test', 'Oregon Test', 'OR', 'OR', 'America/Los_Angeles', 0, 'test-or@ck.local', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id)
	if err != nil {
		t.Fatalf("insert OR provider: %v", err)
	}

	w := &SnapshotWorker{Pool: pool, Log: silentLog()}
	w.runOnce(context.Background())

	// Expect a snapshot with violation_count >= 1 and critical_count >= 1
	// (STATE-NOT-SUPPORTED is Critical).
	var score, vc, cc int
	err = pool.QueryRow(`
		SELECT score, violation_count, critical_count
		FROM compliance_snapshots WHERE provider_id = ? ORDER BY computed_at DESC LIMIT 1`, id).Scan(&score, &vc, &cc)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if vc < 1 {
		t.Errorf("expected at least 1 violation for unsupported state, got %d", vc)
	}
	if cc < 1 {
		t.Errorf("expected at least 1 critical violation, got %d", cc)
	}
	if score >= 100 {
		t.Errorf("score must be <100 for unsupported state, got %d", score)
	}
}

func TestSnapshotWorker_HandlesEmptyDB(t *testing.T) {
	pool := openTestDB(t)
	w := &SnapshotWorker{Pool: pool, Log: silentLog()}
	w.runOnce(context.Background()) // should just log and return cleanly
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM compliance_snapshots`).Scan(&n)
	if n != 0 {
		t.Errorf("expected 0 snapshots, got %d", n)
	}
}

func TestSnapshotWorker_RespectsCanceledContext(t *testing.T) {
	pool := openTestDB(t)
	// Seed 3 providers so the worker has work to do.
	for _, s := range []string{"CA", "TX", "FL"} {
		seedProvider(t, pool, s, false)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before invocation

	w := &SnapshotWorker{Pool: pool, Log: silentLog()}
	w.runOnce(ctx)

	// With a canceled context, the loop returns early after the first snapshot
	// (or zero). Assert that fewer than all 3 were written.
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM compliance_snapshots`).Scan(&n)
	if n >= 3 {
		t.Errorf("expected context cancellation to abort loop, got %d snapshots", n)
	}
}

// ---- Integration with workers.RunDaily (ticker-less happy path) ----

func TestSessionGC_RunDaily_FiresOnceAndExitsOnCancel(t *testing.T) {
	pool := openTestDB(t)
	provider := seedProvider(t, pool, "CA", false)
	expired := seedSession(t, pool, provider, -1*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	gc := &SessionGC{Pool: pool, Log: silentLog()}

	done := make(chan struct{})
	go func() {
		gc.RunDaily(ctx) // runs once, then ticks; must return on cancel
		close(done)
	}()

	// Give it a beat to run the startup pass, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("RunDaily did not exit within 2s after cancel")
	}

	// The startup pass should have cleaned the expired session.
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, expired).Scan(&n)
	if n != 0 {
		t.Errorf("expected startup pass to delete expired session, still present")
	}
}
