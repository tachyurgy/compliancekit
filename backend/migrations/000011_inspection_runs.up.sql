-- Migration 000011: Inspection Readiness Simulator.
-- Stores a run (one walk-through) plus one response row per answered item.
-- Checklist items themselves are in-code under internal/inspection and are
-- joined by the immutable item.ID string. That keeps regulatory text under
-- version control and eliminates a stale-data problem.

CREATE TABLE inspection_runs (
  id              TEXT PRIMARY KEY,
  provider_id     TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  state           TEXT NOT NULL,
  started_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at    DATETIME,
  score           INTEGER,
  total_items     INTEGER NOT NULL,
  items_passed    INTEGER NOT NULL DEFAULT 0,
  items_failed    INTEGER NOT NULL DEFAULT 0,
  items_na        INTEGER NOT NULL DEFAULT 0,
  created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_inspection_runs_provider ON inspection_runs(provider_id, started_at DESC);

CREATE TABLE inspection_responses (
  id                    TEXT PRIMARY KEY,
  run_id                TEXT NOT NULL REFERENCES inspection_runs(id) ON DELETE CASCADE,
  item_id               TEXT NOT NULL,
  answer                TEXT NOT NULL CHECK (answer IN ('pass','fail','na')),
  evidence_document_id  TEXT REFERENCES documents(id) ON DELETE SET NULL,
  note                  TEXT,
  answered_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_inspection_responses_run_item ON inspection_responses(run_id, item_id);
