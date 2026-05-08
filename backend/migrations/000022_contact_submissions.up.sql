CREATE TABLE IF NOT EXISTS contact_submissions (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  email       TEXT NOT NULL,
  company     TEXT,
  state       TEXT,
  topic       TEXT,
  message     TEXT NOT NULL,
  source_ip   TEXT,
  user_agent  TEXT,
  created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_contact_submissions_created_at
  ON contact_submissions (created_at DESC);
