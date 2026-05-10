-- Migration 000029: Inspector Mode (REQ-009).
--
-- Adds two tables that back the live, iPad-friendly Inspector Mode used during
-- a DCF Licensing Counselor visit. Distinct from migration 000011's
-- inspection_runs / inspection_responses, which back the *self-inspection
-- simulator* (a guided pre-visit walk-through driven by an in-code checklist).
-- Inspector Mode is the real-time surface the operator hands the actual
-- inspector — different surface, different lifecycle, different writers.
--
-- We deliberately do NOT touch 000011's tables. The two features live side by
-- side: simulator = pre-flight rehearsal; inspector_events = live event log.
--
-- SQLite dialect (see ADR-017). All timestamps are TEXT in RFC3339/ISO-8601.

-- ---------------------------------------------------------------------------
-- inspection_events: one row per Inspector Mode session opened. The session
-- captures the inspector's name, the deterministic-random sample of
-- staff/children pulled at session start, and the running list of findings
-- entered during the exit conference. When the session ends, a Plan of
-- Correction PDF is generated and its S3 key (or local path) is stored on
-- plan_of_correction_pdf_path.
-- ---------------------------------------------------------------------------
CREATE TABLE inspection_events (
    id                            TEXT PRIMARY KEY,
    facility_id                   TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    started_at                    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at                      TEXT,
    counselor_name                TEXT,
    type                          TEXT NOT NULL DEFAULT 'routine',
    sample_json                   TEXT NOT NULL DEFAULT '{}',
    findings_json                 TEXT NOT NULL DEFAULT '[]',
    plan_of_correction_pdf_path   TEXT,
    started_by_user_id            TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at                    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT inspection_events_type_chk     CHECK (type IN ('routine','renewal','complaint','followup','other')),
    CONSTRAINT inspection_events_sample_json  CHECK (json_valid(sample_json)),
    CONSTRAINT inspection_events_findings_json CHECK (json_valid(findings_json))
);

CREATE INDEX inspection_events_facility_started_idx
    ON inspection_events (facility_id, started_at DESC);

CREATE TRIGGER inspection_events_set_updated_at
AFTER UPDATE ON inspection_events
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE inspection_events SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- ---------------------------------------------------------------------------
-- posting_photos: photo proof for every required wall posting per Handbook
-- § 19. The Inspector Mode "Postings" tab renders one row per posting kind
-- with the most recent photo, last_verified_at, and the room (if posting is
-- per-room — capacity, evac plan).
--
-- room_id is nullable: facility-wide postings (license, emergency phone
-- numbers, weekly menu) leave it NULL; per-room postings (capacity, evac
-- plan) populate it. We don't FK room_id to a rooms table because the rooms
-- table is not yet a stable schema (REQ-003 onboarding stores rooms in JSON
-- on providers).
-- ---------------------------------------------------------------------------
CREATE TABLE posting_photos (
    id                    TEXT PRIMARY KEY,
    facility_id           TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    posting_kind          TEXT NOT NULL,
    room_id               TEXT,
    photo_path            TEXT NOT NULL,
    last_verified_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    verified_by_user_id   TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at            TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX posting_photos_facility_kind_idx
    ON posting_photos (facility_id, posting_kind);
CREATE INDEX posting_photos_facility_verified_idx
    ON posting_photos (facility_id, last_verified_at DESC);

CREATE TRIGGER posting_photos_set_updated_at
AFTER UPDATE ON posting_photos
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE posting_photos SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
