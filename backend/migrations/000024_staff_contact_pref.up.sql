-- Per-staff preferred contact channel for the document nagger.
-- email | sms | both. Default is email so existing rows keep working.
ALTER TABLE staff ADD COLUMN preferred_contact_channel TEXT NOT NULL DEFAULT 'email'
  CHECK (preferred_contact_channel IN ('email','sms','both'));
ALTER TABLE staff ADD COLUMN contact_channel_set_at TEXT;

-- Audit log of every nag that goes out, so operators can see when staff was
-- last contacted about a missing/expiring document. Distinct from the existing
-- document_chase_sends table, which dedups *expiry* sends per threshold.
-- This one logs *every* send (manual or scheduled, missing-doc or expiring).
CREATE TABLE IF NOT EXISTS staff_nag_sends (
  id              TEXT PRIMARY KEY,
  provider_id     TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  staff_id        TEXT NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
  reason          TEXT NOT NULL,                         -- 'missing' | 'expiring' | 'manual'
  doc_kind        TEXT,                                  -- e.g. 'cpr_cert' if reason in (missing,expiring)
  channel         TEXT NOT NULL,                         -- 'email' | 'sms' | 'both'
  email_status    TEXT,                                  -- 'sent' | 'skipped' | 'failed:<reason>'
  sms_status      TEXT,                                  -- 'sent' | 'skipped' | 'failed:<reason>'
  triggered_by    TEXT,                                  -- user_id when manual; '' for scheduled
  created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_staff_nag_sends_staff ON staff_nag_sends (staff_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_staff_nag_sends_provider ON staff_nag_sends (provider_id, created_at DESC);
