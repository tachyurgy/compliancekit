-- Migration 000025: cadence v2 (90/60/30 then weekly) + per-child contact pref.
--
-- Children mirror the staff column added in 000024 so the chase service can
-- honor parent preferences too. Default is 'email' so existing rows keep
-- working without manual backfill.
ALTER TABLE children ADD COLUMN preferred_contact_channel TEXT NOT NULL DEFAULT 'email'
  CHECK (preferred_contact_channel IN ('email','sms','both'));
ALTER TABLE children ADD COLUMN contact_channel_set_at TEXT;

-- Note: document_chase_sends.threshold_days stays INTEGER. The new cadence
-- encodes weekly buckets as negative integers (-1 = first weekly fire after
-- the 30-day reminder, -2 = next week, ...). The existing PRIMARY KEY
-- (document_id, threshold_days, channel) keeps deduping correctly.
