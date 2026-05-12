package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v79"
	billingportal "github.com/stripe/stripe-go/v79/billingportal/session"
	"github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/customer"
	"github.com/stripe/stripe-go/v79/webhook"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// PostCheckoutNotifier is the contract the handlers package implements to
// send the welcome email after Stripe's `checkout.session.completed` lands.
// We declare it as an interface here to avoid a billing -> handlers ->
// notify import cycle (billing already imports nothing from handlers).
type PostCheckoutNotifier interface {
	OnCheckoutCompleted(providerID, sessionID string)
}

// PaymentFailedNotifier is the REQ-022 contract for sending the branded
// payment-failed email after Stripe's `invoice.payment_failed` webhook.
// Kept as a sibling interface (not an extension of PostCheckoutNotifier)
// because the two side-effects have different lifetimes — welcome runs
// once per signup, payment-failed can fire repeatedly for the same
// provider — and tests stub them independently.
//
// amountDueCents is the Stripe `amount_due` value (cents). declineReason
// is human-readable text — the handler is responsible for falling back
// to a safe default if Stripe gave us nothing usable.
type PaymentFailedNotifier interface {
	OnInvoicePaymentFailed(providerID, invoiceID, customerID string, amountDueCents int64, declineReason string)
}

// TrialConvertedNotifier is the REQ-010 contract for sending the branded
// trial-converted email when a provider's subscription flips from
// `trialing` to `active` for the first time (i.e. the first successful
// charge after the 14-day trial). Idempotent via providers.trial_converted_email_sent_at.
//
// The notifier is invoked from onSubscriptionChange when we observe a
// transition into status="active" AND we haven't sent before. Kept as a
// separate interface (sibling to PostCheckoutNotifier / PaymentFailedNotifier)
// for the same testability + lifetime reasons.
type TrialConvertedNotifier interface {
	OnTrialConverted(providerID, subscriptionID string, periodEnd time.Time)
}

type Service struct {
	pool                  *sql.DB
	secretKey             string
	webhookSecret         string
	frontendBaseURL       string
	priceMonthly          string
	priceAnnual           string
	notifier               PostCheckoutNotifier
	paymentFailedNotifier  PaymentFailedNotifier
	trialConvertedNotifier TrialConvertedNotifier
	log                   *slog.Logger
}

type Config struct {
	SecretKey       string
	WebhookSecret   string
	FrontendBaseURL string
	// REQ-002: separate monthly + annual price IDs. The legacy single-price
	// flow continues to work via Service.CreateCheckoutSession.
	PriceIDMonthly string
	PriceIDAnnual  string
	Pool           *sql.DB
	Logger         *slog.Logger
}

func New(cfg Config) *Service {
	stripe.Key = cfg.SecretKey
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		pool:            cfg.Pool,
		secretKey:       cfg.SecretKey,
		webhookSecret:   cfg.WebhookSecret,
		frontendBaseURL: cfg.FrontendBaseURL,
		priceMonthly:    cfg.PriceIDMonthly,
		priceAnnual:     cfg.PriceIDAnnual,
		log:             log,
	}
}

// SetNotifier wires a post-checkout side-effect handler. Optional — webhook
// handling proceeds with or without it.
func (s *Service) SetNotifier(n PostCheckoutNotifier) { s.notifier = n }

// SetPaymentFailedNotifier wires the REQ-022 payment-failed email handler.
// Optional — webhook handling proceeds with or without it.
func (s *Service) SetPaymentFailedNotifier(n PaymentFailedNotifier) { s.paymentFailedNotifier = n }

// SetTrialConvertedNotifier wires the REQ-010 trial-converted email
// handler. Fires when a subscription's status transitions to "active"
// for the first time (the first successful post-trial charge).
func (s *Service) SetTrialConvertedNotifier(n TrialConvertedNotifier) { s.trialConvertedNotifier = n }

// PriceIDFor returns the configured Stripe price ID for the REQ-002 plan
// codes. Returns empty string for unknown codes; the caller falls back.
func (s *Service) PriceIDFor(plan string) string {
	switch plan {
	case "annual_948", "annual":
		return s.priceAnnual
	case "monthly_99", "monthly":
		return s.priceMonthly
	}
	return ""
}

