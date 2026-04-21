-- 000015_reconcile_children_columns
--
-- handlers/children.go writes to columns (enroll_date, parent_email,
-- parent_phone, classroom) that the original 000002 migration never
-- declared. Same situation migration 000012 addressed for the providers
-- table. Add the columns here so runtime SQL succeeds.
--
-- The canonical 000002 columns remain:
--   enrollment_date — the official state-form field, JSON guardians hold
--                     family contact info.
-- The handler-era columns are denormalizations for fast access; writes
-- populate both where possible.

ALTER TABLE children ADD COLUMN enroll_date TEXT;
UPDATE children SET enroll_date = enrollment_date WHERE enroll_date IS NULL;

ALTER TABLE children ADD COLUMN parent_email TEXT;
ALTER TABLE children ADD COLUMN parent_phone TEXT;
ALTER TABLE children ADD COLUMN classroom TEXT;
