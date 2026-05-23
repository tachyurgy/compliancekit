-- 000040_sessions_active_provider.up.sql
-- REQ-038 (demo as second facility): which provider the user is currently
-- viewing lives on the session row. Defaults to the original sessions.provider_id
-- so existing sessions keep working without a backfill (the LookupSession
-- helper falls back to provider_id when active_provider_id IS NULL).

ALTER TABLE sessions ADD COLUMN active_provider_id TEXT REFERENCES providers(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS sessions_active_provider_idx ON sessions (active_provider_id) WHERE active_provider_id IS NOT NULL;