// EnsureCustomer returns an existing Stripe customer ID for the provider,
// or creates one and persists it.
func (s *Service) EnsureCustomer(ctx context.Context, providerID, email, name string) (string, error) {
	var cid *string
	if err := s.pool.QueryRowContext(ctx, `SELECT stripe_customer_id FROM providers WHERE id = ?`, providerID).Scan(&cid); err != nil {
		return "", fmt.Errorf("billing: lookup provider: %w", err)
	}
	if cid != nil && *cid != "" {
		return *cid, nil
	}
	params := &stripe.CustomerParams{
		Email: stripe.String(email),
		Name:  stripe.String(name),
	}
	params.AddMetadata("provider_id", providerID)
	c, err := customer.New(params)
	if err != nil {
		return "", fmt.Errorf("billing: create customer: %w", err)
	}
	if _, err := s.pool.ExecContext(ctx, `UPDATE providers SET stripe_customer_id = ? WHERE id = ?`, c.ID, providerID); err != nil {
		return "", fmt.Errorf("billing: persist customer id: %w", err)
	}
	return c.ID, nil
}

// CreateCheckoutSession returns a hosted Stripe Checkout URL.
func (s *Service) CreateCheckoutSession(ctx context.Context, providerID, priceID, email, name, promoCode string) (string, error) {
	return s.CreateCheckoutSessionV2(ctx, CheckoutInput{
		ProviderID: providerID,
		PriceID:    priceID,
		Email:      email,
		Name:       name,
		PromoCode:  promoCode,
	})
}

// CheckoutInput is the REQ-002 shape: lets callers configure success/cancel
// URLs explicitly so /onboarding/legal lands the operator on the right page,
// and turn on a 14-day trial with card-on-file required.
type CheckoutInput struct {
	ProviderID  string
	PriceID     string
	Email       string
	Name        string
	PromoCode   string
	SuccessURL  string // optional override
	CancelURL   string // optional override
	TrialDays   int64  // 0 = none; 14 = REQ-002 default
}

// CreateCheckoutSessionV2 is the REQ-002 entry point — it sets up the trial,
// records provider_id metadata so the webhook can resolve the tenant, and
// requires payment_method on file even during the trial (so we can charge
// on day 15 without a second user action).
func (s *Service) CreateCheckoutSessionV2(ctx context.Context, in CheckoutInput) (string, error) {
	custID, err := s.EnsureCustomer(ctx, in.ProviderID, in.Email, in.Name)
	if err != nil {
		return "", err
	}
	successURL := in.SuccessURL
	if successURL == "" {
		// REQ-002: after Stripe Checkout returns, land on /onboarding/legal.
		successURL = strings.TrimRight(s.frontendBaseURL, "/") + "/onboarding/legal?session_id={CHECKOUT_SESSION_ID}"
	}
	cancelURL := in.CancelURL
	if cancelURL == "" {
		cancelURL = strings.TrimRight(s.frontendBaseURL, "/") + "/onboarding/checkout?canceled=1"
	}
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		Customer:   stripe.String(custID),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(in.PriceID), Quantity: stripe.Int64(1)},
		},
	}
	if in.TrialDays > 0 {
		params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{
			TrialPeriodDays: stripe.Int64(in.TrialDays),
		}
		// Card-on-file required even during the trial (REQ-002 #3).
		params.PaymentMethodCollection = stripe.String(string(stripe.CheckoutSessionPaymentMethodCollectionAlways))
	}
	params.AddMetadata("provider_id", in.ProviderID)
	if in.PromoCode != "" {
		params.Discounts = []*stripe.CheckoutSessionDiscountParams{{PromotionCode: stripe.String(in.PromoCode)}}
	} else {
		params.AllowPromotionCodes = stripe.Bool(true)
	}
	sess, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("billing: create checkout: %w", err)
	}
	return sess.URL, nil
}

// CustomerPortalURL creates a one-time Stripe Billing Portal URL.
func (s *Service) CustomerPortalURL(ctx context.Context, providerID string) (string, error) {
	var custID *string
	if err := s.pool.QueryRowContext(ctx, `SELECT stripe_customer_id FROM providers WHERE id = ?`, providerID).Scan(&custID); err != nil {
		return "", fmt.Errorf("billing: lookup provider: %w", err)
	}
	if custID == nil || *custID == "" {
		return "", errors.New("billing: provider has no stripe customer")
	}
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(*custID),
		ReturnURL: stripe.String(s.frontendBaseURL + "/settings/billing"),
	}
	p, err := billingportal.New(params)
	if err != nil {
		return "", fmt.Errorf("billing: portal: %w", err)
	}
	return p.URL, nil
}

