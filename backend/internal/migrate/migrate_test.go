package migrate_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/db"
	"github.com/tachyurgy/compliancekit/backend/internal/migrate"
	"github.com/tachyurgy/compliancekit/backend/migrations"
)

func TestUpIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool, err := db.Open(ctx, filepath.Join(t.TempDir(), "ck.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	n, err := migrate.Up(ctx, pool, migrations.FS, nil)
	if err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if n == 0 {
		t.Fatal("expected migrations to apply on an empty database")
	}
	again, err := migrate.Up(ctx, pool, migrations.FS, nil)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if again != 0 {
		t.Fatalf("second Up applied %d migrations, want 0", again)
	}
	var tables int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('providers','documents','sign_sessions','subscriptions')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables < 3 {
		t.Fatalf("expected core tables, found %d", tables)
	}
}
