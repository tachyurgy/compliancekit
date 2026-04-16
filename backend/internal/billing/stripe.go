package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stripe/stripe-go/v79"
	billingportal "github.com/stripe/stripe-go/v79/billingportal/session"
	"github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/customer"
	"github.com/stripe/stripe-go/v79/webhook"
)

type Service struct {
	pool            *pgxpool.Pool
	secretKey       string
	webhookSecret   string
	frontendBaseURL string
	log             *slog.Logger
}

type Config struct {
	SecretKey       string
	WebhookSecret   string
	FrontendBaseURL string
	Pool            *pgxpool.Pool
	Logger          *slog.Logger
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
		log:             log,
	}
}

// EnsureCustomer returns an existing Stripe customer ID for the provider,
// or creates one and persists it.
func (s *Service) EnsureCustomer(ctx context.Context, providerID, email, name string) (string, error) {
	var cid *string
	if err := s.pool.QueryRow(ctx, `SELECT stripe_customer_id FROM providers WHERE id = $1`, providerID).Scan(&cid); err != nil {
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
	if _, err := s.pool.Exec(ctx, `UPDATE providers SET stripe_customer_id = $2 WHERE id = $1`, providerID, c.ID); err != nil {
		return "", fmt.Errorf("billing: persist customer id: %w", err)
	}
	return c.ID, nil
}

// CreateCheckoutSession returns a hosted Stripe Checkout URL.
func (s *Service) CreateCheckoutSession(ctx context.Context, providerID, priceID, email, name, promoCode string) (string, error) {
	custID, err := s.EnsureCustomer(ctx, providerID, email, name)
	if err != nil {
		return "", err
	}
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		Customer:   stripe.String(custID),
		SuccessURL: stripe.String(s.frontendBaseURL + "/billing/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(s.frontendBaseURL + "/billing/cancel"),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(priceID), Quantity: stripe.Int64(1)},
		},
		AllowPromotionCodes: stripe.Bool(true),
	}
	params.AddMetadata("provider_id", providerID)
	if promoCode != "" {
		params.Discounts = []*stripe.CheckoutSessionDiscountParams{{PromotionCode: stripe.String(promoCode)}}
		params.AllowPromotionCodes = nil
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
	if err := s.pool.QueryRow(ctx, `SELECT stripe_customer_id FROM providers WHERE id = $1`, providerID).Scan(&custID); err != nil {
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
	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			return fmt.Errorf("billing: decode checkout: %w", err)
		}
		return s.onCheckoutCompleted(ctx, &sess)

	case "customer.subscription.created",
		"customer.subscription.updated",
		"customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return fmt.Errorf("billing: decode sub: %w", err)
		}
		return s.onSubscriptionChange(ctx, &sub)

	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			return fmt.Errorf("billing: decode invoice: %w", err)
		}
		s.log.Warn("billing: payment failed", "customer", inv.Customer.ID, "invoice", inv.ID)
		return nil

	default:
		s.log.Debug("billing: unhandled event", "type", event.Type)
		return nil
	}
}

func (s *Service) onCheckoutCompleted(ctx context.Context, sess *stripe.CheckoutSession) error {
	providerID := sess.Metadata["provider_id"]
	if providerID == "" && sess.Customer != nil {
		// fall back: look up provider by customer id
		_ = s.pool.QueryRow(ctx, `SELECT id FROM providers WHERE stripe_customer_id = $1`, sess.Customer.ID).Scan(&providerID)
	}
	if providerID == "" {
		return errors.New("billing: checkout missing provider_id")
	}
	s.log.Info("billing: checkout completed", "provider", providerID, "session", sess.ID)
	return nil
}

func (s *Service) onSubscriptionChange(ctx context.Context, sub *stripe.Subscription) error {
	if sub.Customer == nil {
		return errors.New("billing: subscription missing customer")
	}
	var providerID string
	if err := s.pool.QueryRow(ctx, `SELECT id FROM providers WHERE stripe_customer_id = $1`, sub.Customer.ID).Scan(&providerID); err != nil {
		return fmt.Errorf("billing: lookup provider from customer: %w", err)
	}
	priceID := ""
	if len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
		priceID = sub.Items.Data[0].Price.ID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO subscriptions (id, provider_id, stripe_subscription_id, stripe_price_id, status, current_period_end, created_at, updated_at)
		VALUES (gen_random_uuid()::text, $1, $2, $3, $4, to_timestamp($5), NOW(), NOW())
		ON CONFLICT (stripe_subscription_id) DO UPDATE
		SET status = EXCLUDED.status,
		    stripe_price_id = EXCLUDED.stripe_price_id,
		    current_period_end = EXCLUDED.current_period_end,
		    updated_at = NOW()`,
		providerID, sub.ID, priceID, string(sub.Status), sub.CurrentPeriodEnd)
	if err != nil {
		return fmt.Errorf("billing: upsert subscription: %w", err)
	}
	return nil
}

// HasActiveSubscription implements middleware.BillingChecker.
func (s *Service) HasActiveSubscription(ctx context.Context, providerID string) (bool, error) {
	var status string
	var periodEnd time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT status, current_period_end
		FROM subscriptions
		WHERE provider_id = $1
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
