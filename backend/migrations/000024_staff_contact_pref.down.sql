DROP INDEX IF EXISTS idx_staff_nag_sends_provider;
DROP INDEX IF EXISTS idx_staff_nag_sends_staff;
DROP TABLE IF EXISTS staff_nag_sends;
ALTER TABLE staff DROP COLUMN contact_channel_set_at;
ALTER TABLE staff DROP COLUMN preferred_contact_channel;
