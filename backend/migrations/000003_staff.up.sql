-- 000003_staff.up.sql
-- Staff members employed by a provider + their required certifications.

BEGIN;

-- ---------------------------------------------------------------------------
-- staff: soft-deletable. email is NOT unique across providers (same person
-- could, in theory, work at two centers) nor UNIQUE within a provider because
-- some small centers reuse a shared inbox per staff — enforce uniqueness in
-- application logic instead.
-- ---------------------------------------------------------------------------
CREATE TABLE staff (
    id          TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    first_name  TEXT NOT NULL,
    last_name   TEXT NOT NULL,
    email       CITEXT,
    phone       TEXT,
    hired_on    DATE,
    role        TEXT,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ NULL,
    CONSTRAINT staff_status_chk CHECK (status IN ('active','inactive','terminated'))
);

CREATE TRIGGER staff_set_updated_at
BEFORE UPDATE ON staff
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX staff_provider_id_idx ON staff (provider_id) WHERE deleted_at IS NULL;
CREATE INDEX staff_active_idx      ON staff (provider_id) WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX staff_email_idx       ON staff (email) WHERE email IS NOT NULL;

-- ---------------------------------------------------------------------------
-- staff_certifications_required: certificate/background/training checklist.
-- Same status vocabulary as child_documents_required for UI symmetry.
-- ---------------------------------------------------------------------------
CREATE TABLE staff_certifications_required (
    id           TEXT PRIMARY KEY,
    staff_id     TEXT NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
    cert_type    TEXT NOT NULL,
    required_by  DATE NULL,
    status       TEXT NOT NULL DEFAULT 'missing',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT staff_certs_status_chk CHECK (
        status IN ('missing','uploaded','expired','expiring_soon','compliant')
    ),
    CONSTRAINT staff_certs_unique UNIQUE (staff_id, cert_type)
);

CREATE TRIGGER staff_certifications_required_set_updated_at
BEFORE UPDATE ON staff_certifications_required
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX staff_certs_staff_idx  ON staff_certifications_required (staff_id);
CREATE INDEX staff_certs_status_idx ON staff_certifications_required (status);
CREATE INDEX staff_certs_chase_idx  ON staff_certifications_required (required_by)
    WHERE status IN ('missing','expiring_soon','expired');

COMMIT;
