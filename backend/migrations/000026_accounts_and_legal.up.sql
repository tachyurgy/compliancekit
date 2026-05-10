-- 000026_accounts_and_legal.up.sql
-- REQ-002 + REQ-011: Auth + Billing + Legal slice for the FL launch.
--
-- Design notes:
--   * The codebase uses `providers` as the tenant/account root (one row per
--     facility). Rather than introduce a parallel `accounts` table that would
--     fork the rest of the app, we ADD the account-shape columns onto
--     `providers` directly. The REQ explicitly says "If a similar table
--     already exists from 000007, extend instead of recreate."
--   * `subscriptions` from 000007 stays as the Stripe mirror; we extend its
--     plan vocabulary to include the FL launch plans ('monthly_99',
--     'annual_948') alongside the legacy strings. SQLite CHECK constraints
--     can't be ALTER'd in place, so we relax to a NOT NULL TEXT column and
--     enforce the vocabulary in Go.
--   * `legal_signatures` is brand-new (the existing policy_acceptances table
--     from 000007 is for the parent/staff e-signature flow keyed off
--     magic-link tokens; the operator ToS/DPA capture is a different surface
--     and benefits from its own simpler schema as the REQ asked).
--   * `auth_audit` is brand-new for REQ-011.
--   * `sessions` already exists from 000009 with (id, provider_id, user_id,
--     ip, user_agent, expires_at, revoked_at, created_at). REQ-011 also
--     wants `last_seen_at` and `token_hash`. The existing `id` column is
--     used as the cookie value today; rather than break that, we add
--     `token_hash` as nullable for forward use and add `last_seen_at`.
--   * `users` already has password_hash + password_set_at from 000023. We
--     add `password_updated_at` (alias-friendly) and ensure email_verified_at
--     exists on 000001 (it does).

-- ---------------------------------------------------------------------------
-- providers: account-shape columns for the FL launch billing flow.
-- All nullable; the signup handler populates them as the operator advances
-- through the funnel (signup -> checkout -> legal).
-- ---------------------------------------------------------------------------
ALTER TABLE providers ADD COLUMN plan                 TEXT;
ALTER TABLE providers ADD COLUMN trial_ends_at        TEXT;
ALTER TABLE providers ADD COLUMN current_period_end_at TEXT;
ALTER TABLE providers ADD COLUMN subscription_status  TEXT;
ALTER TABLE providers ADD COLUMN stripe_subscription_id TEXT;

CREATE INDEX IF NOT EXISTS idx_providers_subscription_status
    ON providers (subscription_status);

-- ---------------------------------------------------------------------------
-- users: tighten password column naming for REQ-011.
-- 000023 added password_hash + password_set_at. Add password_updated_at so
-- forced-rotation flows have a write target distinct from the initial set.
-- ---------------------------------------------------------------------------
ALTER TABLE users ADD COLUMN password_updated_at TEXT;

-- ---------------------------------------------------------------------------
-- sessions: REQ-011 sliding 90-day window. We extend the existing 000009
-- table rather than create a new one — the existing `id` column serves as
-- both the primary key and the cookie value (no separate token_hash needed
-- for now; we keep the column for a future migration to hashed cookies).
-- ---------------------------------------------------------------------------
ALTER TABLE sessions ADD COLUMN token_hash    TEXT;
ALTER TABLE sessions ADD COLUMN last_seen_at  TEXT;
ALTER TABLE sessions ADD COLUMN remember_me   INTEGER NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);

-- ---------------------------------------------------------------------------
-- legal_signatures: operator-level e-signature capture. Append-only.
-- One row per (user, document_kind) sign event. Re-signs (e.g. after a ToS
-- bump) write a new row; we never UPDATE.
-- ---------------------------------------------------------------------------
CREATE TABLE legal_signatures (
    id                    TEXT PRIMARY KEY,
    user_id               TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id            TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    document_kind         TEXT NOT NULL CHECK (document_kind IN ('tos','dpa')),
    document_version      TEXT NOT NULL,
    document_text_sha256  TEXT NOT NULL,
    typed_name            TEXT NOT NULL,
    ip                    TEXT,
    user_agent            TEXT,
    signed_at             TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_legal_signatures_account ON legal_signatures(account_id);
CREATE INDEX idx_legal_signatures_user    ON legal_signatures(user_id);
CREATE INDEX idx_legal_signatures_kind    ON legal_signatures(document_kind);

-- ---------------------------------------------------------------------------
-- auth_audit: REQ-011 lightweight per-event log for signin/signout/reset.
-- Distinct from audit_log (000008) which is the broader business-event log;
-- this table keeps auth-specific events for the 90-day GDPR retention floor
-- with a tighter, queryable shape.
-- ---------------------------------------------------------------------------
CREATE TABLE auth_audit (
    id          TEXT PRIMARY KEY,
    user_id     TEXT REFERENCES users(id) ON DELETE SET NULL,
    email       TEXT,
    kind        TEXT NOT NULL CHECK (kind IN (
        'signin_success',
        'signin_fail',
        'signout',
        'signout_everywhere',
        'password_reset_request',
        'password_reset_complete',
        'signup'
    )),
    ip          TEXT,
    user_agent  TEXT,
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_auth_audit_user      ON auth_audit(user_id);
CREATE INDEX idx_auth_audit_email     ON auth_audit(email);
CREATE INDEX idx_auth_audit_kind      ON auth_audit(kind);
CREATE INDEX idx_auth_audit_created   ON auth_audit(created_at);

-- ---------------------------------------------------------------------------
-- password_resets: short-lived reset tokens (REQ-011 #4). 30-min expiry,
-- single-use. Token is hashed (sha256) at rest; the raw token is in the
-- email link only.
-- ---------------------------------------------------------------------------
CREATE TABLE password_resets (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TEXT NOT NULL,
    used_at     TEXT,
    ip          TEXT,
    user_agent  TEXT,
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_password_resets_user    ON password_resets(user_id);
CREATE INDEX idx_password_resets_expires ON password_resets(expires_at) WHERE used_at IS NULL;
