// Package integration provides a test harness that spins up the ComplianceKit
// backend against a fresh SQLite file with every migration applied, so
// individual integration tests can focus on behaviour instead of plumbing.
//
// Use NewHarness(t) at the top of a test; call harness.Close() when done
// (testing.T.Cleanup handles this automatically).
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/api"
	"github.com/tachyurgy/compliancekit/backend/internal/db"
	"github.com/tachyurgy/compliancekit/backend/internal/handlers"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// Harness wires together the pieces needed to exercise the real HTTP router
// against an ephemeral SQLite database. External services (SES, Twilio, S3,
// Stripe, Mistral, Gemini) are left nil; handlers that would call them either
// no-op in that case (e.g. email send) or should not be exercised by the
// tests that use this harness.
type Harness struct {
	DB         *sql.DB
	Server     *httptest.Server
	Magic      *magiclink.Service
	SigningKey string
}

// NewHarness opens a fresh SQLite file (auto-cleaned), applies every migration
// under backend/migrations/ in order, and starts an httptest server that
// mounts the same router main() uses.
func NewHarness(t *testing.T) *Harness {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ck.db")

	pool, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	if err := applyMigrations(pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	const signingKey = "test-signing-key-at-least-32-bytes-long-xxxxxxxxxxx"
	magic := magiclink.NewService(pool, signingKey)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	providers := &handlers.ProviderHandler{
		Pool: pool, Magic: magic, Emailer: nil,
		FrontendBase: "http://localhost:5173", AppBase: "http://localhost:8080",
		Log: log.With("component", "providers"),
	}
	children := &handlers.ChildHandler{Pool: pool, Log: log.With("component", "children")}
	staff := &handlers.StaffHandler{Pool: pool, Log: log.With("component", "staff")}
	dash := &handlers.DashboardHandler{Pool: pool, Log: log.With("component", "dashboard")}
	docs := &handlers.DocumentHandler{Pool: pool, Log: log.With("component", "documents")}
	portal := &handlers.PortalHandler{Pool: pool, Magic: magic, Log: log.With("component", "portal")}
	billH := &handlers.BillingHandler{Pool: pool, Log: log.With("component", "billing")}
	stripeWH := &handlers.StripeWebhookHandler{Log: log.With("component", "stripe_wh")}

	router := api.NewRouter(api.Deps{
		Logger:          log,
		Providers:       providers,
		Children:        children,
		Staff:           staff,
		Documents:       docs,
		Dashboard:       dash,
		Portal:          portal,
		Billing:         billH,
		StripeWebhook:   stripeWH,
		Magic:           magic,
		Session:         providers,
		BillingChecker:  allowAllBilling{},
		RateLimit:       mw.NewTokenBucket(1000, 100), // generous so tests don't throttle
		FrontendOrigins: []string{"http://localhost:5173"},
		PDFSign:         nil,
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	return &Harness{DB: pool, Server: srv, Magic: magic, SigningKey: signingKey}
}

// URL builds an absolute test-server URL for the given path.
func (h *Harness) URL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return h.Server.URL + path
}

// applyMigrations applies every *.up.sql in backend/migrations/ in
// lexicographic order (which is also migration-number order).
func applyMigrations(pool *sql.DB) error {
	// Walk up from cwd to find the backend/migrations directory. Tests run
	// from the package directory (backend/test/integration/), so migrations
	// live at ../../migrations/.
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "migrations")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return applyDir(pool, candidate)
		}
		dir = filepath.Dir(dir)
	}
	return fmt.Errorf("could not locate migrations directory from %s", cwd)
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

// allowAllBilling is a BillingChecker that always returns "subscribed" — used
// so paywalled routes are reachable in integration tests without wiring real
// Stripe state. Replace with a stricter fake in tests that need it.
type allowAllBilling struct{}

func (allowAllBilling) HasActiveSubscription(ctx context.Context, providerID string) (bool, error) {
	return true, nil
}
