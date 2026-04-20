-- 000001_init_providers_and_users.down.sql
-- SQLite drops triggers automatically when their parent tables are dropped.

DROP TABLE IF EXISTS magic_link_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS providers;
