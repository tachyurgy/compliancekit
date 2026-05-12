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
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/api"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/billing"
	"github.com/tachyurgy/compliancekit/backend/internal/db"
	"github.com/tachyurgy/compliancekit/backend/internal/handlers"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// HarnessOpts tunes the harness for specific test scenarios — e.g. tightening
// the rate-limit bucket when exercising the /auth/* throttle, or wiring an
// explicit DataExport handler that the default harness leaves nil.
type HarnessOpts struct {
	// RateLimit, if non-nil, replaces the default generous bucket.
	RateLimit *mw.TokenBucket
	// WithDataExport mounts a DataExportHandler (Storage=nil). The existing
	// route is admin-gated; tests can still hit Create/List without Storage
	// configured — only Download needs it.
	WithDataExport bool
	// WithBillingWebhookSecret, if non-empty, wires a real billing.Service
	// with the given webhook secret so the Stripe webhook handler can verify
	// signatures. SecretKey is left empty (we don't make Stripe API calls
	// from the webhook path).
	WithBillingWebhookSecret string
}

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
	return NewHarnessWithOpts(t, HarnessOpts{})
}

// NewHarnessWithOpts is NewHarness plus a tunable HarnessOpts. Use it when a
// test needs a tight rate-limit bucket, a DataExport handler wired in, etc.
func NewHarnessWithOpts(t *testing.T, opts HarnessOpts) *Harness {
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

	var billingSvc *billing.Service
	if opts.WithBillingWebhookSecret != "" {
		billingSvc = billing.New(billing.Config{
			WebhookSecret: opts.WithBillingWebhookSecret,
			Pool:          pool,
			Logger:        log.With("component", "billing"),
		})
	}
	billH := &handlers.BillingHandler{Pool: pool, Billing: billingSvc, Log: log.With("component", "billing")}
	stripeWH := &handlers.StripeWebhookHandler{Billing: billingSvc, Log: log.With("component", "stripe_wh")}
	sesWH := &handlers.SESWebhookHandler{Pool: pool, Log: log.With("component", "ses_wh")}
	twilioWH := &handlers.TwilioWebhookHandler{Pool: pool, Log: log.With("component", "twilio_wh")}
	drills := &handlers.DrillHandler{Pool: pool, Log: log.With("component", "drills")}
	postingsH := &handlers.PostingHandler{Pool: pool, Log: log.With("component", "postings")}
	ratioH := &handlers.RatioHandler{Pool: pool, Log: log.With("component", "ratio")}
	inspectionsH := &handlers.InspectionHandler{Pool: pool, Log: log.With("component", "inspections")}
	inspectionModeH := &handlers.InspectionModeHandler{Pool: pool, Log: log.With("component", "inspector_mode")}
	auditLogH := &handlers.AuditLogHandler{Pool: pool, Log: log.With("component", "audit_log")}

	var dataExportH *handlers.DataExportHandler
	if opts.WithDataExport {
		dataExportH = &handlers.DataExportHandler{Pool: pool, Log: log.With("component", "dataexport")}
	}

	rateLimit := opts.RateLimit
	if rateLimit == nil {
		rateLimit = mw.NewTokenBucket(1000, 100) // generous so tests don't throttle
	}

	facility := &handlers.FacilityHandler{Pool: pool, Log: log.With("component", "facility")}
	uploadLink := &handlers.UploadLinkHandler{Pool: pool, Magic: magic, FrontendBase: "http://localhost:5173"}

	// REQ-011 / REQ-034: AuthHandler powers /api/auth/forgot-password,
	// /api/auth/reset-password, /api/auth/sessions, /api/auth/signout-everywhere.
	// Emailer is nil in tests; the handler skips the email-send branch and
	// still creates the password_resets row, which is what we assert on.
	authH := &handlers.AuthHandler{
		Pool: pool, Emailer: nil,
		FrontendBase: "http://localhost:5173",
		CookieDomain: "localhost", SecureCookie: false,
		Log: log.With("component", "auth"),
	}

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
		SESWebhook:      sesWH,
		TwilioWebhook:   twilioWH,
		Drills:          drills,
		Postings:        postingsH,
		Ratio:           ratioH,
		Inspections:     inspectionsH,
		InspectionMode:  inspectionModeH,
		AuditLog:        auditLogH,
		DataExport:      dataExportH,
		Facility:        facility,
		UploadLink:      uploadLink,
		Auth:            authH,
		Magic:           magic,
		Session:         providers,
		RoleLookup:      mw.PoolRoleLookup{DB: pool},
		BillingChecker:  allowAllBilling{},
		RateLimit:       rateLimit,
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

// AuthAs creates a fresh provider + admin user directly via SQL, issues a
// sessions row (skipping the magic-link flow, which is covered separately),
// and returns an *http.Client whose cookie jar carries the ck_sess cookie for
// every subsequent request. Tests that need a signed-in client use this.
//
// stateCode is stored on both the legacy `state` column and the handler-era
// `state_code` column so all handler SQL reads find the expected value.
func (h *Harness) AuthAs(t *testing.T, stateCode string) (*http.Client, string, string) {
	t.Helper()

	stateUpper := strings.ToUpper(stateCode)
	providerID := base62.NewID()[:22]
	userID := base62.NewID()[:22]
	sessionID := base62.NewID()
	email := fmt.Sprintf("owner-%s@example.com", strings.ToLower(providerID))

	// Provider row — legal_name is NOT NULL in 000001; also populate the
	// migration-000012 columns the handlers actually read from.
	if _, err := h.DB.Exec(`
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'America/Los_Angeles', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		providerID, "Test Facility "+providerID, "Test Facility "+providerID, email, stateUpper, stateUpper); err != nil {
		t.Fatalf("AuthAs: insert provider: %v", err)
	}

	if _, err := h.DB.Exec(`
		INSERT INTO users (id, provider_id, email, full_name, role, created_at, updated_at)
		VALUES (?, ?, ?, 'Test Admin', 'provider_admin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		userID, providerID, email); err != nil {
		t.Fatalf("AuthAs: insert user: %v", err)
	}

	// Session row — expires_at is TEXT per 000009 schema, so we pass a
	// datetime('now', '+14 days') expression; but since we're going through
	// database/sql, bind a literal ISO-8601 timestamp instead.
	expires := time.Now().Add(14 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := h.DB.Exec(`
		INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		sessionID, providerID, userID, expires); err != nil {
		t.Fatalf("AuthAs: insert session: %v", err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("AuthAs: cookie jar: %v", err)
	}
	u, err := url.Parse(h.Server.URL)
	if err != nil {
		t.Fatalf("AuthAs: parse server URL: %v", err)
	}
	jar.SetCookies(u, []*http.Cookie{{
		Name:  "ck_sess",
		Value: sessionID,
		Path:  "/",
	}})

	client := &http.Client{Jar: jar}
	return client, providerID, userID
}

// AuthAsStaff is AuthAs but with role=provider_staff. Returns the admin
// provider_id so tenant-isolation assertions against admin-created rows are
// possible. Tests that need both an admin AND a staff client on the same
// provider should instead use rbac_test.go's seedUserWithRole helper — that
// seeds two users on one provider. AuthAsStaff here always creates a fresh
// provider with a single staff user (mirrors AuthAs).
func (h *Harness) AuthAsStaff(t *testing.T, stateCode string) (*http.Client, string, string) {
	t.Helper()

	stateUpper := strings.ToUpper(stateCode)
	providerID := base62.NewID()[:22]
	userID := base62.NewID()[:22]
	sessionID := base62.NewID()
	email := fmt.Sprintf("staff-%s@example.com", strings.ToLower(providerID))

	if _, err := h.DB.Exec(`
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'America/Los_Angeles', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		providerID, "Test Facility "+providerID, "Test Facility "+providerID, email, stateUpper, stateUpper); err != nil {
		t.Fatalf("AuthAsStaff: insert provider: %v", err)
	}
	if _, err := h.DB.Exec(`
		INSERT INTO users (id, provider_id, email, full_name, role, created_at, updated_at)
		VALUES (?, ?, ?, 'Test Staff', 'provider_staff', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		userID, providerID, email); err != nil {
		t.Fatalf("AuthAsStaff: insert user: %v", err)
	}
	expires := time.Now().Add(14 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := h.DB.Exec(`
		INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		sessionID, providerID, userID, expires); err != nil {
		t.Fatalf("AuthAsStaff: insert session: %v", err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("AuthAsStaff: cookie jar: %v", err)
	}
	u, err := url.Parse(h.Server.URL)
	if err != nil {
		t.Fatalf("AuthAsStaff: parse server URL: %v", err)
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "ck_sess", Value: sessionID, Path: "/"}})

	return &http.Client{Jar: jar}, providerID, userID
}
