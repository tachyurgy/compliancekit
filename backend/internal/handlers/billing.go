// Package handlers — billing.go
//
// REQ-002 billing handlers beyond the legacy CreateCheckout/Portal that
// already lived in webhook_stripe.go. We add:
//
//   - POST /api/billing/checkout-session  — REQ-002 shape: { plan: "monthly_99"|"annual_948" } returns Stripe URL
//   - POST /api/billing/portal-session    — REQ-002 alias of /portal
//   - GET  /api/billing/subscription      — frontend reads this on /settings/billing
//
// We deliberately keep the legacy /api/billing/checkout + /api/billing/portal
// routes in webhook_stripe.go so existing tests/UI continue to work; the new
// shape is additive.
package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/billing"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/notify"
)

// BillingV2Handler hosts the REQ-002 billing shape.
type BillingV2Handler struct {
	Pool         *sql.DB
	Billing      *billing.Service
	Emailer      *notify.Emailer
	FrontendBase string
	AppBase      string
	Log          *slog.Logger
}

// POST /api/billing/checkout-session
//
// Body: { "plan": "monthly_99" | "annual_948" }
// Returns: { "url": "https://checkout.stripe.com/..." }
//
// REQ-002 #3: 14-day trial with card-on-file; success URL goes to
// /onboarding/legal so the operator captures the ToS/DPA before they hit
// the dashboard.
func (h *BillingV2Handler) CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in struct {
		Plan string `json:"plan"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	plan := strings.TrimSpace(in.Plan)
	if plan == "" {
		// REQ-002: annual is the default if the caller doesn't specify.
		plan = "annual_948"
	}
	priceID := h.Billing.PriceIDFor(plan)
	if priceID == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("unknown plan: %s", plan))
		return
	}

	var email, name string
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(owner_email,''), COALESCE(name, legal_name, '') FROM providers WHERE id = ?`,
		pid).Scan(&email, &name); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	url, err := h.Billing.CreateCheckoutSessionV2(r.Context(), billing.CheckoutInput{
		ProviderID: pid,
		PriceID:    priceID,
		Email:      email,
		Name:       name,
		TrialDays:  14,
	})
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Stash the chosen plan locally so the dashboard can render it before
	// the webhook lands. The webhook will overwrite if Stripe disagrees.
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE providers SET plan = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		plan, pid); err != nil {
		h.Log.Warn("billing: persist plan", "err", err)
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]string{"url": url, "plan": plan})
}

