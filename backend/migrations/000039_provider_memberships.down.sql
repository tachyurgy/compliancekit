-- 000039_provider_memberships.down.sql
DROP INDEX IF EXISTS provider_memberships_provider_idx;
DROP INDEX IF EXISTS provider_memberships_user_idx;
DROP TABLE IF EXISTS provider_memberships;
