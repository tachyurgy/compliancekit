-- 000009_sessions_chase_drills.down.sql
-- SQLite supports DROP COLUMN since 3.35 but only one per statement.

ALTER TABLE providers DROP COLUMN postings_checked_at;
ALTER TABLE providers DROP COLUMN ratio_checked_at;
ALTER TABLE providers DROP COLUMN postings_complete;
ALTER TABLE providers DROP COLUMN ratio_ok;

DROP TABLE IF EXISTS drill_logs;
DROP TABLE IF EXISTS document_chase_sends;
DROP TABLE IF EXISTS sessions;
