package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"golang.org/x/crypto/bcrypt"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
	"github.com/tachyurgy/compliancekit/backend/internal/notify"
)

type ProviderHandler struct {
	Pool         *sql.DB
	Magic        *magiclink.Service
	Emailer      *notify.Emailer
	FrontendBase string
	AppBase      string
	CookieDomain string
	SecureCookie bool
	Log          *slog.Logger
}

// minPasswordLen is the floor for new passwords. We require length only;
// strength rules tend to push users to predictable substitutions and we'd
// rather invest in good rate-limiting (already wired) than complexity rules.
const minPasswordLen = 10

// POST /api/auth/signup
// body: { "name", "owner_email", "state_code", "password" }
// Creates a provider AND its admin user with a password hash, then logs in.
func (h *ProviderHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		OwnerEmail string `json:"owner_email"`
		StateCode  string `json:"state_code"`
		Password   string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	in.OwnerEmail = strings.ToLower(strings.TrimSpace(in.OwnerEmail))
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.OwnerEmail == "" || in.StateCode == "" || in.Password == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("name, owner_email, state_code, and password are required"))
		return
	}
	if len(in.Password) < minPasswordLen {
		httpx.RenderError(w, r, httpx.BadRequestf("password must be at least %d characters", minPasswordLen))
		return
	}
	switch strings.ToUpper(in.StateCode) {
	case "CA", "TX", "FL":
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("state_code must be one of CA, TX, FL (MVP scope)"))
		return
	}

	// Refuse to silently overwrite an existing account on signup. Returning a
	// generic error here is a tradeoff: it leaks that the email is registered,
	// but the alternative (auto-login on duplicate) lets anyone reset another
	// owner's account. Prefer explicit /signin if you already have an account.
	var existingID string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT id FROM providers WHERE owner_email = ? AND deleted_at IS NULL`, in.OwnerEmail).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if existingID != "" {
		httpx.RenderError(w, r, httpx.BadRequestf("an account with that email already exists; please sign in"))
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	providerID := base62.NewID()[:22]
	stateUpper := strings.ToUpper(in.StateCode)

	tx, err := h.Pool.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'America/Los_Angeles', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		providerID, in.Name, in.Name, in.OwnerEmail, stateUpper, stateUpper); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	userID := base62.NewID()[:22]
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO users (id, provider_id, email, full_name, role, password_hash, password_set_at, email_verified_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'provider_admin', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		userID, providerID, in.OwnerEmail, in.Name, string(hashed)); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	if err := h.establishSession(w, r, providerID, userID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitSignup(r.Context(), h.Pool, providerID, map[string]any{
		"state_code":  stateUpper,
		"owner_email": in.OwnerEmail,
	}, r)
	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"status":      "ok",
		"provider_id": providerID,
		"user_id":     userID,
	})
}

// POST /api/auth/signin { "email", "password" } -> sets session cookie
func (h *ProviderHandler) Signin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("email and password required"))
		return
	}

	var userID, providerID, hash string
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT u.id, u.provider_id, COALESCE(u.password_hash, '')
		FROM users u
		JOIN providers p ON p.id = u.provider_id
		WHERE u.email = ? AND u.role = 'provider_admin'
		  AND u.deleted_at IS NULL AND p.deleted_at IS NULL
		LIMIT 1`, email).Scan(&userID, &providerID, &hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Burn a hash anyway to keep the timing similar between
			// nonexistent-email and bad-password.
			_ = bcrypt.CompareHashAndPassword(
				[]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvalid"),
				[]byte(in.Password))
			httpx.RenderError(w, r, httpx.ErrUnauthorized)
			return
		}
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if hash == "" {
		// Account predates password auth (was magic-link only). Force a reset
		// rather than silently letting them sign in.
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrUnauthorized,
			fmt.Errorf("account requires password reset")))
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)); err != nil {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}

	if err := h.establishSession(w, r, providerID, userID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`, userID); err != nil {
		h.Log.Warn("signin: last_login_at update", "err", err)
	}
	auditlog.EmitLogin(r.Context(), h.Pool, providerID, providerID, r)
	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"status":      "ok",
		"provider_id": providerID,
		"user_id":     userID,
	})
}

// Callback is preserved as a deprecation stub. Magic-link auth has been
// removed in favor of passwords; existing links 410.
func (h *ProviderHandler) Callback(w http.ResponseWriter, r *http.Request) {
	httpx.RenderError(w, r, httpx.BadRequestf("magic-link login is no longer supported; please sign in with your password"))
}

// establishSession inserts a sessions row and sets the ck_sess cookie. Used
// by Signup and Signin so the cookie/expiry contract stays in one place.
func (h *ProviderHandler) establishSession(w http.ResponseWriter, r *http.Request, providerID, userID string) error {
	sessID := base62.NewID()
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, datetime('now', '+14 days'), CURRENT_TIMESTAMP)`,
		sessID, providerID, userID); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "ck_sess",
		Value:    sessID,
		Domain:   h.CookieDomain,
		Path:     "/",
		Expires:  time.Now().Add(14 * 24 * time.Hour),
		HttpOnly: true,
		Secure:   h.SecureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// GET /api/me
