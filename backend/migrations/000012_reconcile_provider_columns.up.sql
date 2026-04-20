-- 000012_reconcile_provider_columns
--
-- Reconcile handler SQL with the initial 000001 schema. Several handlers
-- (providers.go Signup/Me/Update) reference `name`, `owner_email`,
-- `state_code`, and `capacity` columns that the original migration did not
-- include. Integration tests caught the mismatch. Rather than rewrite a dozen
-- SQL statements across multiple handlers in a 3-day sprint window, we add
-- the columns here so the runtime schema matches the code.
--
-- Semantics:
--   name         — display name (== dba if set else legal_name for existing rows)
--   owner_email  — primary admin contact for the provider; used as the upsert
--                  key during signup. Unique where non-null.
--   state_code   — uppercased 2-char state code. The `state` column continues
--                  to exist; keep both in sync on writes (handlers already do).
--   capacity     — integer, licensed facility capacity.

ALTER TABLE providers ADD COLUMN name TEXT;
UPDATE providers SET name = COALESCE(NULLIF(dba, ''), legal_name) WHERE name IS NULL;

ALTER TABLE providers ADD COLUMN owner_email TEXT;
-- owner_email has no non-trivial backfill source; leave NULL for existing rows.

ALTER TABLE providers ADD COLUMN state_code TEXT;
UPDATE providers SET state_code = UPPER(state) WHERE state_code IS NULL;

ALTER TABLE providers ADD COLUMN capacity INTEGER NOT NULL DEFAULT 0;

-- Full unique index (not partial) so the ON CONFLICT (owner_email) upsert in
-- handlers/providers.go Signup resolves against it. SQLite treats NULLs as
-- distinct by default, so pre-existing rows with NULL owner_email do not
-- collide with each other or with new signups.
CREATE UNIQUE INDEX IF NOT EXISTS idx_providers_owner_email
    ON providers(owner_email);
