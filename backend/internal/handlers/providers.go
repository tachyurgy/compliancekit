package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
	"github.com/tachyurgy/compliancekit/backend/internal/notify"
)

type ProviderHandler struct {
	Pool         *pgxpool.Pool
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

	// Upsert provider by owner_email. State code normalized.
	providerID := base62.NewID()[:22]
	_, err := h.Pool.Exec(r.Context(), `
		INSERT INTO providers (id, name, owner_email, state_code, capacity, timezone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 0, 'America/Los_Angeles', NOW(), NOW())
		ON CONFLICT (owner_email) DO UPDATE SET name = EXCLUDED.name, state_code = EXCLUDED.state_code, updated_at = NOW()`,
		providerID, in.Name, in.OwnerEmail, strings.ToUpper(in.StateCode))
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	// Re-read to get the canonical ID (may have been pre-existing).
	var actualID string
	_ = h.Pool.QueryRow(r.Context(), `SELECT id FROM providers WHERE owner_email = $1`, in.OwnerEmail).Scan(&actualID)

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
	err := h.Pool.QueryRow(r.Context(),
		`SELECT id, name FROM providers WHERE owner_email = $1 AND deleted_at IS NULL`, email).Scan(&providerID, &name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	sessID := base62.NewID()
	// Store session. Expect sessions table (provider_id, session_id, expires_at).
	if _, err := h.Pool.Exec(r.Context(), `
		INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		VALUES ($1, $2, $2, NOW() + INTERVAL '14 days', NOW())`,
		sessID, claim.ProviderID); err != nil {
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
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id, name, COALESCE(legal_name, ''), state_code, COALESCE(license_number, ''),
		       owner_email, COALESCE(owner_phone, ''), capacity, timezone,
		       COALESCE(stripe_customer_id, ''), created_at, updated_at
		FROM providers WHERE id = $1`, pid).
		Scan(&p.ID, &p.Name, &p.LegalName, &p.StateCode, &p.LicenseNumber, &p.OwnerEmail,
			&p.OwnerPhone, &p.Capacity, &p.Timezone, &p.StripeCustID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
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
	_, err := h.Pool.Exec(r.Context(), `
		UPDATE providers
		SET name          = COALESCE($2, name),
		    legal_name    = COALESCE($3, legal_name),
		    license_number= COALESCE($4, license_number),
		    owner_phone   = COALESCE($5, owner_phone),
		    capacity      = COALESCE($6, capacity),
		    timezone      = COALESCE($7, timezone),
		    updated_at    = NOW()
		WHERE id = $1`,
		pid, in.Name, in.LegalName, in.LicenseNumber, in.OwnerPhone, in.Capacity, in.Timezone)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, fmt.Errorf("update provider: %w", err)))
		return
	}
	h.Me(w, r)
}

// LookupSession implements middleware.SessionReader.
func (h *ProviderHandler) LookupSession(ctx context.Context, token string) (providerID, userID string, err error) {
	err = h.Pool.QueryRow(ctx, `
		SELECT provider_id, user_id FROM sessions WHERE id = $1 AND expires_at > NOW()`, token).
		Scan(&providerID, &userID)
	if err != nil {
		return "", "", err
	}
	return providerID, userID, nil
}

// Logout clears the session cookie and deletes the session row.
func (h *ProviderHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("ck_sess"); err == nil && c.Value != "" {
		_, _ = h.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE id = $1`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "ck_sess", Value: "", Domain: h.CookieDomain, Path: "/",
		Expires: time.Unix(0, 0), HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
