-- 000006_compliance_and_notifications.down.sql

BEGIN;

DROP TABLE IF EXISTS notification_suppressions;
DROP TABLE IF EXISTS chase_events;
DROP TABLE IF EXISTS compliance_snapshots;

COMMIT;
