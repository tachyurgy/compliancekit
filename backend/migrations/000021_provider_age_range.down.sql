-- 000021_provider_age_range (down)
--
-- SQLite does not support DROP COLUMN prior to 3.35; modernc's embedded
-- SQLite is new enough, so the drops below are supported. Order matches
-- the up migration in reverse.

ALTER TABLE providers DROP COLUMN max_age_months;
ALTER TABLE providers DROP COLUMN min_age_months;
