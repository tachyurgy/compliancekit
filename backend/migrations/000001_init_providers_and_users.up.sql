-- 000001_init_providers_and_users.up.sql
-- Bootstraps the ComplianceKit schema: providers, users, magic_link_tokens.
-- SQLite dialect (see ADR-017). Pragmas (foreign_keys, WAL) are applied by
-- the Go driver at Open(), not here.
--
-- Type conventions for this codebase under SQLite:
--   TEXT PRIMARY KEY                  base62 ID generated in Go
--   TEXT COLLATE NOCASE               case-insensitive (replaces PG CITEXT)
--   TEXT DEFAULT CURRENT_TIMESTAMP    ISO-8601 UTC timestamps
--   BLOB                              binary bytes (replaces PG BYTEA)
--   TEXT + json_valid()               JSON (replaces PG JSONB)
--   TEXT                              IP addresses (replaces PG INET)

-- ---------------------------------------------------------------------------
-- providers: one row per licensed facility / account tenant.
-- Soft-deletable. State format ([A-Z]{2}) is enforced in Go at insert time —
-- SQLite lacks regex CHECK without a loadable extension.
-- ---------------------------------------------------------------------------
CREATE TABLE providers (
    id                   TEXT PRIMARY KEY,
    legal_name           TEXT NOT NULL,
    dba                  TEXT,
    state                TEXT NOT NULL,
    license_type         TEXT,
    license_number       TEXT,
    license_expires_on   TEXT,
    address_line1        TEXT,
    address_line2        TEXT,
    city                 TEXT,
    state_abbr           TEXT,
    postal_code          TEXT,
    phone                TEXT,
    timezone             TEXT NOT NULL DEFAULT 'America/New_York',
    onboarding_complete  INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at           TEXT NULL,
    CONSTRAINT providers_state_len      CHECK (length(state) = 2),
    CONSTRAINT providers_state_abbr_len CHECK (state_abbr IS NULL OR length(state_abbr) = 2)
);

-- Canonical updated_at trigger for SQLite: only fires if the caller didn't
-- already bump updated_at (NEW IS OLD). The trigger's own UPDATE sets a new
-- value, so the WHEN clause is false on re-entry → no recursion.
CREATE TRIGGER providers_set_updated_at
AFTER UPDATE ON providers
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE providers SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE INDEX providers_state_idx            ON providers (state) WHERE deleted_at IS NULL;
CREATE INDEX providers_license_expires_idx  ON providers (license_expires_on) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- users: humans who can log in. Tied to a provider (tenant).
-- email uses COLLATE NOCASE so 'Foo@Bar.com' and 'foo@bar.com' collide at
-- the unique index.
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id                 TEXT PRIMARY KEY,
    provider_id        TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    email              TEXT NOT NULL COLLATE NOCASE,
    phone              TEXT,
    full_name          TEXT NOT NULL,
    role               TEXT NOT NULL,
    last_login_at      TEXT NULL,
    email_verified_at  TEXT NULL,
    phone_verified_at  TEXT NULL,
    created_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at         TEXT NULL,
    CONSTRAINT users_role_chk CHECK (role IN ('provider_admin','provider_staff'))
);

CREATE TRIGGER users_set_updated_at
AFTER UPDATE ON users
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE UNIQUE INDEX users_email_uidx    ON users (email COLLATE NOCASE);
CREATE INDEX users_provider_id_idx      ON users (provider_id) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- magic_link_tokens: every passwordless flow. Only the HMAC-SHA-256 hash is
-- stored — the raw token is delivered out-of-band (email/SMS) and never
-- persisted.
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
    provider_id    TEXT NULL,
    subject_id     TEXT,
    token_hash     BLOB NOT NULL UNIQUE,
    expires_at     TEXT NOT NULL,
    consumed_at    TEXT NULL,
    last_used_at   TEXT NULL,
    ip             TEXT,
    user_agent     TEXT,
    created_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT magic_link_kind_chk CHECK (
        kind IN ('provider_signup','provider_signin','parent_upload','staff_upload','document_sign')
    )
);

CREATE INDEX magic_link_tokens_token_hash_idx ON magic_link_tokens (token_hash);
CREATE INDEX magic_link_tokens_expires_idx    ON magic_link_tokens (expires_at) WHERE consumed_at IS NULL;
CREATE INDEX magic_link_tokens_subject_idx    ON magic_link_tokens (kind, subject_id);
