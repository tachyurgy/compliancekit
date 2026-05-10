-- Migration 000030: REQ-013 compliance_score_snapshots.
--
-- Append-only daily snapshots of the synthesized 0-100 score plus its
-- per-category breakdown.  We already have `compliance_snapshots` from
-- earlier migrations holding the raw engine Report payload; this new table
-- holds the *synthesized* score from internal/compliance/score.go so we can
-- render a 30-day sparkline without re-running the engine on read.
--
-- One row per provider per day.  Index on (provider_id, calculated_at DESC)
-- supports the 30-day query path.
--
-- SQLite dialect.  Timestamps are TEXT in RFC3339 / ISO-8601.

CREATE TABLE compliance_score_snapshots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id     TEXT    NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    score           INTEGER NOT NULL,
    band            TEXT    NOT NULL CHECK (band IN ('excellent','solid','attention','urgent')),
    breakdown_json  TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(breakdown_json)),
    calculated_at   TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_compliance_score_snapshots_provider_calc
    ON compliance_score_snapshots (provider_id, calculated_at DESC);
