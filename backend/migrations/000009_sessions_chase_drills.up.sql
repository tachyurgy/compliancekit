-- Migration 000009: close gaps surfaced by the Go handler code.
-- Adds: sessions, document_chase_sends, drill_logs; providers.ratio_ok,
-- providers.postings_complete. SQLite dialect (see ADR-017).

-- Server-side session store. Cookie value is the session id (base62).
-- Rotated on privilege change; purge job removes expired rows.
CREATE TABLE sessions (
  id            TEXT PRIMARY KEY,
  provider_id   TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  ip            TEXT,
  user_agent    TEXT,
  expires_at    TEXT NOT NULL,
  revoked_at    TEXT,
  created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX idx_sessions_user_id    ON sessions(user_id);

-- Dedup table for the chase service. A (document_id, threshold_days, channel)
-- tuple is sent at most once. `chase_events` remains the long-form audit log.
CREATE TABLE document_chase_sends (
  document_id     TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  threshold_days  INTEGER NOT NULL,
  channel         TEXT NOT NULL DEFAULT 'email' CHECK (channel IN ('email','sms','inapp')),
  sent_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (document_id, threshold_days, channel)
);

CREATE INDEX idx_document_chase_sends_sent_at ON document_chase_sends(sent_at);

-- Fire/emergency drill log. States require monthly or quarterly cadence; the
-- compliance engine counts rows in this table over a 90-day window.
CREATE TABLE drill_logs (
  id                     TEXT PRIMARY KEY,
  provider_id            TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  drill_kind             TEXT NOT NULL CHECK (drill_kind IN ('fire','tornado','lockdown','earthquake','evacuation','other')),
  drill_date             TEXT NOT NULL,
  logged_by_user_id      TEXT REFERENCES users(id) ON DELETE SET NULL,
  duration_seconds       INTEGER,
  notes                  TEXT,
  attachment_document_id TEXT REFERENCES documents(id) ON DELETE SET NULL,
  created_at             TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at             TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_drill_logs_provider_date ON drill_logs(provider_id, drill_date DESC);

CREATE TRIGGER trg_drill_logs_updated_at
AFTER UPDATE ON drill_logs
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE drill_logs SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- Operational flags the dashboard reads directly. Computed daily by a worker
-- from staff:child ratios and wall-posting checklist completion; cached here
-- to make the dashboard endpoint a single round-trip.
-- SQLite ALTER TABLE supports only one ADD COLUMN per statement.
ALTER TABLE providers ADD COLUMN ratio_ok            INTEGER NOT NULL DEFAULT 1;
ALTER TABLE providers ADD COLUMN postings_complete   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE providers ADD COLUMN ratio_checked_at    TEXT;
ALTER TABLE providers ADD COLUMN postings_checked_at TEXT;