// POST /api/billing/portal-session — REQ-002 alias of /api/billing/portal.
func (h *BillingV2Handler) CreatePortalSession(w http.ResponseWriter, r *http.Request) {
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

// GET /api/billing/subscription — frontend reads this on /settings/billing.
//
// We return null (HTTP 200, body = null) if the provider has no subscription
// row yet — the UI shows the "pick a plan" CTA in that case.
func (h *BillingV2Handler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	type subOut struct {
		Plan              string  `json:"plan"`
		Status            string  `json:"status"`
		CurrentPeriodEnd  *string `json:"currentPeriodEnd"`
		CancelAtPeriodEnd bool    `json:"cancelAtPeriodEnd"`
		PriceCents        int     `json:"priceCents"`
		Seats             *int    `json:"seats"`
		TrialEndsAt       *string `json:"trialEndsAt,omitempty"`
	}
	var (
		plan        sql.NullString
		status      sql.NullString
		periodEnd   sql.NullString
		trialEnd    sql.NullString
		stripeSubID sql.NullString
	)
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT plan, subscription_status, current_period_end_at, trial_ends_at, stripe_subscription_id
		  FROM providers WHERE id = ?`, pid).Scan(&plan, &status, &periodEnd, &trialEnd, &stripeSubID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if !status.Valid && !stripeSubID.Valid {
		// No Stripe activity yet — return null so the frontend renders the
		// "Start your free trial" card.
		httpx.RenderJSON(w, http.StatusOK, nil)
		return
	}
	out := subOut{
		Plan:   coalesce(plan.String, "monthly_99"),
		Status: coalesce(status.String, "incomplete"),
	}
	out.PriceCents = priceCentsForPlan(out.Plan)
	if periodEnd.Valid {
		s := periodEnd.String
		out.CurrentPeriodEnd = &s
	}
	if trialEnd.Valid {
		s := trialEnd.String
		out.TrialEndsAt = &s
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// PostCheckoutNotifier implementation — sends the welcome email and writes
// trial_ends_at on the provider row. Wired via Service.SetNotifier in main.go.
type CheckoutNotifier struct {
	Pool    *sql.DB
	Emailer *notify.Emailer
	AppBase string
	Log     *slog.Logger
}

func (n *CheckoutNotifier) OnCheckoutCompleted(providerID, sessionID string) {
	if n == nil || n.Pool == nil {
		return
	}
	// We accept the brief context-less call here because billing.Service
	// already returns 2xx to Stripe before invoking us in a goroutine.
	// Email send failures are logged, never retried (the user can resend
	// from /settings if it really matters).
	logger := n.Log
	if logger == nil {
		logger = slog.Default()
	}

	var email, name, fullName string
	if err := n.Pool.QueryRow(`
		SELECT COALESCE(p.owner_email,''),
		       COALESCE(p.name, p.legal_name, ''),
		       COALESCE((SELECT u.full_name FROM users u
		                  WHERE u.provider_id = p.id AND u.role = 'provider_admin'
		                    AND u.deleted_at IS NULL
		                  ORDER BY u.created_at ASC LIMIT 1), '')
		  FROM providers p WHERE p.id = ?`, providerID).Scan(&email, &name, &fullName); err != nil {
		logger.Warn("welcome: lookup provider", "provider_id", providerID, "err", err)
		return
	}

	// Approximate trial end. The webhook for customer.subscription.updated
	// will overwrite trial_ends_at with Stripe's authoritative value within
	// a few hundred ms; this fallback ensures the welcome email always has
	// a date to display.
	trialEnds := time.Now().UTC().Add(14 * 24 * time.Hour)
	var trialFromDB sql.NullString
	_ = n.Pool.QueryRow(`SELECT trial_ends_at FROM providers WHERE id = ?`, providerID).Scan(&trialFromDB)
	if trialFromDB.Valid && trialFromDB.String != "" {
		if t, err := time.Parse(time.RFC3339, trialFromDB.String); err == nil {
			trialEnds = t
		}
	}

	if n.Emailer == nil {
		logger.Info("welcome: emailer not configured; skipping send", "provider_id", providerID)
		return
	}

	subject, html, text, err := notify.RenderWelcome(notify.WelcomeData{
		FirstName:    pickFirstName(fullName, name),
		FacilityName: name,
		AppURL:       strings.TrimRight(n.AppBase, "/"),
		TrialEndsOn:  notify.FormatTrialEnds(trialEnds),
	})
	if err != nil {
		logger.Warn("welcome: render", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.Emailer.Send(ctx, notify.EmailMessage{
		To: email, Subject: subject, HTMLBody: html, PlainBody: text,
		ReferenceID: "welcome:" + sessionID,
	}); err != nil {
		logger.Warn("welcome: send", "provider_id", providerID, "err", err)
		return
	}
	logger.Info("welcome: sent", "provider_id", providerID, "to", email)
}

// PaymentFailedNotifier implements billing.PaymentFailedNotifier. It renders
// and sends the REQ-022 branded payment-failed email after Stripe's
// invoice.payment_failed webhook lands. Wired via Service.SetPaymentFailedNotifier
// in main.go.
type PaymentFailedNotifier struct {
	Pool    *sql.DB
	Emailer *notify.Emailer
	AppBase string
	Log     *slog.Logger
}

func (n *PaymentFailedNotifier) OnInvoicePaymentFailed(providerID, invoiceID, customerID string, amountDueCents int64, declineReason string) {
	if n == nil || n.Pool == nil {
		return
	}
	logger := n.Log
	if logger == nil {
		logger = slog.Default()
	}
	if n.Emailer == nil {
		logger.Info("payment-failed: emailer not configured; skipping send", "provider_id", providerID)
		return
	}

	var email, name, fullName string
	if err := n.Pool.QueryRow(`
		SELECT COALESCE(p.owner_email,''),
		       COALESCE(p.name, p.legal_name, ''),
		       COALESCE((SELECT u.full_name FROM users u
		                  WHERE u.provider_id = p.id AND u.role = 'provider_admin'
		                    AND u.deleted_at IS NULL
		                  ORDER BY u.created_at ASC LIMIT 1), '')
		  FROM providers p WHERE p.id = ?`, providerID).Scan(&email, &name, &fullName); err != nil {
		logger.Warn("payment-failed: lookup provider", "provider_id", providerID, "err", err)
		return
	}
	if email == "" {
		logger.Warn("payment-failed: provider has no owner_email", "provider_id", providerID)
		return
	}

	if declineReason == "" {
		declineReason = notify.DefaultDeclineReason
	}

	// Retry deadline = now + 7 days, matching the past-due grace window in
	// Service.HasActiveSubscription. Stripe's smart retries actually run on
	// a different cadence; the email's purpose is to give the operator a
	// concrete date by which they need to act, not to mirror Stripe exactly.
	retryDeadline := time.Now().UTC().Add(7 * 24 * time.Hour).Format("Jan 2, 2006")

	subject, html, text, err := notify.RenderPaymentFailed(notify.PaymentFailedData{
		FirstName:        pickFirstName(fullName, name),
		FacilityName:     name,
		CardBrand:        "Card on file", // brand/last4 aren't in the invoice.payment_failed event; the template suppresses 'ending in' when CardLast4 is empty
		CardLast4:        "",
		AmountDue:        notify.FormatAmount(amountDueCents),
		DeclineReason:    declineReason,
		RetryDeadline:    retryDeadline,
		UpdatePaymentURL: strings.TrimRight(n.AppBase, "/") + "/settings/billing",
	})
	if err != nil {
		logger.Warn("payment-failed: render", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.Emailer.Send(ctx, notify.EmailMessage{
		To: email, Subject: subject, HTMLBody: html, PlainBody: text,
		ReferenceID: "payment-failed:" + invoiceID,
	}); err != nil {
		logger.Warn("payment-failed: send", "provider_id", providerID, "err", err)
		return
	}
	logger.Info("payment-failed: sent", "provider_id", providerID, "to", email, "invoice", invoiceID)
}

// TrialConvertedNotifier implements billing.TrialConvertedNotifier. Fires
// the REQ-010 branded trial-converted email after the first successful
// charge (subscription status flips from "trialing" to "active"). The
// billing service already won the dedup race via the
// providers.trial_converted_email_sent_at column, so this notifier just
// renders + sends.
type TrialConvertedNotifier struct {
	Pool    *sql.DB
	Emailer *notify.Emailer
	AppBase string
	Log     *slog.Logger
}

func (n *TrialConvertedNotifier) OnTrialConverted(providerID, subscriptionID string, periodEnd time.Time) {
	if n == nil || n.Pool == nil {
		return
	}
	logger := n.Log
	if logger == nil {
		logger = slog.Default()
	}
	if n.Emailer == nil {
		logger.Info("trial-converted: emailer not configured; skipping send", "provider_id", providerID)
		return
	}

	var email, name, fullName, plan string
	if err := n.Pool.QueryRow(`
		SELECT COALESCE(p.owner_email,''),
		       COALESCE(p.name, p.legal_name, ''),
		       COALESCE((SELECT u.full_name FROM users u
		                  WHERE u.provider_id = p.id AND u.role = 'provider_admin'
		                    AND u.deleted_at IS NULL
		                  ORDER BY u.created_at ASC LIMIT 1), ''),
		       COALESCE(p.plan, '')
		  FROM providers p WHERE p.id = ?`, providerID).Scan(&email, &name, &fullName, &plan); err != nil {
		logger.Warn("trial-converted: lookup provider", "provider_id", providerID, "err", err)
		return
	}
	if email == "" {
		logger.Warn("trial-converted: provider has no owner_email", "provider_id", providerID)
		return
	}

	planLabel, amount := planLabelAndAmount(plan)
	nextCharge := ""
	if !periodEnd.IsZero() {
		nextCharge = periodEnd.Format("Jan 2, 2006")
	}

	subject, html, text, err := notify.RenderTrialConverted(notify.TrialConvertedData{
		FirstName:     pickFirstName(fullName, name),
		FacilityName:  name,
		PlanName:      planLabel,
		AmountCharged: amount,
		NextChargeOn:  nextCharge,
		AppURL:        strings.TrimRight(n.AppBase, "/"),
	})
	if err != nil {
		logger.Warn("trial-converted: render", "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.Emailer.Send(ctx, notify.EmailMessage{
		To: email, Subject: subject, HTMLBody: html, PlainBody: text,
		ReferenceID: "trial-converted:" + subscriptionID,
	}); err != nil {
		logger.Warn("trial-converted: send", "provider_id", providerID, "err", err)
		return
	}
	logger.Info("trial-converted: sent", "provider_id", providerID, "to", email, "subscription", subscriptionID)
}

// planLabelAndAmount maps the REQ-002 plan codes to display label + the
// pre-formatted dollar amount the trial-converted template expects.
func planLabelAndAmount(plan string) (label, amount string) {
	switch plan {
	case "annual_948":
		return "Levelbrook Professional — Annual", "948.00"
	case "monthly_99":
		return "Levelbrook Professional — Monthly", "99.00"
	default:
		// Default to monthly — the trial-converted hook only fires once a
		// real subscription is active so this is just a safe fallback.
		return "Levelbrook Professional — Monthly", "99.00"
	}
}

func pickFirstName(fullName, fallback string) string {
	if s := strings.TrimSpace(fullName); s != "" {
		return s
	}
	return fallback
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func priceCentsForPlan(plan string) int {
	switch plan {
	case "annual_948":
		return 94800
	case "monthly_99":
		return 9900
	case "enterprise":
		return 19900
	}
	return 9900
}
