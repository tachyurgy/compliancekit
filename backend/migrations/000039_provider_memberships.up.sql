-- 000039_provider_memberships.up.sql
-- REQ-038 (demo as second facility): a user can now belong to multiple
-- providers. Their REAL provider stays on users.provider_id (home / owner
-- row), and a second is_demo=1 provider is attached via this join.
--
-- We backfill one row per existing user so the table is the authoritative
-- "what providers can this user open" lookup from now on. The demo provider
-- gets its own row when the demo facility is seeded.
--
-- role mirrors users.role today; later REQs can introduce per-membership
-- roles without touching this schema.

CREATE TABLE provider_memberships (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id)     ON DELETE CASCADE,
    provider_id  TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    role         TEXT NOT NULL DEFAULT 'provider_admin',
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, provider_id)
);

CREATE INDEX provider_memberships_user_idx     ON provider_memberships (user_id);
CREATE INDEX provider_memberships_provider_idx ON provider_memberships (provider_id);

-- Backfill: every existing user gets a membership row pointing at their
-- current users.provider_id. lower(hex(randomblob)) is SQLite-flavored and
-- produces a deterministic-ish unique id good enough for the backfill — the
-- column is just a row key, not a base62 id.
INSERT INTO provider_memberships (id, user_id, provider_id, role)
SELECT lower(hex(randomblob(16))), id, provider_id, role
  FROM users
 WHERE deleted_at IS NULL
   AND provider_id IS NOT NULL
   AND provider_id != '';
