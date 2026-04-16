-- 000001_init_providers_and_users.up.sql
-- Bootstraps the ComplianceKit schema: extensions, shared helper trigger,
-- providers, users, and magic_link_tokens.

BEGIN;

-- ---------------------------------------------------------------------------
-- Extensions
-- ---------------------------------------------------------------------------
-- CITEXT gives us case-insensitive email comparison without LOWER() everywhere.
CREATE EXTENSION IF NOT EXISTS citext;
-- pgcrypto is not strictly required (we generate IDs in Go with base62) but is
-- useful for digest()/gen_random_bytes() in future migrations.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Shared trigger function to keep updated_at current.
-- Every subsequent migration attaches this trigger to new tables.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- providers: one row per licensed facility / account tenant.
-- Soft-deletable. state and state_abbr are intentionally separate fields:
-- "state" is the licensing state (regulatory jurisdiction), state_abbr is
-- the physical address state. They are usually the same but not always
-- (e.g. corporate HQ in a different state).
-- ---------------------------------------------------------------------------
CREATE TABLE providers (
    id                   TEXT PRIMARY KEY,
    legal_name           TEXT NOT NULL,
    dba                  TEXT,
    state                CHAR(2) NOT NULL,
    license_type         TEXT,
    license_number       TEXT,
    license_expires_on   DATE,
    address_line1        TEXT,
    address_line2        TEXT,
    city                 TEXT,
    state_abbr           CHAR(2),
    postal_code          TEXT,
    phone                TEXT,
    timezone             TEXT NOT NULL DEFAULT 'America/New_York',
    onboarding_complete  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ NULL,
    CONSTRAINT providers_state_chk      CHECK (state ~ '^[A-Z]{2}$'),
    CONSTRAINT providers_state_abbr_chk CHECK (state_abbr IS NULL OR state_abbr ~ '^[A-Z]{2}$')
);

CREATE TRIGGER providers_set_updated_at
BEFORE UPDATE ON providers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX providers_state_idx        ON providers (state) WHERE deleted_at IS NULL;
CREATE INDEX providers_license_expires_idx ON providers (license_expires_on) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- users: humans who can log in. Tied to a provider (tenant).
-- role uses a CHECK constraint rather than an ENUM type for easier evolution
-- (adding a value to a Postgres ENUM requires ALTER TYPE and locks).
-- email is CITEXT so 'Foo@Bar.com' and 'foo@bar.com' collide at the unique idx.
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id                 TEXT PRIMARY KEY,
    provider_id        TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    email              CITEXT NOT NULL UNIQUE,
    phone              TEXT,
    full_name          TEXT NOT NULL,
    role               TEXT NOT NULL,
    last_login_at      TIMESTAMPTZ NULL,
    email_verified_at  TIMESTAMPTZ NULL,
    phone_verified_at  TIMESTAMPTZ NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ NULL,
    CONSTRAINT users_role_chk CHECK (role IN ('provider_admin','provider_staff'))
);

CREATE TRIGGER users_set_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX users_email_idx       ON users (email);
CREATE INDEX users_provider_id_idx ON users (provider_id) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- magic_link_tokens: every passwordless flow (signup, signin, parent upload,
-- staff upload, document sign). Only the SHA-256 hash is stored — the raw
-- token is delivered out-of-band (email/SMS) and never persisted.
--
-- subject_id is polymorphic and its meaning depends on kind:
--   provider_signin  -> users.id
--   provider_signup  -> pending providers.id (pre-activation)
--   parent_upload    -> children.id
--   staff_upload     -> staff.id
--   document_sign    -> sign_sessions.id (see 000005)
-- No FK because of the polymorphism.
-- ---------------------------------------------------------------------------
CREATE TABLE magic_link_tokens (
    id             TEXT PRIMARY KEY,
    kind           TEXT NOT NULL,
    subject_id     TEXT,
    token_hash     BYTEA NOT NULL UNIQUE,
    expires_at     TIMESTAMPTZ NOT NULL,
    consumed_at    TIMESTAMPTZ NULL,
    ip             INET,
    user_agent     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT magic_link_kind_chk CHECK (
        kind IN ('provider_signup','provider_signin','parent_upload','staff_upload','document_sign')
    )
);

CREATE INDEX magic_link_tokens_token_hash_idx ON magic_link_tokens (token_hash);
CREATE INDEX magic_link_tokens_expires_idx    ON magic_link_tokens (expires_at) WHERE consumed_at IS NULL;
CREATE INDEX magic_link_tokens_subject_idx    ON magic_link_tokens (kind, subject_id);

COMMIT;
