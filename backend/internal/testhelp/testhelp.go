// Package testhelp provides shared helpers for unit tests that need a
// migrated SQLite database.
//
// The package is deliberately small: one function OpenDB that returns a
// *sql.DB pointing at a fresh temp file with every up.sql applied.
// Tests that need richer fixtures (HTTP server, auth, etc.) should use
// the integration-level harness in test/integration/fixtures.go instead.
package testhelp

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/db"
)

// OpenDB opens a fresh SQLite file in t.TempDir(), applies every
// migrations/*.up.sql in order, and returns the pool. Cleanup is registered
// via t.Cleanup. Unit tests that just need "a schema-correct DB" call this.
func OpenDB(t testing.TB) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ck.db")

	pool, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("testhelp: db.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	if err := ApplyMigrations(pool); err != nil {
		t.Fatalf("testhelp: migrations: %v", err)
	}
	return pool
}

// ApplyMigrations walks up from the cwd looking for backend/migrations and
// applies every *.up.sql. Exposed for tests that manage their own pool.
func ApplyMigrations(pool *sql.DB) error {
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "migrations")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return applyDir(pool, candidate)
		}
		dir = filepath.Dir(dir)
	}
	return fmt.Errorf("testhelp: migrations directory not found from %s", cwd)
}

func applyDir(pool *sql.DB, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var ups []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(ups)
	for _, p := range ups {
		body, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		if _, err := pool.Exec(string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", filepath.Base(p), err)
		}
	}
	return nil
}
