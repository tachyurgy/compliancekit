-- 000016_reconcile_staff_columns
--
-- handlers/staff.go + test fixtures reference a column named `hire_date`
-- that the original 000003 migration declared as `hired_on`. Add the
-- handler-era column and backfill from the canonical `hired_on` value.
-- Matches the pattern already used in migrations 000012 (providers) and
-- 000015 (children).

ALTER TABLE staff ADD COLUMN hire_date TEXT;
UPDATE staff SET hire_date = hired_on WHERE hire_date IS NULL;
