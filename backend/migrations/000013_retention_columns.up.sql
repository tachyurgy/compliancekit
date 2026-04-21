-- 000013_retention_columns.up.sql
-- Retention metadata for the 90-day data purge worker (internal/retention).
--
-- canceled_at: ISO-8601 UTC timestamp. Set by the Stripe webhook handler when a
-- subscription transitions to `canceled`, or explicitly set by an admin purge
-- request (Settings → "Cancel & delete"). The retention worker deletes every
-- tenant row (children, staff, documents, etc.) once canceled_at + 90 days is
-- in the past AND the provider has not re-subscribed.
--
-- deleted_at already exists on providers (000001) for soft deletes. The purge
-- worker also fires on deleted_at + 90 days, for the case where the owner
-- requests outright account deletion without waiting for Stripe to churn.
--
-- Indexes target the two nightly-scan predicates used by the purge worker.
-- Both are partial to stay small in the steady state (99%+ of rows NULL).
--
-- SQLite dialect (see ADR-017).

ALTER TABLE providers ADD COLUMN canceled_at TEXT;

CREATE INDEX IF NOT EXISTS providers_canceled_at_idx
    ON providers (canceled_at)
    WHERE canceled_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS providers_deleted_at_purge_idx
    ON providers (deleted_at)
    WHERE deleted_at IS NOT NULL;
