DROP INDEX IF EXISTS idx_providers_owner_email;
ALTER TABLE providers DROP COLUMN capacity;
ALTER TABLE providers DROP COLUMN state_code;
ALTER TABLE providers DROP COLUMN owner_email;
ALTER TABLE providers DROP COLUMN name;
