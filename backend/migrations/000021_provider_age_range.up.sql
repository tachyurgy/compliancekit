-- 000021_provider_age_range
--
-- The onboarding wizard (StepFacility) captures the age range the facility
-- serves in months. The review-step POST /api/provider/onboarding payload
-- includes agesServedMonths.{minMonths,maxMonths}, but the providers table
-- never had columns to persist them. Add both here.
--
-- Defaults (0, 72) cover "birth through 6 years" — the broadest plausible
-- range. Existing rows (none have onboarding data yet at the time this
-- migration runs; all pre-onboarding) get those defaults.

ALTER TABLE providers ADD COLUMN min_age_months INTEGER NOT NULL DEFAULT 0;
ALTER TABLE providers ADD COLUMN max_age_months INTEGER NOT NULL DEFAULT 72;
