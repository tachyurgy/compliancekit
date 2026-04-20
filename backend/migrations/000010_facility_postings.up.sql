-- 000010_facility_postings.up.sql
-- Adds per-provider wall-posting checklist state (JSON blob) and a soft-delete
-- column on drill_logs so the operations feature can hide removed rows without
-- losing audit trail. SQLite dialect (see ADR-017).
--
-- Why a JSON blob instead of a new table? The checklist is small (~7 items),
-- read/written as a whole unit, never joined. A single TEXT column keeps
-- migrations cheap and writes atomic. We validate with json_valid() at write
-- time. If the checklist grows to dozens of items or we need cross-provider
-- analytics, we'll promote to a wall_postings table.

ALTER TABLE providers ADD COLUMN facility_postings TEXT NOT NULL DEFAULT '{}';

-- SQLite CHECK constraints on ALTER TABLE ADD COLUMN apply to future writes
-- only (existing rows are grandfathered). json_valid('{}') is trivially true.
-- A trigger enforces validity going forward.
CREATE TRIGGER trg_providers_postings_json_valid_ins
BEFORE INSERT ON providers
FOR EACH ROW
WHEN NEW.facility_postings IS NOT NULL AND NOT json_valid(NEW.facility_postings)
BEGIN
    SELECT RAISE(ABORT, 'facility_postings must be valid JSON');
END;

CREATE TRIGGER trg_providers_postings_json_valid_upd
BEFORE UPDATE OF facility_postings ON providers
FOR EACH ROW
WHEN NEW.facility_postings IS NOT NULL AND NOT json_valid(NEW.facility_postings)
BEGIN
    SELECT RAISE(ABORT, 'facility_postings must be valid JSON');
END;

-- Soft delete for drill_logs — removed drills stay on disk for the audit trail
-- but are hidden from the compliance engine's 90-day count.
ALTER TABLE drill_logs ADD COLUMN deleted_at TEXT;

CREATE INDEX idx_drill_logs_provider_active
    ON drill_logs(provider_id, drill_date DESC)
    WHERE deleted_at IS NULL;
