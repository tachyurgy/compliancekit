-- REQ-007: opt-out registry honored by the chase engine.
--
-- Two hooks populate this:
--   1) Twilio inbound SMS webhook: STOP/UNSUBSCRIBE/END/QUIT/CANCEL keyword
--      → insert (channel='sms', recipient=From, reason='twilio_stop').
--   2) AWS SES SNS bounce webhook: Permanent bounce → insert
--      (channel='email', recipient=BouncedEmail, reason='ses_hard_bounce').
--
-- Chase service reads this at send time and skips the matching channel,
-- but a recipient who opted out of email still receives SMS (and vice versa).
-- TCPA-relevant for the SMS side; CAN-SPAM/SES policy for email.
CREATE TABLE IF NOT EXISTS unsubscribes (
  id          TEXT PRIMARY KEY,
  channel     TEXT NOT NULL CHECK (channel IN ('email','sms')),
  recipient   TEXT NOT NULL,
  reason      TEXT NOT NULL,
  created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (channel, recipient)
);
CREATE INDEX IF NOT EXISTS idx_unsubscribes_recipient ON unsubscribes (recipient);