func (h *ProviderHandler) Me(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var p models.Provider
	// created_at / updated_at are stored as TEXT (see ADR-017); modernc's
	// driver only auto-parses columns declared DATE/DATETIME/TIMESTAMP. Scan
	// them as strings and parse below so callers still receive time.Time.
	var createdStr, updatedStr string
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT p.id,
		       COALESCE(p.name, p.legal_name, '') AS name,
		       COALESCE(p.legal_name, '') AS legal_name,
		       COALESCE(p.state_code, p.state, '') AS state_code,
		       COALESCE(p.license_number, '') AS license_number,
		       COALESCE(p.owner_email, '') AS owner_email,
		       COALESCE(p.phone, '') AS phone,
		       p.capacity, p.timezone,
		       COALESCE((SELECT s.stripe_customer_id FROM subscriptions s WHERE s.provider_id = p.id ORDER BY s.created_at DESC LIMIT 1), '') AS stripe_cust,
		       p.created_at, p.updated_at
		FROM providers p WHERE p.id = ?`, pid).
		Scan(&p.ID, &p.Name, &p.LegalName, &p.StateCode, &p.LicenseNumber, &p.OwnerEmail,
			&p.OwnerPhone, &p.Capacity, &p.Timezone, &p.StripeCustID, &createdStr, &updatedStr)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	p.CreatedAt = parseSQLiteTime(createdStr)
	p.UpdatedAt = parseSQLiteTime(updatedStr)

	// The frontend's SessionUser schema expects a flatter shape keyed by
	// `email`, `fullName`, `providerId`, `role`, `onboardingComplete` — see
	// frontend/src/api/providers.ts. Until the schemas are unified we emit
	// BOTH the Provider fields AND the SessionUser fields so whichever side
	// reads the payload finds what it expects.
	//
	// onboardingComplete: we treat a provider with a non-empty address+capacity
	// as onboarded. Exact criteria will live on a dedicated column in a future
	// migration (tracked as ADR: onboarding-state).
	var onboarded bool
	var city string
	_ = h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(onboarding_complete, 0), COALESCE(city, '') FROM providers WHERE id = ?`, pid).
		Scan(&onboarded, &city)
	// Fall back to a heuristic if onboarding_complete is untrusted.
	if !onboarded {
		onboarded = p.Capacity > 0 && city != ""
	}

	// Resolve the owner user row (created lazily on first callback).
	var userID, userName string
	_ = h.Pool.QueryRowContext(r.Context(), `
		SELECT id, COALESCE(full_name, '') FROM users
		WHERE provider_id = ? AND role = 'provider_admin' AND deleted_at IS NULL
		ORDER BY created_at ASC LIMIT 1`, pid).Scan(&userID, &userName)

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		// Provider-shape fields (backend consumers).
		"id":                  p.ID,
		"name":                p.Name,
		"legal_name":          p.LegalName,
		"state_code":          p.StateCode,
		"license_number":      p.LicenseNumber,
		"owner_email":         p.OwnerEmail,
		"phone":               p.OwnerPhone,
		"capacity":            p.Capacity,
		"timezone":            p.Timezone,
		"stripe_customer_id":  p.StripeCustID,
		"created_at":          p.CreatedAt,
		"updated_at":          p.UpdatedAt,
		// SessionUser-shape fields (frontend consumers).
		"email":               p.OwnerEmail,
		"fullName":            userName,
		"providerId":          p.ID,
		"role":                "owner",
		"onboardingComplete":  onboarded,
	})
}

