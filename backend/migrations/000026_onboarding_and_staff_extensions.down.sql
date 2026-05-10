-- 000026 down: drop the tables we created; ALTER TABLE columns are left
-- in place because SQLite does not natively support DROP COLUMN before 3.35
-- and the cost of the workaround (table copy) is not worth the rollback
-- safety here — these migrations are forward-only in practice.

DROP TABLE IF EXISTS staff_activity_events;
DROP TABLE IF EXISTS document_requirements;
DROP TABLE IF EXISTS rooms;
