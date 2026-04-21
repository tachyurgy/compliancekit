-- 000013_retention_columns.down.sql
-- Reverse of 000013. Drops the indexes; leaves the column in place because
-- SQLite's ALTER TABLE DROP COLUMN is only available in 3.35+ and even there
-- some modernc.org/sqlite builds disable it. Dropping the column would also
-- discard real churn data that ops teams may care about.

DROP INDEX IF EXISTS providers_canceled_at_idx;
DROP INDEX IF EXISTS providers_deleted_at_purge_idx;