// PATCH /api/me
func (h *ProviderHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in struct {
		Name          *string `json:"name"`
		LegalName     *string `json:"legal_name"`
		LicenseNumber *string `json:"license_number"`
		OwnerPhone    *string `json:"owner_phone"`
		Capacity      *int    `json:"capacity"`
		Timezone      *string `json:"timezone"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	_, err := h.Pool.ExecContext(r.Context(), `
		UPDATE providers
		SET name          = COALESCE(?, name),
		    legal_name    = COALESCE(?, legal_name),
		    license_number= COALESCE(?, license_number),
		    owner_phone   = COALESCE(?, owner_phone),
		    capacity      = COALESCE(?, capacity),
		    timezone      = COALESCE(?, timezone),
		    updated_at    = CURRENT_TIMESTAMP
		WHERE id = ?`,
		in.Name, in.LegalName, in.LicenseNumber, in.OwnerPhone, in.Capacity, in.Timezone, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, fmt.Errorf("update provider: %w", err)))
		return
	}
	auditlog.EmitMeUpdate(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), r)
	h.Me(w, r)
}

// DELETE /api/providers/me — soft-delete the provider (start the 90-day
// retention clock). Requires a typed confirmation (body {"confirm":"DELETE"})
// to avoid accidental one-click wipeouts. The retention worker will
// hard-delete the tenant's data after the grace window elapses.
//
// This handler does NOT log the user out — the session stays valid so the
// Settings UI can render a "Scheduled for deletion on <date>" banner on the
// next request. Re-subscribing (or clearing deleted_at via support) cancels
// the pending purge.
func (h *ProviderHandler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.Confirm != "DELETE" {
		httpx.RenderError(w, r, httpx.BadRequestf(`confirmation required: send {"confirm":"DELETE"}`))
		return
	}
	_, err := h.Pool.ExecContext(r.Context(), `
		UPDATE providers
		   SET deleted_at = CURRENT_TIMESTAMP,
		       canceled_at = COALESCE(canceled_at, CURRENT_TIMESTAMP),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, fmt.Errorf("soft-delete provider: %w", err)))
		return
	}
	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    mw.UserIDFrom(r.Context()),
		Action:     "provider.deletion_requested",
		TargetKind: auditlog.TargetKindProvider,
		TargetID:   pid,
		IP:         auditlog.ClientIP(r),
		UserAgent:  r.UserAgent(),
	})
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"status":             "scheduled_for_deletion",
		"grace_period_days":  90,
		"message":            "Your account is scheduled for deletion. All data will be permanently removed in 90 days unless you contact support to cancel.",
	})
}

