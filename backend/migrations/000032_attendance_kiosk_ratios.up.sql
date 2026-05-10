-- Migration 000032: REQ-016 daily attendance + kiosk PINs.
--
-- After three rounds of design (active/product/inspection-tracking.html)
-- the v1 spec is no-rooms, no-ratio-coach: just attendance events, kiosk
-- sessions, parent + staff PINs, and the per-child expected-days schedule.
-- Ratio "warnings" are computed on the fly from today's headcount and age
-- mix in internal/compliance/ratio.go — no per-room tables required.
--
-- Tables added:
--   * attendance_events     — append-only sign-in/out log
--   * kiosk_sessions        — short-lived facility-scoped tokens for the
--                             mounted iPad (4-hour TTL).
--
-- Columns added:
--   * children.expected_days, children.parent_pin_hash
--   * staff.kiosk_pin_hash
--
-- All PINs are bcrypt'd at rest. The raw 4-digit PIN is shown to the
-- operator exactly once (on issue / rotation) and never persisted.

-- ---- attendance --------------------------------------------------------

CREATE TABLE attendance_events (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id       TEXT    NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    kind              TEXT    NOT NULL CHECK (kind IN ('child_in','child_out','staff_in','staff_out')),
    subject_kind      TEXT    NOT NULL CHECK (subject_kind IN ('child','staff')),
    subject_id        TEXT    NOT NULL,
    dropped_off_by    TEXT,                            -- typed name, child_in only
    picked_up_by      TEXT,                            -- typed name, child_out only
    timestamp_utc     TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    kiosk_session_id  TEXT,
    notes             TEXT
);

CREATE INDEX idx_attendance_events_provider_time
    ON attendance_events (provider_id, timestamp_utc DESC);
CREATE INDEX idx_attendance_events_subject
    ON attendance_events (provider_id, subject_kind, subject_id, timestamp_utc DESC);

-- ---- kiosk sessions -----------------------------------------------------

CREATE TABLE kiosk_sessions (
    id            TEXT PRIMARY KEY,                    -- random base62
    provider_id   TEXT    NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    label         TEXT,                                -- "front door iPad", etc.
    issued_at     TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at    TEXT    NOT NULL,                    -- 4h after issue
    last_seen_at  TEXT
);

CREATE INDEX idx_kiosk_sessions_provider ON kiosk_sessions (provider_id);

-- ---- PIN + expected-days columns on children/staff ---------------------

ALTER TABLE children ADD COLUMN expected_days   TEXT;        -- JSON array, e.g. ["mon","tue","wed","thu","fri"]
ALTER TABLE children ADD COLUMN parent_pin_hash TEXT;        -- bcrypt'd 4-digit PIN

ALTER TABLE staff    ADD COLUMN kiosk_pin_hash  TEXT;
