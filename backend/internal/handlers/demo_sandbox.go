package handlers

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/tachyurgy/compliancekit/backend/internal/demoaccount"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
)

// DemoSandboxHandler backs the one-click "Explore the live demo" button. It is
// only mounted when the server runs with DEMO_MODE=1 (the public demo); a
// normal deployment never exposes it.
type DemoSandboxHandler struct {
	Pool         *sql.DB
	Provisioner  *demoaccount.Provisioner
	CookieDomain string
	SecureCookie bool
	Log          *slog.Logger
}

// POST /api/auth/demo — provisions a private, pre-seeded sandbox tenant and
// signs the caller into it (24h session, no "remember me").
func (h *DemoSandboxHandler) Start(w http.ResponseWriter, r *http.Request) {
	acct, err := h.Provisioner.CreateSandbox(r.Context())
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if _, err := EstablishSession(w, r, h.Pool, h.CookieDomain, h.SecureCookie,
		acct.ProviderID, acct.UserID, false); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	h.Log.Info("demo sandbox started", "provider_id", acct.ProviderID)
	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"status":      "ok",
		"provider_id": acct.ProviderID,
		"user_id":     acct.UserID,
	})
}