// POST /api/provider/onboarding
//
// Completes the onboarding wizard: persists all facility fields, bulk-inserts
// any staff and children drafts the wizard collected, and flips
// onboarding_complete = 1. Returns the provider in the frontend's expected
// camelCase shape (see frontend/src/api/providers.ts ProviderSchema).
//
// The whole operation runs in a single transaction so a half-written row
// can't leave the tenant in a partially-onboarded state on crash.
func (h *ProviderHandler) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}

	type staffDraft struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Email     string `json:"email"`
		Role      string `json:"role"`
	}
	type childDraft struct {
		FirstName   string `json:"firstName"`
		LastName    string `json:"lastName"`
		DateOfBirth string `json:"dateOfBirth"` // YYYY-MM-DD
		ParentEmail string `json:"parentEmail"`
	}
	type agesServed struct {
		MinMonths int `json:"minMonths"`
		MaxMonths int `json:"maxMonths"`
	}
	var in struct {
		StateCode        string       `json:"stateCode"`
		LicenseType      string       `json:"licenseType"`
		LicenseNumber    string       `json:"licenseNumber"`
		Name             string       `json:"name"`
		Address1         string       `json:"address1"`
		Address2         string       `json:"address2"`
		City             string       `json:"city"`
		StateRegion      string       `json:"stateRegion"`
		PostalCode       string       `json:"postalCode"`
		Capacity         int          `json:"capacity"`
		AgesServedMonths agesServed   `json:"agesServedMonths"`
		Staff            []staffDraft `json:"staff"`
		Children         []childDraft `json:"children"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}

	// Validate the tight invariants. Free-text fields (address, name) we
	// trust to be non-empty; the frontend already enforces required-ness.
	stateUpper := strings.ToUpper(strings.TrimSpace(in.StateCode))
	switch stateUpper {
	case "CA", "TX", "FL":
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("stateCode must be CA, TX, or FL"))
		return
	}
	switch in.LicenseType {
	case "center", "family_home":
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("licenseType must be center or family_home"))
		return
	}
	if in.Name == "" || in.Address1 == "" || in.City == "" || in.PostalCode == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("name, address1, city, postalCode are required"))
		return
	}
	if in.Capacity <= 0 {
		httpx.RenderError(w, r, httpx.BadRequestf("capacity must be > 0"))
		return
	}
	// state_abbr constraint in 000001 requires exactly 2 chars when non-null.
	stateRegion := strings.ToUpper(strings.TrimSpace(in.StateRegion))
	if stateRegion == "" {
		stateRegion = stateUpper
	}
	if len(stateRegion) != 2 {
		httpx.RenderError(w, r, httpx.BadRequestf("stateRegion must be a 2-letter state abbreviation"))
		return
	}
	if in.AgesServedMonths.MinMonths < 0 || in.AgesServedMonths.MaxMonths < 0 ||
		in.AgesServedMonths.MinMonths > in.AgesServedMonths.MaxMonths {
		httpx.RenderError(w, r, httpx.BadRequestf("agesServedMonths must be a non-negative range with min <= max"))
		return
	}

	tx, err := h.Pool.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	// On any early return below we rollback; the final Commit wins.
	defer func() { _ = tx.Rollback() }()

	// Update provider — mirror both legacy (state, legal_name, address_line1/2)
	// and handler-era (state_code, name) columns so downstream readers find
	// what they expect regardless of which spelling they use.
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE providers
		   SET name                 = ?,
		       legal_name           = ?,
		       state                = ?,
		       state_code           = ?,
		       license_type         = ?,
		       license_number       = NULLIF(?, ''),
		       address_line1        = ?,
		       address_line2        = NULLIF(?, ''),
		       city                 = ?,
		       state_abbr           = ?,
		       postal_code          = ?,
		       capacity             = ?,
		       min_age_months       = ?,
		       max_age_months       = ?,
		       onboarding_complete  = 1,
		       updated_at           = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		in.Name, in.Name, stateUpper, stateUpper,
		in.LicenseType, in.LicenseNumber,
		in.Address1, in.Address2, in.City, stateRegion, in.PostalCode,
		in.Capacity, in.AgesServedMonths.MinMonths, in.AgesServedMonths.MaxMonths,
		pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, fmt.Errorf("update provider: %w", err)))
		return
	}

	// Bulk-insert staff drafts. role is TEXT with no DB-level CHECK, so the
	// wizard's vocabulary (director/lead_teacher/assistant/aide/cook/other)
	// flows through unchanged. Empty email stays NULL to avoid collisions
	// with the email_idx and to preserve the "email is optional" contract.
	for _, s := range in.Staff {
		if strings.TrimSpace(s.FirstName) == "" || strings.TrimSpace(s.LastName) == "" {
			continue
		}
		role := s.Role
		if role == "" {
			role = "lead_teacher"
		}
		id := base62.NewID()[:22]
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO staff (id, provider_id, first_name, last_name, role, email, hired_on, hire_date, status, created_at, updated_at)
			VALUES (?,?,?,?,?,NULLIF(?, ''),CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
			id, pid, strings.TrimSpace(s.FirstName), strings.TrimSpace(s.LastName), role,
			strings.TrimSpace(s.Email)); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, fmt.Errorf("insert staff: %w", err)))
			return
		}
	}

	// Bulk-insert children drafts. date_of_birth is required; we already gate
	// on that client-side but double-check here so a malformed CSV import
	// can't corrupt the batch.
	for _, c := range in.Children {
		fn := strings.TrimSpace(c.FirstName)
		ln := strings.TrimSpace(c.LastName)
		dobRaw := strings.TrimSpace(c.DateOfBirth)
		if fn == "" || ln == "" || dobRaw == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", dobRaw); err != nil {
			httpx.RenderError(w, r, httpx.BadRequestf("child %s %s: invalid dateOfBirth (expected YYYY-MM-DD)", fn, ln))
			return
		}
		id := base62.NewID()[:22]
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth,
			                     enrollment_date, enroll_date, guardians, parent_email, status,
			                     created_at, updated_at)
			VALUES (?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'[]',NULLIF(?, ''),'enrolled',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
			id, pid, fn, ln, dobRaw, strings.TrimSpace(c.ParentEmail)); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, fmt.Errorf("insert child: %w", err)))
			return
		}
	}

	if err := tx.Commit(); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    mw.UserIDFrom(r.Context()),
		Action:     "provider.onboarding_complete",
		TargetKind: auditlog.TargetKindProvider,
		TargetID:   pid,
		Metadata: map[string]any{
			"state_code":    stateUpper,
			"license_type":  in.LicenseType,
			"capacity":      in.Capacity,
			"staff_count":   len(in.Staff),
			"children_count": len(in.Children),
		},
		IP:        auditlog.ClientIP(r),
		UserAgent: r.UserAgent(),
	})

	// Re-read to emit a canonical response shape matching frontend
	// ProviderSchema (camelCase, agesServedMonths nested).
	var (
		outID, outName, outStateCode, outLicenseType, outAddr1, outCity, outStateRegion, outPostal string
		outLicenseNumber, outAddr2                                                                 sql.NullString
		outCapacity, outMinMonths, outMaxMonths                                                    int
		outOnboarded                                                                                bool
		outCreatedStr                                                                               string
	)
	err = h.Pool.QueryRowContext(r.Context(), `
		SELECT id,
		       COALESCE(name, legal_name, '') AS name,
		       COALESCE(state_code, state, '') AS state_code,
		       COALESCE(license_type, '') AS license_type,
		       license_number,
		       COALESCE(address_line1, '') AS address1,
		       address_line2,
		       COALESCE(city, '') AS city,
		       COALESCE(state_abbr, '') AS state_region,
		       COALESCE(postal_code, '') AS postal_code,
		       capacity,
		       COALESCE(min_age_months, 0),
		       COALESCE(max_age_months, 0),
		       COALESCE(onboarding_complete, 0),
		       created_at
		  FROM providers WHERE id = ?`, pid).
		Scan(&outID, &outName, &outStateCode, &outLicenseType, &outLicenseNumber,
			&outAddr1, &outAddr2, &outCity, &outStateRegion, &outPostal,
			&outCapacity, &outMinMonths, &outMaxMonths, &outOnboarded, &outCreatedStr)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	var licenseNumOut, addr2Out any
	if outLicenseNumber.Valid {
		licenseNumOut = outLicenseNumber.String
	} else {
		licenseNumOut = nil
	}
	if outAddr2.Valid {
		addr2Out = outAddr2.String
	} else {
		addr2Out = nil
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"id":            outID,
		"name":          outName,
		"stateCode":     outStateCode,
		"licenseType":   outLicenseType,
		"licenseNumber": licenseNumOut,
		"address1":      outAddr1,
		"address2":      addr2Out,
		"city":          outCity,
		"stateRegion":   outStateRegion,
		"postalCode":    outPostal,
		"capacity":      outCapacity,
		"agesServedMonths": map[string]int{
			"minMonths": outMinMonths,
			"maxMonths": outMaxMonths,
		},
		"onboardingComplete": outOnboarded,
		"createdAt":          parseSQLiteTime(outCreatedStr).Format(time.RFC3339),
	})
}

// POST /api/children/{id}/portal-link — mint a parent-upload magic link
// for a child. Admin-only. Returns { url, expires_at } so the admin can copy
// and send the link out-of-band (SMS, email from their own account, etc.).
// If the child's parent_email is on file AND an Emailer is configured, we
// also send the link via SES as a convenience — controlled by ?send=email
// in the query string.
func (h *ProviderHandler) MintParentPortalLink(w http.ResponseWriter, r *http.Request) {
	h.mintPortalLink(w, r, magiclink.KindParentUpload, "child")
}

// POST /api/staff/{id}/portal-link — mint a staff-upload magic link. Mirror
// of MintParentPortalLink for staff certifications.
func (h *ProviderHandler) MintStaffPortalLink(w http.ResponseWriter, r *http.Request) {
	h.mintPortalLink(w, r, magiclink.KindStaffUpload, "staff")
}

// mintPortalLink is the shared implementation for both child and staff
// portal link generation. Validates subject belongs to the caller's provider,
// mints a sliding-TTL magic link, and returns { url, expires_at }.
func (h *ProviderHandler) mintPortalLink(w http.ResponseWriter, r *http.Request, kind magiclink.Kind, subjectKind string) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	subjectID := chi.URLParam(r, "id")
	if subjectID == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("subject id required"))
		return
	}

	// Verify subject belongs to the caller's provider. Two different tables;
	// pick the right one by subjectKind. Using a prepared query per kind is
	// simpler than a UNION and keeps the FK semantics explicit.
	var (
		firstName, lastName, email string
		found                       bool
	)
	switch subjectKind {
	case "child":
		var emailNull sql.NullString
		err := h.Pool.QueryRowContext(r.Context(),
			`SELECT first_name, last_name, parent_email FROM children WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
			subjectID, pid).Scan(&firstName, &lastName, &emailNull)
		if errors.Is(err, sql.ErrNoRows) {
			httpx.RenderError(w, r, httpx.ErrNotFound)
			return
		}
		if err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		if emailNull.Valid {
			email = emailNull.String
		}
		found = true
	case "staff":
		var emailNull sql.NullString
		err := h.Pool.QueryRowContext(r.Context(),
			`SELECT first_name, last_name, email FROM staff WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
			subjectID, pid).Scan(&firstName, &lastName, &emailNull)
		if errors.Is(err, sql.ErrNoRows) {
			httpx.RenderError(w, r, httpx.ErrNotFound)
			return
		}
		if err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		if emailNull.Valid {
			email = emailNull.String
		}
		found = true
	}
	if !found {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}

	token, path, err := h.Magic.Generate(r.Context(), kind, pid, subjectID, 0)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	_ = token // the raw token is embedded in path via PathFor; we return the URL, not the token alone.
	url := h.FrontendBase + path
	expiresAt := time.Now().Add(magiclink.TTLFor(kind)).UTC()

	// Optional: email the recipient if ?send=email AND we have an address AND
	// an emailer is wired. This is a nice-to-have; the primary contract is
	// returning the URL so the admin can send it themselves.
	emailed := false
	if r.URL.Query().Get("send") == "email" && email != "" && h.Emailer != nil {
		actionText := "Upload your child's required forms"
		if subjectKind == "staff" {
			actionText = "Upload your required certifications"
		}
		sub, html, text := notify.RenderMagicLinkEmail(notify.MagicLinkEmailData{
			RecipientName: firstName + " " + lastName,
			ActionText:    actionText,
			URL:           url,
			ExpiresIn:     "7 days",
		})
		if err := h.Emailer.Send(r.Context(), notify.EmailMessage{
			To: email, Subject: sub, HTMLBody: html, PlainBody: text,
		}); err != nil {
			h.Log.Warn("portal link: email send", "err", err, "subject", subjectKind, "id", subjectID)
		} else {
			emailed = true
		}
	}

	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    mw.UserIDFrom(r.Context()),
		Action:     "portal.link_generated",
		TargetKind: subjectKind,
		TargetID:   subjectID,
		Metadata: map[string]any{
			"kind":    string(kind),
			"emailed": emailed,
		},
		IP:        auditlog.ClientIP(r),
		UserAgent: r.UserAgent(),
	})

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"url":         url,
		"expires_at":  expiresAt.Format(time.RFC3339),
		"subject_id":  subjectID,
		"subject_kind": subjectKind,
		"emailed":     emailed,
	})
}

// LookupSession implements middleware.SessionReader.
func (h *ProviderHandler) LookupSession(ctx context.Context, token string) (providerID, userID string, err error) {
	err = h.Pool.QueryRowContext(ctx, `
		SELECT provider_id, user_id FROM sessions WHERE id = ? AND expires_at > CURRENT_TIMESTAMP`, token).
		Scan(&providerID, &userID)
	if err != nil {
		return "", "", err
	}
	return providerID, userID, nil
}

// Logout clears the session cookie and deletes the session row.
func (h *ProviderHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("ck_sess"); err == nil && c.Value != "" {
		_, _ = h.Pool.ExecContext(r.Context(), `DELETE FROM sessions WHERE id = ?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "ck_sess", Value: "", Domain: h.CookieDomain, Path: "/",
		Expires: time.Unix(0, 0), HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
