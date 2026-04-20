-- 000002_children.up.sql
-- Children enrolled at a provider + the document checklist rows we chase on.
-- SQLite dialect (see ADR-017).

-- ---------------------------------------------------------------------------
-- children: one row per enrolled child. Soft-deletable so we preserve history
-- for audits (a state inspector may ask about a child who left months ago).
--
-- guardians is TEXT holding a JSON array (SQLite stores json as TEXT; the
-- json1 extension functions — json_valid, json_type, json_extract — are
-- built in). Shape is flexible (1-4 entries, mixed custody, variable
-- contact fields). It is NOT a source of truth for relational queries —
-- if we need to send to all guardians across all children we derive a
-- flat table or use json_each.
-- ---------------------------------------------------------------------------
CREATE TABLE children (
    id                TEXT PRIMARY KEY,
    provider_id       TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    first_name        TEXT NOT NULL,
    last_name         TEXT NOT NULL,
    date_of_birth     TEXT NOT NULL,
    enrollment_date   TEXT NOT NULL,
    withdrawal_date   TEXT NULL,
    guardians         TEXT NOT NULL DEFAULT '[]',
    allergies         TEXT,
    medical_notes     TEXT,
    created_at        TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at        TEXT NULL,
    CONSTRAINT children_guardians_is_json_array CHECK (
        json_valid(guardians) AND json_type(guardians) = 'array'
    )
);

CREATE TRIGGER children_set_updated_at
AFTER UPDATE ON children
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE children SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE INDEX children_provider_id_idx ON children (provider_id) WHERE deleted_at IS NULL;
CREATE INDEX children_active_idx      ON children (provider_id) WHERE deleted_at IS NULL AND withdrawal_date IS NULL;
-- No GIN under SQLite. If we need to look up children by guardian email,
-- we'll do it via json_each at read time or denormalize into a table.

-- ---------------------------------------------------------------------------
-- child_documents_required: the compliance checklist. Seeded per-child from
-- a state+age-specific template. status is the driver of the dashboard
-- compliance score.
--
-- ON DELETE CASCADE — if a child record is hard-deleted these rows go too.
-- ---------------------------------------------------------------------------
CREATE TABLE child_documents_required (
    id           TEXT PRIMARY KEY,
    child_id     TEXT NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    doc_type     TEXT NOT NULL,
    required_by  TEXT NULL,
    status       TEXT NOT NULL DEFAULT 'missing',
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT child_docs_status_chk CHECK (
        status IN ('missing','uploaded','expired','expiring_soon','compliant')
    ),
    CONSTRAINT child_docs_unique UNIQUE (child_id, doc_type)
);

CREATE TRIGGER child_documents_required_set_updated_at
AFTER UPDATE ON child_documents_required
FOR EACH ROW
WHEN NEW.updated_at IS OLD.updated_at
BEGIN
    UPDATE child_documents_required SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE INDEX child_docs_required_child_idx  ON child_documents_required (child_id);
CREATE INDEX child_docs_required_status_idx ON child_documents_required (status);
CREATE INDEX child_docs_required_chase_idx  ON child_documents_required (required_by)
    WHERE status IN ('missing','expiring_soon','expired');
