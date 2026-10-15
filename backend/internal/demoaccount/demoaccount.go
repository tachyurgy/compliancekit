// Package demoaccount provisions ready-to-use demo tenants for the public
// demo deployment (DEMO_MODE=1). It never runs in a normal deployment.
//
// Two shapes:
//
//   - EnsureShared keeps one known login (documented in the README) alive and
//     re-seeds it if a visitor purged its demo rows.
//   - CreateSandbox mints a throw-away tenant per visitor for the one-click
//     "Explore the live demo" button, so reviewers never trip over each
//     other's edits. Sandboxes are deleted after SandboxTTL by Janitor.
//
// Every tenant is a fully onboarded Florida center (legal terms accepted,
// subscription active, rooms/staff/children/documents seeded by demoseed),
// which is the state a real operator reaches after signup -> checkout ->
// legal -> wizard. Stripe is never called: the subscription row is written
// directly with a fake customer id.
package demoaccount

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/demoseed"
	"github.com/tachyurgy/compliancekit/backend/internal/pdfsign"
)

// SandboxDomain is the reserved (RFC 2606) domain used for per-visitor
// sandbox owners. Nothing can ever be delivered to it.
const SandboxDomain = "sandbox.invalid"

// SandboxTTL is how long a one-click sandbox lives.
const SandboxTTL = 24 * time.Hour

// PermissionSlipPDF is a fictional one-page form seeded as an e-signature
// template so the signing flow can be tried without uploading anything.
// Source: assets/permission-slip.html, printed to PDF with headless Chromium.
//
//go:embed assets/permission-slip.pdf
var PermissionSlipPDF []byte

// PermissionSlipFields are the pre-placed fields for PermissionSlipPDF, in PDF
// points from the bottom-left corner (US Letter, 612x792), matching the boxes
// drawn in assets/permission-slip.html.
func PermissionSlipFields() []pdfsign.Field {
	return []pdfsign.Field{
		{ID: "agree", Type: pdfsign.FieldCheckbox, PageIndex: 0, X: 72, Y: 316.8, Width: 21.6, Height: 21.6, Required: true, Label: "I agree"},
		{ID: "printed_name", Type: pdfsign.FieldText, PageIndex: 0, X: 72, Y: 252, Width: 252, Height: 36, Required: true, Label: "Printed name"},
		{ID: "signature", Type: pdfsign.FieldSignature, PageIndex: 0, X: 72, Y: 172.8, Width: 252, Height: 43.2, Required: true, Label: "Signature"},
		{ID: "date", Type: pdfsign.FieldDate, PageIndex: 0, X: 360, Y: 172.8, Width: 162, Height: 43.2, Required: true, Label: "Date"},
	}
}

// Provisioner creates demo tenants. AfterCreate, when set, runs once per new
// tenant after the relational seed (main wires it to upload the sample
// e-signature template through the pdfsign service and storage).
type Provisioner struct {
	DB          *sql.DB
	AfterCreate func(ctx context.Context, providerID string) error
}

// Account identifies a provisioned demo tenant.
type Account struct {
	ProviderID string
	UserID     string
	Email      string
}

