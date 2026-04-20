-- 000010_facility_postings.down.sql
-- SQLite supports DROP COLUMN since 3.35 but only one per statement.

DROP INDEX IF EXISTS idx_drill_logs_provider_active;
ALTER TABLE drill_logs DROP COLUMN deleted_at;

DROP TRIGGER IF EXISTS trg_providers_postings_json_valid_upd;
DROP TRIGGER IF EXISTS trg_providers_postings_json_valid_ins;
ALTER TABLE providers DROP COLUMN facility_postings;
