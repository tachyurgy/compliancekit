-- 000007_billing_and_legal.down.sql

BEGIN;

ALTER TABLE signatures DROP CONSTRAINT IF EXISTS signatures_consent_version_fk;

DROP TABLE IF EXISTS policy_acceptances;
DROP TABLE IF EXISTS policy_versions;
DROP TABLE IF EXISTS stripe_events;
DROP TABLE IF EXISTS subscriptions;

COMMIT;