// HandleWebhook validates the Stripe signature and processes lifecycle events.
// Must be mounted on a raw-body route (no JSON decoding middleware).
//
// REQ-002: idempotent on Stripe event id. We INSERT OR IGNORE into
// stripe_events first; if the row already had a non-NULL processed_at we
// skip processing. This is the dedup contract — Stripe will retry
// indefinitely on non-2xx, and a 200 with skipped processing is correct.
func (s *Service) HandleWebhook(r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("billing: read body: %w", err)
	}
	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), s.webhookSecret)
	if err != nil {
		return fmt.Errorf("billing: verify signature: %w", err)
	}

	ctx := r.Context()

	// Idempotency: persist the raw event keyed on stripe_event_id (UNIQUE).
	// If we've already processed it, return success without re-running side-effects.
	if s.pool != nil {
		row := s.pool.QueryRowContext(ctx,
			`SELECT processed_at FROM stripe_events WHERE stripe_event_id = ?`, event.ID)
		var processed sql.NullString
		switch err := row.Scan(&processed); {
		case errors.Is(err, sql.ErrNoRows):
			// First time seeing this event — record it.
			if _, err := s.pool.ExecContext(ctx, `
				INSERT INTO stripe_events (id, stripe_event_id, type, payload, received_at)
				VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
				base62.NewID()[:22], event.ID, string(event.Type), string(body)); err != nil {
				// Race: another worker inserted between our SELECT and INSERT.
				// Drop the duplicate; the other goroutine will process.
				s.log.Debug("billing: stripe_events duplicate insert", "event_id", event.ID, "err", err)
				return nil
			}
		case err != nil:
			s.log.Warn("billing: stripe_events lookup", "event_id", event.ID, "err", err)
			// Fall through and process; missing audit row is recoverable.
		default:
			if processed.Valid && processed.String != "" {
				s.log.Debug("billing: webhook already processed", "event_id", event.ID)
				return nil
			}
		}
	}

	var processErr error
	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			return fmt.Errorf("billing: decode checkout: %w", err)
		}
		processErr = s.onCheckoutCompleted(ctx, &sess)

	case "customer.subscription.created",
		"customer.subscription.updated",
		"customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return fmt.Errorf("billing: decode sub: %w", err)
		}
		processErr = s.onSubscriptionChange(ctx, &sub)

	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			return fmt.Errorf("billing: decode invoice: %w", err)
		}
		if inv.Customer != nil {
			s.log.Warn("billing: payment failed", "customer", inv.Customer.ID, "invoice", inv.ID)
		}
		// Mark the provider's subscription_status to past_due; the dashboard
		// banner reads this column.
		var providerID string
		if inv.Customer != nil {
			_, _ = s.pool.ExecContext(ctx,
				`UPDATE providers SET subscription_status = 'past_due', updated_at = CURRENT_TIMESTAMP
				 WHERE stripe_customer_id = ?`, inv.Customer.ID)
			_ = s.pool.QueryRowContext(ctx, `SELECT id FROM providers WHERE stripe_customer_id = ?`, inv.Customer.ID).Scan(&providerID)
		}
		// REQ-022: fire the branded payment-failed email. Async so we ack
		// Stripe fast — Stripe expects 2xx within ~30s.
		if s.paymentFailedNotifier != nil && providerID != "" {
			declineReason := ""
			if inv.LastFinalizationError != nil {
				declineReason = inv.LastFinalizationError.Msg
			}
			customerID := ""
			if inv.Customer != nil {
				customerID = inv.Customer.ID
			}
			go s.paymentFailedNotifier.OnInvoicePaymentFailed(providerID, inv.ID, customerID, inv.AmountDue, declineReason)
		}

	default:
		s.log.Debug("billing: unhandled event", "type", event.Type)
	}

	// Mark idempotency row processed even if processErr != nil — Stripe should
	// retry only the events we explicitly 5xx on. Persist the error too.
	if s.pool != nil {
		errStr := ""
		if processErr != nil {
			errStr = processErr.Error()
		}
		_, _ = s.pool.ExecContext(ctx,
			`UPDATE stripe_events
			    SET processed_at = CURRENT_TIMESTAMP,
			        processing_error = NULLIF(?, '')
			  WHERE stripe_event_id = ?`, errStr, event.ID)
	}
	return processErr
}

func (s *Service) onCheckoutCompleted(ctx context.Context, sess *stripe.CheckoutSession) error {
	providerID := sess.Metadata["provider_id"]
	if providerID == "" && sess.Customer != nil {
		// fall back: look up provider by customer id
		_ = s.pool.QueryRowContext(ctx, `SELECT id FROM providers WHERE stripe_customer_id = ?`, sess.Customer.ID).Scan(&providerID)
	}
	if providerID == "" {
		return errors.New("billing: checkout missing provider_id")
	}
	s.log.Info("billing: checkout completed", "provider", providerID, "session", sess.ID)

	// Notify the handlers package so it can send the welcome email + any
	// other side-effects (REQ-002 wiring). Run in a goroutine so we ack the
	// webhook fast — Stripe expects 2xx within ~30s.
	if s.notifier != nil {
		go s.notifier.OnCheckoutCompleted(providerID, sess.ID)
	}
	return nil
}

func (s *Service) onSubscriptionChange(ctx context.Context, sub *stripe.Subscription) error {
	if sub.Customer == nil {
		return errors.New("billing: subscription missing customer")
	}
	var providerID string
	if err := s.pool.QueryRowContext(ctx, `SELECT id FROM providers WHERE stripe_customer_id = ?`, sub.Customer.ID).Scan(&providerID); err != nil {
		return fmt.Errorf("billing: lookup provider from customer: %w", err)
	}
	priceID := ""
	if len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
		priceID = sub.Items.Data[0].Price.ID
	}
	// IDs generated in Go (base62) per ADR-004; period_end converted to a
	// time.Time so the SQLite driver stores an ISO 8601 string matching the
	// rest of the schema.
	subID := base62.NewID()[:22]
	periodEnd := time.Unix(sub.CurrentPeriodEnd, 0).UTC()

	// We also stash trial_end on the provider row for the trial-countdown
	// banner in the dashboard. Stripe may set sub.TrialEnd to 0 once the
	// trial ends; that's fine — the banner hides itself when trial_ends_at
	// is in the past.
	var trialEnd *time.Time
	if sub.TrialEnd > 0 {
		t := time.Unix(sub.TrialEnd, 0).UTC()
		trialEnd = &t
	}

	tx, err := s.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("billing: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO subscriptions (id, provider_id, stripe_subscription_id, stripe_price_id, status, current_period_end, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (stripe_subscription_id) DO UPDATE
		SET status = EXCLUDED.status,
		    stripe_price_id = EXCLUDED.stripe_price_id,
		    current_period_end = EXCLUDED.current_period_end,
		    updated_at = CURRENT_TIMESTAMP`,
		subID, providerID, sub.ID, priceID, string(sub.Status), periodEnd); err != nil {
		return fmt.Errorf("billing: upsert subscription: %w", err)
	}

	// Mirror onto providers (REQ-002 account-shape columns).
	plan := planFromPriceID(priceID, s.priceMonthly, s.priceAnnual)
	var trialArg any
	if trialEnd != nil {
		trialArg = trialEnd.Format(time.RFC3339)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE providers
		   SET subscription_status   = ?,
		       stripe_subscription_id = ?,
		       plan                   = COALESCE(?, plan),
		       trial_ends_at          = COALESCE(?, trial_ends_at),
		       current_period_end_at  = ?,
		       updated_at             = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		string(sub.Status), sub.ID, nullIfEmpty(plan), trialArg,
		periodEnd.Format(time.RFC3339), providerID); err != nil {
		return fmt.Errorf("billing: update provider account fields: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// REQ-010: when the subscription transitions into "active" AND we
	// haven't already sent the trial-converted email, fire it. The marker
	// column (providers.trial_converted_email_sent_at) is checked + set
	// atomically below so concurrent webhook deliveries can't double-send.
	if sub.Status == stripe.SubscriptionStatusActive && s.trialConvertedNotifier != nil {
		// SQLite UPDATE..WHERE..IS NULL is atomic — RowsAffected==1 means
		// we won the race and own the send.
		res, err := s.pool.ExecContext(ctx,
			`UPDATE providers SET trial_converted_email_sent_at = ?
			  WHERE id = ? AND trial_converted_email_sent_at IS NULL`,
			time.Now().UTC().Format(time.RFC3339), providerID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 1 {
				go s.trialConvertedNotifier.OnTrialConverted(providerID, sub.ID, periodEnd)
			}
		} else {
			s.log.Warn("billing: mark trial_converted_email_sent_at", "provider", providerID, "err", err)
		}
	}

	return nil
}

// planFromPriceID maps a Stripe price ID back to the REQ-002 plan code.
// Returns "" when the price doesn't match either configured price.
func planFromPriceID(priceID, monthly, annual string) string {
	switch priceID {
	case "":
		return ""
	case monthly:
		return "monthly_99"
	case annual:
		return "annual_948"
	}
	return ""
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// HasActiveSubscription implements middleware.BillingChecker.
func (s *Service) HasActiveSubscription(ctx context.Context, providerID string) (bool, error) {
	var status string
	var periodEnd time.Time
	err := s.pool.QueryRowContext(ctx, `
		SELECT status, current_period_end
		FROM subscriptions
		WHERE provider_id = ?
		ORDER BY updated_at DESC
		LIMIT 1`, providerID).Scan(&status, &periodEnd)
	if err != nil {
		// no subscription found -> not active (but not an error).
		return false, nil
	}
	if status == "active" || status == "trialing" {
		return true, nil
	}
	if status == "past_due" && time.Now().Before(periodEnd.Add(7*24*time.Hour)) {
		return true, nil // grace period
	}
	return false, nil
}
