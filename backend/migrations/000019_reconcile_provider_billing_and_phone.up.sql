-- 000019_reconcile_provider_billing_and_phone
--
-- Final wave of handler-vs-schema reconciles:
--   * providers.owner_phone — PATCH /api/me writes it; never declared.
--   * providers.stripe_customer_id — billing.go reads from providers first
--     (denormalized cache of the subscription's column). Add the column so
--     the SELECT/UPDATE in billing.Service succeed.
--   * subscriptions.stripe_price_id — handler writes this; 000007 declared
--     `plan` instead. Add the column; both can coexist.
--   * subscriptions.current_period_end column was declared as TEXT; add
--     trial_end + cancel_at_period_end that billing code may reference.

ALTER TABLE providers ADD COLUMN owner_phone TEXT;
ALTER TABLE providers ADD COLUMN stripe_customer_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_providers_stripe_customer
    ON providers(stripe_customer_id)
    WHERE stripe_customer_id IS NOT NULL;

ALTER TABLE subscriptions ADD COLUMN stripe_price_id TEXT;
