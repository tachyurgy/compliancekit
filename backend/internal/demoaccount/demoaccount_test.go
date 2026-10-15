package demoaccount_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/db"
	"github.com/tachyurgy/compliancekit/backend/internal/demoaccount"
	"github.com/tachyurgy/compliancekit/backend/internal/migrate"
	"github.com/tachyurgy/compliancekit/backend/migrations"
)

func TestSharedAndSandbox(t *testing.T) {
	ctx := context.Background()
	pool, err := db.Open(ctx, filepath.Join(t.TempDir(), "ck.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := migrate.Up(ctx, pool, migrations.FS, nil); err != nil {
		t.Fatal(err)
	}

	p := &demoaccount.Provisioner{DB: pool}
	a, err := p.EnsureShared(ctx, "demo@example.com", "demo-password")
	if err != nil {
		t.Fatalf("EnsureShared: %v", err)
	}
	b, err := p.EnsureShared(ctx, "demo@example.com", "demo-password")
	if err != nil || b.ProviderID != a.ProviderID {
		t.Fatalf("second EnsureShared: %v %v", b, err)
	}
	var children int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM children WHERE provider_id = ?`, a.ProviderID).Scan(&children)
	if children == 0 {
		t.Fatal("expected seeded children")
	}

	s, err := p.CreateSandbox(ctx)
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	n, err := demoaccount.PurgeExpired(ctx, pool, time.Now().Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("PurgeExpired: n=%d err=%v", n, err)
	}
	var left int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM children WHERE provider_id = ?`, s.ProviderID).Scan(&left)
	if left != 0 {
		t.Fatalf("sandbox children survived purge: %d", left)
	}
	var shared int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM providers WHERE id = ?`, a.ProviderID).Scan(&shared)
	if shared != 1 {
		t.Fatal("purge removed the shared account")
	}
}
