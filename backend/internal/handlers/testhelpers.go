package handlers

// Test-only helper endpoints for LIVE end-to-end tests.
//
// These routes are ONLY mounted when APP_ENV != "production" (see router.go).
// They expose internals (raw magic-link tokens, session creation) that would
// be unsafe in production — the mount guard is the only thing protecting us.
//
// If you need a new test affordance, add it here rather than wiring it into
// the production handlers. Keep the surface minimal.

import (
	"database/sql"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
)

// TestHelperHandler exposes a few read/write utilities tests need to walk the
// magic-link flow without going through email. Mount with care.
type TestHelperHandler struct {
	Pool         *sql.DB
	Magic        *magiclink.Service
	CookieDomain string
	SecureCookie bool
	Log          *slog.Logger
}

// GET /api/test/latest-magic-link?email=<owner_email>
//
// Returns the most recent unconsumed magic-link token path for the provider
// whose owner_email matches. Because Magic.Generate returns the plaintext
// token and we never persist it, we can't look up the plaintext after the
// fact — instead this helper generates a FRESH signin token for the matching
// provider and returns it. That's fine for tests: consuming the new token is
// functionally equivalent to consuming the one that was "emailed."
func (h *TestHelperHandler) LatestMagicLink(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	if email == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("email query param required"))
		return
	}
	var providerID string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT id FROM providers WHERE owner_email = ? AND deleted_at IS NULL`, email).Scan(&providerID)
	if err != nil {
		httpx.RenderError(w, r, httpx.NotFoundf("no provider for email %q", email))
		return
	}
	// Mint a fresh signup token — the kind doesn't matter for the callback
	// since both signin and signup are accepted there.
	token, path, err := h.Magic.Generate(r.Context(), magiclink.KindProviderSignup, providerID, providerID, 0)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"token":       token,
		"path":        path,
		"provider_id": providerID,
	})
}

// POST /api/test/session?email=<owner_email>
//
// Creates a session row directly and sets the ck_sess cookie on the response.
// This is the fastest way for a Playwright test to land in an authenticated
// browser state without doing the full magic-link round trip.
func (h *TestHelperHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	if email == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("email query param required"))
		return
	}
	var providerID string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT id FROM providers WHERE owner_email = ? AND deleted_at IS NULL`, email).Scan(&providerID)
	if err != nil {
		httpx.RenderError(w, r, httpx.NotFoundf("no provider for email %q", email))
		return
	}
	sessID := base62.NewID()
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, datetime('now', '+14 days'), CURRENT_TIMESTAMP)`,
		sessID, providerID, providerID); err != nil {
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
	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"session_id":  sessID,
		"provider_id": providerID,
	})
}

// POST /api/test/reset — delete all provider-scoped data. This is a blunt
// reset useful between test suites. We DELETE FROM each data table; schema
// is left alone.
func (h *TestHelperHandler) Reset(w http.ResponseWriter, r *http.Request) {
	tables := []string{
		"audit_log",
		"sessions",
		"magic_link_tokens",
		"data_exports",
		"compliance_snapshots",
		"chase_events",
		"document_chase_sends",
		"document_ocr_results",
		"document_unassigned_photos",
		"documents",
		"staff_certifications_required",
		"child_documents_required",
		"children",
		"staff",
		"inspection_items",
		"inspection_runs",
		"drills",
		"facility_postings",
		"signatures",
		"sign_sessions",
		"policy_acceptances",
		"providers",
	}
	for _, t := range tables {
		_, _ = h.Pool.ExecContext(r.Context(), "DELETE FROM "+t)
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// unused import guard — kept so the file compiles even if encoding/hex is
// removed from future additions.
var _ = hex.EncodeToString
