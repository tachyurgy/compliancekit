-- 000007_billing_and_legal.up.sql
-- Stripe subscription state, raw webhook log, and legal policy versioning.

BEGIN;

-- ---------------------------------------------------------------------------
-- subscriptions: one row per provider. UNIQUE(provider_id) because we only
-- run one active subscription per tenant. We mirror Stripe state here so the
-- API doesn't have to fan out to Stripe on every request — webhook-driven.
-- ---------------------------------------------------------------------------
CREATE TABLE subscriptions (
    id                    TEXT PRIMARY KEY,
    provider_id           TEXT NOT NULL UNIQUE REFERENCES providers(id) ON DELETE CASCADE,
    stripe_customer_id    TEXT NOT NULL,
    stripe_subscription_id TEXT,
    plan                  TEXT NOT NULL,
    status                TEXT NOT NULL,
    current_period_end    TIMESTAMPTZ,
    cancel_at_period_end  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT subscriptions_plan_chk   CHECK (plan IN ('starter','pro','enterprise')),
    CONSTRAINT subscriptions_status_chk CHECK (status IN ('trialing','active','past_due','canceled','incomplete'))
);

CREATE TRIGGER subscriptions_set_updated_at
BEFORE UPDATE ON subscriptions
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX subscriptions_stripe_customer_idx      ON subscriptions (stripe_customer_id);
CREATE INDEX subscriptions_stripe_subscription_idx  ON subscriptions (stripe_subscription_id);
CREATE INDEX subscriptions_status_idx               ON subscriptions (status);

-- ---------------------------------------------------------------------------
-- stripe_events: every webhook we receive, stored raw. Idempotency key is
-- stripe_event_id (UNIQUE). processed_at being NULL means "enqueued but not
-- yet handled"; processing_error captures retryable failures.
-- ---------------------------------------------------------------------------
CREATE TABLE stripe_events (
    id                TEXT PRIMARY KEY,
    stripe_event_id   TEXT NOT NULL UNIQUE,
    type              TEXT NOT NULL,
    payload           JSONB NOT NULL,
    received_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at      TIMESTAMPTZ NULL,
    processing_error  TEXT
);

CREATE INDEX stripe_events_unprocessed_idx ON stripe_events (received_at)
    WHERE processed_at IS NULL;
CREATE INDEX stripe_events_type_idx        ON stripe_events (type);

-- ---------------------------------------------------------------------------
-- policy_versions: every version of every legal document we've ever shown
-- anyone. Immutable, never deleted. content_url points to the PDF in
-- ck-audit-trail; sha256 is its fingerprint so any tampering is detectable.
-- ---------------------------------------------------------------------------
CREATE TABLE policy_versions (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,
    version      TEXT NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL,
    content_url  TEXT NOT NULL,
    sha256       BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT policy_versions_kind_chk CHECK (
        kind IN ('privacy','tos','msa','dpa','esign_disclosure','parent_consent','staff_consent')
    ),
    CONSTRAINT policy_versions_unique UNIQUE (kind, version)
);

CREATE INDEX policy_versions_current_idx ON policy_versions (kind, effective_at DESC);

-- Now that policy_versions exists, attach the FK we pre-declared in 000005.
ALTER TABLE signatures
    ADD CONSTRAINT signatures_consent_version_fk
    FOREIGN KEY (consent_version_id) REFERENCES policy_versions(id) ON DELETE RESTRICT;

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
    accepted_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip                  INET,
    user_agent          TEXT,
    CONSTRAINT policy_acceptances_actor_chk CHECK (
        user_id IS NOT NULL OR magic_link_token_id IS NOT NULL
    )
);

CREATE INDEX policy_acceptances_user_idx    ON policy_acceptances (user_id);
CREATE INDEX policy_acceptances_version_idx ON policy_acceptances (policy_version_id);

COMMIT;
