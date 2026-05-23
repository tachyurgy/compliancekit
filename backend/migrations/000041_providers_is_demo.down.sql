-- 000041_providers_is_demo.down.sql
DROP INDEX IF EXISTS providers_is_demo_idx;
ALTER TABLE providers DROP COLUMN is_demo;
