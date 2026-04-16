-- Migration 000009: close gaps surfaced by the Go handler code.
-- Adds: sessions, document_chase_sends, drill_logs; providers.ratio_ok, providers.postings_complete.

BEGIN;

-- Server-side session store. Cookie value is the session id (base62, 32 bytes).
-- Rotated on privilege change; purge job removes expired rows.
CREATE TABLE sessions (
  id            TEXT PRIMARY KEY,
  provider_id   TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  ip            INET,
  user_agent    TEXT,
  expires_at    TIMESTAMPTZ NOT NULL,
  revoked_at    TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX idx_sessions_user_id    ON sessions(user_id);

-- Dedup table for the chase service. A (document_id, threshold_days) pair is
-- sent at most once. `chase_events` remains the long-form audit log.
CREATE TABLE document_chase_sends (
  document_id     TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  threshold_days  SMALLINT NOT NULL,
  channel         TEXT NOT NULL DEFAULT 'email' CHECK (channel IN ('email','sms','inapp')),
  sent_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (document_id, threshold_days, channel)
);

CREATE INDEX idx_document_chase_sends_sent_at ON document_chase_sends(sent_at);

-- Fire/emergency drill log. States require monthly or quarterly cadence; the
-- compliance engine counts rows in this table over a 90-day window.
CREATE TABLE drill_logs (
  id            TEXT PRIMARY KEY,
  provider_id   TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  drill_kind    TEXT NOT NULL CHECK (drill_kind IN ('fire','tornado','lockdown','earthquake','evacuation','other')),
  drill_date    TIMESTAMPTZ NOT NULL,
  logged_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  duration_seconds  INTEGER,
  notes         TEXT,
  attachment_document_id TEXT REFERENCES documents(id) ON DELETE SET NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_drill_logs_provider_date ON drill_logs(provider_id, drill_date DESC);
CREATE TRIGGER trg_drill_logs_updated_at
  BEFORE UPDATE ON drill_logs
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Operational flags the dashboard reads directly. Computed daily by a worker
-- from staff:child ratios and wall-posting checklist completion; cached here
-- to make the dashboard endpoint a single round-trip.
ALTER TABLE providers
  ADD COLUMN ratio_ok           BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN postings_complete  BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN ratio_checked_at   TIMESTAMPTZ,
  ADD COLUMN postings_checked_at TIMESTAMPTZ;

COMMIT;
