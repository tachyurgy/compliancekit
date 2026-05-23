-- 000040_sessions_active_provider.down.sql
DROP INDEX IF EXISTS sessions_active_provider_idx;
ALTER TABLE sessions DROP COLUMN active_provider_id;
