ALTER TABLE subscriptions DROP COLUMN stripe_price_id;
DROP INDEX IF EXISTS idx_providers_stripe_customer;
ALTER TABLE providers DROP COLUMN stripe_customer_id;
ALTER TABLE providers DROP COLUMN owner_phone;
