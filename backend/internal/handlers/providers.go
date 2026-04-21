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

// POST /api/auth/signup
// body: { "name": "Sunshine Daycare", "owner_email": "...", "state_code": "CA" }
// Creates a provider stub and emails a signup magic link.
func (h *ProviderHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		OwnerEmail string `json:"owner_email"`
		StateCode  string `json:"state_code"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	in.OwnerEmail = strings.ToLower(strings.TrimSpace(in.OwnerEmail))
	if in.Name == "" || in.OwnerEmail == "" || in.StateCode == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("name, owner_email, and state_code are required"))
		return
	}
	switch strings.ToUpper(in.StateCode) {
	case "CA", "TX", "FL":
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("state_code must be one of CA, TX, FL (MVP scope)"))
		return
	}

	// Upsert provider by owner_email. Populate both the legacy canonical
	// columns (legal_name, state) from migration 000001 and the handler-era
	// columns (name, state_code) added by migration 000012, so reads work from
	// either schema view.
	providerID := base62.NewID()[:22]
	stateUpper := strings.ToUpper(in.StateCode)
	_, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'America/Los_Angeles', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (owner_email) DO UPDATE SET
			name = EXCLUDED.name,
			legal_name = EXCLUDED.legal_name,
			state = EXCLUDED.state,
			state_code = EXCLUDED.state_code,
			updated_at = CURRENT_TIMESTAMP`,
		providerID, in.Name, in.Name, in.OwnerEmail, stateUpper, stateUpper)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	// Re-read to get the canonical ID (may have been pre-existing).
	var actualID string
	_ = h.Pool.QueryRowContext(r.Context(), `SELECT id FROM providers WHERE owner_email = ?`, in.OwnerEmail).Scan(&actualID)

	token, path, err := h.Magic.Generate(r.Context(), magiclink.KindProviderSignup, actualID, actualID, 0)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	url := h.FrontendBase + path
	if h.Emailer != nil {
		sub, html, text := notify.RenderMagicLinkEmail(notify.MagicLinkEmailData{
			RecipientName: in.Name, ActionText: "Finish creating your ComplianceKit account",
			URL: url, ExpiresIn: "15 minutes",
		})
		if err := h.Emailer.Send(r.Context(), notify.EmailMessage{
			To: in.OwnerEmail, Subject: sub, HTMLBody: html, PlainBody: text,
		}); err != nil {
			h.Log.Warn("signup: email send", "err", err)
		}
	}
	_ = token
	auditlog.EmitSignup(r.Context(), h.Pool, actualID, map[string]any{
		"state_code":  stateUpper,
		"owner_email": in.OwnerEmail,
	}, r)
	httpx.RenderJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
}

// POST /api/auth/signin { "email": "..." }  -> emails a 15-min magic link
func (h *ProviderHandler) Signin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("email required"))
		return
	}
	var providerID, name string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT id, name FROM providers WHERE owner_email = ? AND deleted_at IS NULL`, email).Scan(&providerID, &name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Don't leak whether email exists; pretend success.
			httpx.RenderJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
			return
		}
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	_, path, err := h.Magic.Generate(r.Context(), magiclink.KindProviderSignin, providerID, providerID, 0)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	url := h.FrontendBase + path
	if h.Emailer != nil {
		sub, html, text := notify.RenderMagicLinkEmail(notify.MagicLinkEmailData{
			RecipientName: name, ActionText: "Sign in to ComplianceKit",
			URL: url, ExpiresIn: "15 minutes",
		})
		_ = h.Emailer.Send(r.Context(), notify.EmailMessage{To: email, Subject: sub, HTMLBody: html, PlainBody: text})
	}
	httpx.RenderJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
}

// GET /api/auth/callback?t=... — exchanges magic link for a session cookie.
func (h *ProviderHandler) Callback(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("t")
	if token == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	claim, err := h.Magic.Consume(r.Context(), token)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrUnauthorized, err))
		return
	}
	if claim.Kind != magiclink.KindProviderSignup && claim.Kind != magiclink.KindProviderSignin {
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}
	// Ensure a user row exists for this provider. Signup only creates the
	// providers row; the owner user is materialized lazily on first callback.
	// We look up by (provider_id, email==owner_email, role='provider_admin').
	var userID string
	err = h.Pool.QueryRowContext(r.Context(), `
		SELECT u.id FROM users u
		JOIN providers p ON p.id = u.provider_id
		WHERE u.provider_id = ? AND u.email = p.owner_email AND u.role = 'provider_admin'
		  AND u.deleted_at IS NULL
		LIMIT 1`, claim.ProviderID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		var ownerEmail, name string
		if err := h.Pool.QueryRowContext(r.Context(),
			`SELECT owner_email, COALESCE(name, legal_name, '') FROM providers WHERE id = ?`,
			claim.ProviderID).Scan(&ownerEmail, &name); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		userID = base62.NewID()[:22]
		if _, err := h.Pool.ExecContext(r.Context(), `
			INSERT INTO users (id, provider_id, email, full_name, role, email_verified_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'provider_admin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			userID, claim.ProviderID, ownerEmail, name); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
	} else if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	sessID := base62.NewID()
	// Store session. SQLite lacks INTERVAL literals; use datetime(...).
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, datetime('now', '+14 days'), CURRENT_TIMESTAMP)`,
		sessID, claim.ProviderID, userID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
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
	auditlog.EmitLogin(r.Context(), h.Pool, claim.ProviderID, claim.ProviderID, r)
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"status": "ok", "provider_id": claim.ProviderID})
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
		SELECT id, COALESCE(name, legal_name, '') AS name,
		       COALESCE(legal_name, '') AS legal_name,
		       COALESCE(state_code, state, '') AS state_code,
		       COALESCE(license_number, '') AS license_number,
		       COALESCE(owner_email, '') AS owner_email,
		       COALESCE(phone, '') AS phone,
		       capacity, timezone,
		       COALESCE(stripe_customer_id, '') AS stripe_cust,
		       created_at, updated_at
		FROM providers WHERE id = ?`, pid).
		Scan(&p.ID, &p.Name, &p.LegalName, &p.StateCode, &p.LicenseNumber, &p.OwnerEmail,
			&p.OwnerPhone, &p.Capacity, &p.Timezone, &p.StripeCustID, &createdStr, &updatedStr)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	p.CreatedAt = parseSQLiteTime(createdStr)
	p.UpdatedAt = parseSQLiteTime(updatedStr)
	httpx.RenderJSON(w, http.StatusOK, p)
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
