-- REQ-022: dedup marker for the trial-ending-3-days email worker.
-- Set to the timestamp of a successful send. NULL means "not yet sent."
-- Cleared if a provider re-enters trial (rare; not v1).
ALTER TABLE providers ADD COLUMN trial_ending_email_sent_at TEXT;
