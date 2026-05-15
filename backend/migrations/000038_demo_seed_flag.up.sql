-- 000038_demo_seed_flag.up.sql
-- REQ-024 (Shayna tour): mark rows produced by seedDemoFacility so the
-- "Make this mine" button at the end of the onboarding tour can purge
-- them in a single DELETE per table.
--
-- We add the flag to every table the seed writes to. Existing rows
-- default to is_demo=0 (real). Indexed only on (provider_id) where
-- is_demo=1, so production reads pay zero cost for the new column.

ALTER TABLE staff      ADD COLUMN is_demo INTEGER NOT NULL DEFAULT 0;
ALTER TABLE children   ADD COLUMN is_demo INTEGER NOT NULL DEFAULT 0;
ALTER TABLE documents  ADD COLUMN is_demo INTEGER NOT NULL DEFAULT 0;
ALTER TABLE rooms      ADD COLUMN is_demo INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS staff_demo_idx     ON staff     (provider_id) WHERE is_demo = 1;
CREATE INDEX IF NOT EXISTS children_demo_idx  ON children  (provider_id) WHERE is_demo = 1;
CREATE INDEX IF NOT EXISTS documents_demo_idx ON documents (provider_id) WHERE is_demo = 1;
CREATE INDEX IF NOT EXISTS rooms_demo_idx     ON rooms     (provider_id) WHERE is_demo = 1;
