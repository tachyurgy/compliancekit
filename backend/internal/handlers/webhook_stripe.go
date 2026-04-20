package handlers

import (
	"log/slog"
	"net/http"

	"database/sql"

	"github.com/tachyurgy/compliancekit/backend/internal/billing"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

type StripeWebhookHandler struct {
	Billing *billing.Service
	Log     *slog.Logger
}

// POST /webhooks/stripe
// MUST be mounted on a route WITHOUT JSON middleware — Stripe verifies the raw body.
func (h *StripeWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if err := h.Billing.HandleWebhook(r); err != nil {
		h.Log.Warn("stripe webhook", "err", err)
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrBadRequest, err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

type BillingHandler struct {
	Pool        *sql.DB
	Billing     *billing.Service
	StripePrice string
	Log         *slog.Logger
}

// POST /api/billing/checkout { "promo_code": "..." }
func (h *BillingHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PromoCode string `json:"promo_code"`
	}
	_ = httpx.DecodeJSON(r, &in)

	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var email, name string
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT owner_email, name FROM providers WHERE id = ?`, pid).Scan(&email, &name); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	url, err := h.Billing.CreateCheckoutSession(r.Context(), pid, h.StripePrice, email, name, in.PromoCode)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"url": url})
}

// POST /api/billing/portal
func (h *BillingHandler) Portal(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	url, err := h.Billing.CustomerPortalURL(r.Context(), pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"url": url})
}
