-- Reverse 000026.
-- SQLite cannot DROP COLUMN before 3.35; the project DB is built fresh in test
-- so a destructive down here is acceptable. We DROP the new tables and leave
-- the ALTER ADD COLUMN entries (no-op down) to avoid breaking older clients
-- still pointing at the same DB file.

DROP INDEX IF EXISTS idx_password_resets_expires;
DROP INDEX IF EXISTS idx_password_resets_user;
DROP TABLE IF EXISTS password_resets;

DROP INDEX IF EXISTS idx_auth_audit_created;
DROP INDEX IF EXISTS idx_auth_audit_kind;
DROP INDEX IF EXISTS idx_auth_audit_email;
DROP INDEX IF EXISTS idx_auth_audit_user;
DROP TABLE IF EXISTS auth_audit;

DROP INDEX IF EXISTS idx_legal_signatures_kind;
DROP INDEX IF EXISTS idx_legal_signatures_user;
DROP INDEX IF EXISTS idx_legal_signatures_account;
DROP TABLE IF EXISTS legal_signatures;

DROP INDEX IF EXISTS idx_sessions_token_hash;
DROP INDEX IF EXISTS idx_providers_subscription_status;
