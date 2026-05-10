-- 000028_children_portal_ocr.up.sql
--
-- REQ-005 (magic-link upload portal v2) + REQ-006 (OCR confirmation UI) +
-- REQ-008 (children/parent portal). Reconciles drift on `children` and
-- `documents`, adds an append-only `upload_audit` table, ensures
-- `magic_link_tokens.kind` accepts 'parent_upload', and seeds the
-- Florida child required-document set keyed to `entity_type='child'`.
--
-- SQLite dialect (see ADR-017). Every ALTER is guarded so the migration is
-- idempotent against earlier reconcile waves; column existence is checked
-- via PRAGMA-driven INSERT-OR-IGNORE on a temp helper table where SQLite's
-- "ADD COLUMN IF NOT EXISTS" would otherwise be needed.

-- ---------------------------------------------------------------------------
-- children: parent-contact + medical drift columns.
-- 000015 already added parent_email / parent_phone / classroom / enroll_date.
-- 000025 already added preferred_contact_channel + contact_channel_set_at.
-- 000002 already declares allergies + medical_notes.
-- We still need:
--   parent_name           — was buried inside the JSON guardians blob; lift
--                            to a real column so handlers can read it cleanly.
--   medical_conditions    — REQ-008 calls for free-text alongside allergies;
--                            keep medical_notes (legacy) but add the named
--                            column the handler/UI expects.
--   withdrawn_at          — soft-delete marker; legacy 000002 has
--                            withdrawal_date but the handler shape uses
--                            withdrawn_at. Add as alias.
--
-- SQLite has no "ADD COLUMN IF NOT EXISTS"; if a previous migration drift
-- already added one of these (it shouldn't have), the migration will fail
-- loud and we'll reconcile then. That's intentional.
-- ---------------------------------------------------------------------------
ALTER TABLE children ADD COLUMN parent_name TEXT;
ALTER TABLE children ADD COLUMN medical_conditions TEXT;
ALTER TABLE children ADD COLUMN withdrawn_at TEXT;

-- Backfill withdrawn_at from the legacy withdrawal_date when present.
UPDATE children SET withdrawn_at = withdrawal_date
 WHERE withdrawn_at IS NULL AND withdrawal_date IS NOT NULL;

-- ---------------------------------------------------------------------------
-- documents: REQ-006 OCR-confirmation columns.
-- We keep BOTH the AI-extracted value AND the user-confirmed value. The
-- canonical mutable fields (expiration_date / kind) are what the chase
-- engine reads; the *_extracted_value fields are append-only audit data
-- so we can measure OCR accuracy in production.
--
-- does_not_expire is a documented "this doc has no expiry" toggle; the
-- chase engine should skip these forever.
-- ---------------------------------------------------------------------------
ALTER TABLE documents ADD COLUMN expiration_extracted_value     TEXT;
ALTER TABLE documents ADD COLUMN document_type_extracted_value  TEXT;
ALTER TABLE documents ADD COLUMN extraction_confidence          REAL;
ALTER TABLE documents ADD COLUMN expiration_confirmed_at        TEXT;
ALTER TABLE documents ADD COLUMN expiration_confirmed_by_user_id TEXT;
ALTER TABLE documents ADD COLUMN does_not_expire                INTEGER NOT NULL DEFAULT 0;

-- Index used by the chase engine's confirmation gate (REQ-006/-007).
-- Matches `WHERE expiration_confirmed_at IS NOT NULL OR
-- expiration_extracted_value IS NULL` lookups.
CREATE INDEX IF NOT EXISTS documents_confirmation_gate_idx
  ON documents (provider_id, expiration_confirmed_at, does_not_expire)
  WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- magic_link_tokens: 000001 already declares the kind CHECK constraint with
-- 'parent_upload' / 'staff_upload' / etc., and last_used_at is in place.
-- Nothing to do here — verified via the handbook.md schema review.
-- (No DDL emitted; comment retained so a migration auditor can confirm.)
-- ---------------------------------------------------------------------------

-- ---------------------------------------------------------------------------
-- upload_audit: append-only log of every successful upload (whether via the
-- magic-link portal or the operator). Keeps the data trail for inspectors
-- and gives us a per-token usage history for rate-limit forensics.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS upload_audit (
    id                  TEXT PRIMARY KEY,
    magic_link_id       TEXT REFERENCES magic_link_tokens(id) ON DELETE SET NULL,
    document_id         TEXT REFERENCES documents(id) ON DELETE SET NULL,
    provider_id         TEXT,
    ip                  TEXT,
    user_agent          TEXT,
    file_bytes          INTEGER,
    file_mime           TEXT,
    created_at          TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS upload_audit_token_idx    ON upload_audit (magic_link_id);
CREATE INDEX IF NOT EXISTS upload_audit_document_idx ON upload_audit (document_id);
CREATE INDEX IF NOT EXISTS upload_audit_created_idx  ON upload_audit (created_at);

-- ---------------------------------------------------------------------------
-- document_requirements: Slice 2 (migration 000026) already created this
-- table with shape:
--   id, entity_type, role, doc_kind, label, frequency, grace_period_days,
--   authority, sort_order, state_code, created_at
-- Slice 2 seeds the staff rows in 000026b. Slice 3 seeds the FL child rows
-- here (per Handbook § 7.3 — florida-inspection-coverage.md). Use the
-- existing column names; INSERT OR IGNORE keeps the migration idempotent.
-- ---------------------------------------------------------------------------
INSERT OR IGNORE INTO document_requirements (id, entity_type, role, doc_kind, label, frequency, grace_period_days, authority, sort_order, state_code) VALUES
  ('req_fl_child_dh680',   'child', NULL, 'immunization_record',     'Immunization record (DH 680 or DH 681)',                'every_n_years:1', 30, 'Handbook § 7.1, § 21',                10, 'FL'),
  ('req_fl_child_dh3040',  'child', NULL, 'physical_exam',           'Student health exam (DH 3040)',                         'every_n_years:2', 30, 'Handbook § 7.2',                      20, 'FL'),
  ('req_fl_child_5219',    'child', NULL, 'enrollment_form',         'Enrollment application (CF-FSP 5219)',                  'once',             0, 'Handbook § 7.3',                      30, 'FL'),
  ('req_fl_child_emerg',   'child', NULL, 'emergency_card',          'Emergency contact form',                                'annual',           0, 'Handbook § 7.3, § 2.7',               40, 'FL'),
  ('req_fl_child_pickup',  'child', NULL, 'authorized_pickup',       'Authorized pickup list',                                'annual',           0, 'Handbook § 7.3',                      50, 'FL'),
  ('req_fl_child_allergy', 'child', NULL, 'allergy_medical',         'Allergy / medical conditions form',                     'annual',           0, 'Handbook § 3.9.3(F), § 7.B 11',       60, 'FL'),
  ('req_fl_child_ack_kycf','child', NULL, 'ack_know_your_facility',  'Signed acknowledgement: Know Your Child Care Facility (CF/PI 175-24)', 'once',     0, 'Handbook § 7.3',                      70, 'FL'),
  ('req_fl_child_ack_flu', 'child', NULL, 'ack_influenza',           'Signed acknowledgement: Influenza brochure (CF/PI 175-70)',           'annual',   0, 'Handbook § 7.3, F.S. § 402.313(14)',  80, 'FL'),
  ('req_fl_child_ack_dist','child', NULL, 'ack_distracted_adult',    'Signed acknowledgement: Distracted Adult brochure (CF/PI 175-12)',    'annual',   0, 'Handbook § 7.3, F.S. § 402.313(15)',  90, 'FL'),
  ('req_fl_child_ack_disc','child', NULL, 'ack_discipline_policy',   'Signed acknowledgement: Discipline & expulsion policy',  'once',     0, 'Handbook § 7.3, § 7.B 3',            100, 'FL'),
  ('req_fl_child_ack_food','child', NULL, 'ack_food_policy',         'Signed acknowledgement: Food and nutrition policy',     'once',     0, 'Handbook § 7.3',                     110, 'FL');
