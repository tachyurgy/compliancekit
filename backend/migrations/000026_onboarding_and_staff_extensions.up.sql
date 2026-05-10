-- 000026_onboarding_and_staff_extensions
--
-- REQ-003 (daycare onboarding wizard) + REQ-004 (employee detail page).
--
-- The pre-assigned migration number for this batch was 000027 in the REQ
-- text, but 000026 was unused, so we slot in here. The seed of
-- document_requirements lives in the companion 000026b_seed file.
--
-- This migration:
--   * extends `providers` (the facility row) with FL-launch fields used by
--     the 4-step onboarding wizard. The `providers` table doubles as the
--     facility table — we did not split them.
--   * creates a `rooms` table (one row per licensed classroom) per
--     Handbook § 3.4.1(A). The wizard's step 4 writes here.
--   * adds `archived_at` to `staff` so departing employees stop nagging
--     without losing the audit trail.
--   * creates `document_requirements` — the static-config table that
--     drives the per-role required-doc checklist on the staff detail page.
--   * creates `staff_activity_events` so the staff detail page's timeline
--     has a single source of truth (uploads, reminders, replacements).
--
-- All ALTER TABLE statements are idempotent against a fresh DB; existing
-- rows backfill cleanly with NULLs / 0 / unknown defaults.

-- ---------------------------------------------------------------------------
-- providers extensions (FL-onboarding fields).
-- ---------------------------------------------------------------------------
-- evening_license: facility licensed for evening child care (Handbook § 2.4.3).
-- weekend_license: licensed for weekend operation.
-- twenty_four_hour: 24-hour-care license (Handbook § 2.4.4).
-- outdoor_playground: presence of an outdoor play area (Handbook § 3.3).
-- indoor_sqft: total indoor square footage; ratio calculator divides by 35.
-- county: FL county (drives the 5-county local-agency overlay; see coverage doc).
-- onboarding_completed_at: timestamp the operator clicked "Finish onboarding".
--
-- providers.phone already exists (000019), so we do not re-add it.
-- providers.license_expires_on already exists (000001) — covers license expiry.
ALTER TABLE providers ADD COLUMN evening_license     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE providers ADD COLUMN weekend_license     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE providers ADD COLUMN twenty_four_hour    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE providers ADD COLUMN outdoor_playground  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE providers ADD COLUMN indoor_sqft         INTEGER;
ALTER TABLE providers ADD COLUMN county              TEXT;
ALTER TABLE providers ADD COLUMN owner_full_name     TEXT;
ALTER TABLE providers ADD COLUMN director_full_name  TEXT;
ALTER TABLE providers ADD COLUMN director_email      TEXT;
ALTER TABLE providers ADD COLUMN onboarding_completed_at TEXT;

-- ---------------------------------------------------------------------------
-- rooms: one row per classroom inside a facility.
-- age_band uses FL's bands (see REQ-003 step 4): infant_0_11, ones, twos,
-- threes, fours, fives, school_age. Free-text TEXT so future states' bands
-- (TX/CA) can flow through without a migration.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rooms (
    id                 TEXT PRIMARY KEY,
    provider_id        TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    age_band           TEXT,
    licensed_capacity  INTEGER,
    indoor_sqft        INTEGER,
    created_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at         TEXT NULL
);

CREATE TRIGGER IF NOT EXISTS rooms_set_updated_at
AFTER UPDATE ON rooms
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE rooms SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE INDEX IF NOT EXISTS rooms_provider_idx ON rooms (provider_id) WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- staff: archived_at for soft-departure (REQ-004).
-- We already had a `status` column with 'terminated' but the REQ explicitly
-- asks for archived_at so the chase engine can simply WHERE archived_at IS
-- NULL without learning a new vocabulary.
-- ---------------------------------------------------------------------------
ALTER TABLE staff ADD COLUMN archived_at TEXT;

-- ---------------------------------------------------------------------------
-- document_requirements: the source of truth for which docs each role needs.
-- entity_type: 'staff' | 'child' (we seed staff in 000026b; children later).
-- role: applies to staff only; NULL for child rows. 'all' means every role.
-- doc_kind: matches documents.kind / DocumentKind in models.go.
-- frequency: 'once' | 'annual' | 'every_n_years:N' | 'every_n_months:N'
-- grace_period_days: how long after frequency lapses before flagging missing.
-- authority: free-text citation for tooltips ("Handbook § 4.2.4").
-- sort_order: rendering order on the detail page.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS document_requirements (
    id                 TEXT PRIMARY KEY,
    entity_type        TEXT NOT NULL,
    role               TEXT,
    doc_kind           TEXT NOT NULL,
    label              TEXT NOT NULL,
    frequency          TEXT NOT NULL,
    grace_period_days  INTEGER NOT NULL DEFAULT 0,
    authority          TEXT,
    sort_order         INTEGER NOT NULL DEFAULT 0,
    state_code         TEXT NOT NULL DEFAULT 'FL',
    created_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT document_requirements_entity_chk CHECK (entity_type IN ('staff', 'child'))
);

CREATE INDEX IF NOT EXISTS document_requirements_lookup_idx
    ON document_requirements (state_code, entity_type, role);

-- ---------------------------------------------------------------------------
-- staff_activity_events: timeline rows for the staff detail page.
-- event_kind: 'document_uploaded' | 'document_replaced' | 'reminder_sent' |
--             'status_change' | 'archived' | 'created'.
-- detail TEXT is JSON-encoded payload for rich rendering.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS staff_activity_events (
    id           TEXT PRIMARY KEY,
    provider_id  TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    staff_id     TEXT NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
    event_kind   TEXT NOT NULL,
    summary      TEXT NOT NULL,
    detail       TEXT,
    actor        TEXT,
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS staff_activity_events_staff_idx
    ON staff_activity_events (staff_id, created_at DESC);
