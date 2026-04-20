-- 000007_billing_and_legal.up.sql
-- Stripe subscription state, raw webhook log, and legal policy versioning.
-- SQLite dialect (see ADR-017).

-- ---------------------------------------------------------------------------
-- subscriptions: one row per provider. UNIQUE(provider_id) because we only
-- run one active subscription per tenant. We mirror Stripe state here so the
-- API doesn't have to fan out to Stripe on every request — webhook-driven.
-- ---------------------------------------------------------------------------
CREATE TABLE subscriptions (
    id                     TEXT PRIMARY KEY,
    provider_id            TEXT NOT NULL UNIQUE REFERENCES providers(id) ON DELETE CASCADE,
    stripe_customer_id     TEXT NOT NULL,
    stripe_subscription_id TEXT,
    plan                   TEXT NOT NULL,
    status                 TEXT NOT NULL,
    current_period_end     TEXT,
    cancel_at_period_end   INTEGER NOT NULL DEFAULT 0,
    created_at             TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT subscriptions_plan_chk   CHECK (plan IN ('starter','pro','enterprise')),
    CONSTRAINT subscriptions_status_chk CHECK (status IN ('trialing','active','past_due','canceled','incomplete'))
);

CREATE TRIGGER subscriptions_set_updated_at
AFTER UPDATE ON subscriptions
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE subscriptions SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE INDEX subscriptions_stripe_customer_idx      ON subscriptions (stripe_customer_id);
CREATE INDEX subscriptions_stripe_subscription_idx  ON subscriptions (stripe_subscription_id);
CREATE INDEX subscriptions_status_idx               ON subscriptions (status);

-- ---------------------------------------------------------------------------
-- stripe_events: every webhook we receive, stored raw. Idempotency key is
-- stripe_event_id (UNIQUE). processed_at being NULL means "enqueued but not
-- yet handled"; processing_error captures retryable failures.
-- payload is TEXT JSON, validated.
-- ---------------------------------------------------------------------------
CREATE TABLE stripe_events (
    id                TEXT PRIMARY KEY,
    stripe_event_id   TEXT NOT NULL UNIQUE,
    type              TEXT NOT NULL,
    payload           TEXT NOT NULL,
    received_at       TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at      TEXT NULL,
    processing_error  TEXT,
    CONSTRAINT stripe_events_payload_chk CHECK (json_valid(payload))
);

CREATE INDEX stripe_events_unprocessed_idx ON stripe_events (received_at)
    WHERE processed_at IS NULL;
CREATE INDEX stripe_events_type_idx        ON stripe_events (type);

-- ---------------------------------------------------------------------------
-- policy_versions: every version of every legal document we've ever shown
-- anyone. Immutable, never deleted. content_url points to the PDF in the
-- ck-files bucket (audit/ prefix); sha256 is its fingerprint so any tampering
-- is detectable.
-- ---------------------------------------------------------------------------
CREATE TABLE policy_versions (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,
    version      TEXT NOT NULL,
    effective_at TEXT NOT NULL,
    content_url  TEXT NOT NULL,
    sha256       BLOB NOT NULL,
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT policy_versions_kind_chk CHECK (
        kind IN ('privacy','tos','msa','dpa','esign_disclosure','parent_consent','staff_consent')
    ),
    CONSTRAINT policy_versions_unique UNIQUE (kind, version)
);

CREATE INDEX policy_versions_current_idx ON policy_versions (kind, effective_at DESC);

-- Note: the PG version added a deferred FK from signatures.consent_version_id
-- to policy_versions(id) here via ALTER TABLE ADD CONSTRAINT. SQLite does not
-- support that operation (ADR-017). The app layer (pdfsign package) enforces
-- the referential integrity on insert.

-- ---------------------------------------------------------------------------
-- policy_acceptances: who agreed to what, when, from where. Each row is an
-- unalterable audit datum. Either user_id or magic_link_token_id is set —
-- the latter for parents/staff who agree via a magic-link flow without a
-- user account.
-- ---------------------------------------------------------------------------
CREATE TABLE policy_acceptances (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT REFERENCES users(id) ON DELETE SET NULL,
    magic_link_token_id TEXT REFERENCES magic_link_tokens(id) ON DELETE SET NULL,
    policy_version_id   TEXT NOT NULL REFERENCES policy_versions(id) ON DELETE RESTRICT,
    accepted_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ip                  TEXT,
    user_agent          TEXT,
    CONSTRAINT policy_acceptances_actor_chk CHECK (
        user_id IS NOT NULL OR magic_link_token_id IS NOT NULL
    )
);

CREATE INDEX policy_acceptances_user_idx    ON policy_acceptances (user_id);
CREATE INDEX policy_acceptances_version_idx ON policy_acceptances (policy_version_id);
