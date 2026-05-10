DROP INDEX IF EXISTS idx_kiosk_sessions_provider;
DROP TABLE IF EXISTS kiosk_sessions;
DROP INDEX IF EXISTS idx_attendance_events_subject;
DROP INDEX IF EXISTS idx_attendance_events_provider_time;
DROP TABLE IF EXISTS attendance_events;
-- Columns left in place — SQLite cannot drop them cleanly.
