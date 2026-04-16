-- 000001_init_providers_and_users.down.sql
-- Drops in reverse dependency order. Extensions stay in place.

BEGIN;

DROP TABLE IF EXISTS magic_link_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS providers;

DROP FUNCTION IF EXISTS set_updated_at();

-- Intentionally leave citext / pgcrypto installed — they may be used by
-- sibling schemas and are harmless to keep.

COMMIT;
