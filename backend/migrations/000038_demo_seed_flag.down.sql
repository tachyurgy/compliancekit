-- Reverse 000038_demo_seed_flag.up.sql. SQLite doesn't support DROP COLUMN
-- pre-3.35; the production droplet ships SQLite 3.46+, so we can.
DROP INDEX IF EXISTS staff_demo_idx;
DROP INDEX IF EXISTS children_demo_idx;
DROP INDEX IF EXISTS documents_demo_idx;
DROP INDEX IF EXISTS rooms_demo_idx;

ALTER TABLE staff     DROP COLUMN is_demo;
ALTER TABLE children  DROP COLUMN is_demo;
ALTER TABLE documents DROP COLUMN is_demo;
ALTER TABLE rooms     DROP COLUMN is_demo;
