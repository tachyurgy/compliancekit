-- 000002_children.up.sql
-- Children enrolled at a provider + the document checklist rows we chase on.

BEGIN;

-- ---------------------------------------------------------------------------
-- children: one row per enrolled child. Soft-deletable so we preserve history
-- for audits (a state inspector may ask about a child who left months ago).
--
-- guardians is JSONB because the shape is flexible (1-4 entries, mixed
-- custody arrangements, variable contact fields). It is NOT a source of truth
-- for relational queries — if we need to send to all guardians across all
-- children we derive a flat table or use jsonb_array_elements.
-- ---------------------------------------------------------------------------
CREATE TABLE children (
    id                TEXT PRIMARY KEY,
    provider_id       TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    first_name        TEXT NOT NULL,
    last_name         TEXT NOT NULL,
    date_of_birth     DATE NOT NULL,
    enrollment_date   DATE NOT NULL,
    withdrawal_date   DATE NULL,
    guardians         JSONB NOT NULL DEFAULT '[]'::jsonb,
    allergies         TEXT,
    medical_notes     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ NULL,
    CONSTRAINT children_guardians_is_array CHECK (jsonb_typeof(guardians) = 'array')
);

CREATE TRIGGER children_set_updated_at
BEFORE UPDATE ON children
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX children_provider_id_idx ON children (provider_id) WHERE deleted_at IS NULL;
CREATE INDEX children_active_idx      ON children (provider_id) WHERE deleted_at IS NULL AND withdrawal_date IS NULL;
-- GIN on guardians so we can find all children for a given guardian email.
CREATE INDEX children_guardians_gin   ON children USING GIN (guardians jsonb_path_ops);

-- ---------------------------------------------------------------------------
-- child_documents_required: the compliance checklist. Seeded per-child from a
-- state+age-specific template (state rule: "FL requires X for every child
-- under 2" etc.). status is the driver of the dashboard compliance score.
--
-- ON DELETE CASCADE — if a child record is hard-deleted these rows go too.
-- ---------------------------------------------------------------------------
CREATE TABLE child_documents_required (
    id           TEXT PRIMARY KEY,
    child_id     TEXT NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    doc_type     TEXT NOT NULL,
    required_by  DATE NULL,
    status       TEXT NOT NULL DEFAULT 'missing',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT child_docs_status_chk CHECK (
        status IN ('missing','uploaded','expired','expiring_soon','compliant')
    ),
    CONSTRAINT child_docs_unique UNIQUE (child_id, doc_type)
);

CREATE TRIGGER child_documents_required_set_updated_at
BEFORE UPDATE ON child_documents_required
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX child_docs_required_child_idx  ON child_documents_required (child_id);
CREATE INDEX child_docs_required_status_idx ON child_documents_required (status);
-- Partial index for the chase worker: find everything missing or expiring soon.
CREATE INDEX child_docs_required_chase_idx  ON child_documents_required (required_by)
    WHERE status IN ('missing','expiring_soon','expired');

COMMIT;