// EnsureShared creates the shared demo login if it does not exist yet, and
// re-seeds its demo data if a visitor removed it. Idempotent.
func (p *Provisioner) EnsureShared(ctx context.Context, email, password string) (Account, error) {
	db := p.DB
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(password) < 8 {
		return Account{}, errors.New("demoaccount: shared account needs an email and an 8+ char password")
	}
	var a Account
	err := db.QueryRowContext(ctx, `
		SELECT u.id, u.provider_id FROM users u
		  JOIN providers p ON p.id = u.provider_id
		 WHERE u.email = ? AND u.deleted_at IS NULL AND p.deleted_at IS NULL
		 LIMIT 1`, email).Scan(&a.UserID, &a.ProviderID)
	switch {
	case err == nil:
		a.Email = email
		// Re-seed if the demo rows were purged ("Make this mine").
		if _, err := demoseed.Seed(ctx, db, a.ProviderID); err != nil {
			return a, fmt.Errorf("demoaccount: reseed shared: %w", err)
		}
		var templates int
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pdfsign_templates WHERE provider_id = ?`, a.ProviderID).Scan(&templates)
		if templates == 0 && p.AfterCreate != nil {
			if err := p.AfterCreate(ctx, a.ProviderID); err != nil {
				return a, fmt.Errorf("demoaccount: reseed shared extras: %w", err)
			}
		}
		return a, nil
	case errors.Is(err, sql.ErrNoRows):
		return p.create(ctx, email, password, "Demo Director")
	default:
		return Account{}, fmt.Errorf("demoaccount: lookup shared: %w", err)
	}
}

// CreateSandbox provisions a fresh tenant with an unguessable owner email
// and no usable password; the caller establishes a session for it directly.
func (p *Provisioner) CreateSandbox(ctx context.Context) (Account, error) {
	email := "visitor-" + strings.ToLower(base62.NewID()[:12]) + "@" + SandboxDomain
	return p.create(ctx, email, base62.NewID(), "Demo Visitor")
}

func (p *Provisioner) create(ctx context.Context, email, password, fullName string) (Account, error) {
	db := p.DB
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, fmt.Errorf("demoaccount: hash: %w", err)
	}
	a := Account{
		ProviderID: base62.NewID()[:22],
		UserID:     base62.NewID()[:22],
		Email:      email,
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, fmt.Errorf("demoaccount: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmts := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO providers
		      (id, legal_name, name, dba, state, state_code, state_abbr, owner_email,
		       license_number, license_type, timezone, onboarding_complete,
		       onboarding_completed_at, is_demo, created_at, updated_at)
		  VALUES (?, ?, ?, ?, 'FL', 'FL', 'FL', ?, 'C11MD0000', 'center', 'America/New_York', 1,
		          CURRENT_TIMESTAMP, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			[]any{a.ProviderID, demoseed.DemoFacilityName, demoseed.DemoFacilityName,
				demoseed.DemoFacilityName, email}},
		{`INSERT INTO users (id, provider_id, email, full_name, role, password_hash,
		                     password_set_at, password_updated_at, created_at, updated_at)
		  VALUES (?, ?, ?, ?, 'provider_admin', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP,
		          CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			[]any{a.UserID, a.ProviderID, email, fullName, string(hash)}},
		{`INSERT INTO provider_memberships (id, user_id, provider_id, role)
		  VALUES (?, ?, ?, 'provider_admin')`,
			[]any{base62.NewID()[:22], a.UserID, a.ProviderID}},
		{`INSERT INTO subscriptions (id, provider_id, stripe_customer_id, plan, status, current_period_end)
		  VALUES (?, ?, ?, 'pro', 'active', ?)`,
			[]any{base62.NewID()[:22], a.ProviderID, "cus_demo_" + a.ProviderID[:10],
				time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339)}},
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s.q, s.args...); err != nil {
			return Account{}, fmt.Errorf("demoaccount: provision: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Account{}, fmt.Errorf("demoaccount: commit: %w", err)
	}
	if _, err := demoseed.Seed(ctx, db, a.ProviderID); err != nil {
		return a, fmt.Errorf("demoaccount: seed: %w", err)
	}
	if p.AfterCreate != nil {
		if err := p.AfterCreate(ctx, a.ProviderID); err != nil {
			return a, fmt.Errorf("demoaccount: after-create: %w", err)
		}
	}
	return a, nil
}

// Janitor deletes expired sandboxes and keeps the shared account seeded.
// It runs until ctx is cancelled.
func (p *Provisioner) Janitor(ctx context.Context, sharedEmail, sharedPassword string, log *slog.Logger) {
	db := p.DB
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if n, err := PurgeExpired(ctx, db, time.Now().Add(-SandboxTTL)); err != nil {
			log.Warn("demo janitor: purge", "err", err)
		} else if n > 0 {
			log.Info("demo janitor: purged sandboxes", "count", n)
		}
		if sharedEmail != "" {
			if _, err := p.EnsureShared(ctx, sharedEmail, sharedPassword); err != nil {
				log.Warn("demo janitor: shared account", "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// PurgeExpired hard-deletes sandbox tenants created before cutoff. Child rows
// go with them through ON DELETE CASCADE. Returns the number of tenants removed.
func PurgeExpired(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	like := "%@" + SandboxDomain
	ts := cutoff.UTC().Format("2006-01-02 15:04:05")
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM sessions WHERE provider_id IN
		  (SELECT id FROM providers WHERE owner_email LIKE ? AND created_at < ?)`, like, ts); err != nil {
		return 0, fmt.Errorf("demoaccount: purge sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM users WHERE provider_id IN
		  (SELECT id FROM providers WHERE owner_email LIKE ? AND created_at < ?)`, like, ts); err != nil {
		return 0, fmt.Errorf("demoaccount: purge users: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`DELETE FROM providers WHERE owner_email LIKE ? AND created_at < ?`, like, ts)
	if err != nil {
		return 0, fmt.Errorf("demoaccount: purge providers: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, tx.Commit()
}
