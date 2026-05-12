-- REQ-010: dedup markers for the trial-ending-1-day reminder and the
-- trial-converted email. Same idiom as 000033_trial_email_marker — set
-- the column to the send timestamp; NULL means "not yet sent."
--
-- Why two separate columns instead of an email_sends log table:
--   * We only have three trial-lifecycle emails today (3-day, 1-day,
--     converted). A table with a 3-row enum + idempotency index would
--     be heavier than the columns it replaces.
--   * If/when we add many more transactional emails per provider we'll
--     migrate to provider_email_sends with (kind, sent_at) — but not
--     for two columns.
ALTER TABLE providers ADD COLUMN trial_ending_1day_email_sent_at TEXT;
ALTER TABLE providers ADD COLUMN trial_converted_email_sent_at TEXT;
