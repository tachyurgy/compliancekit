-- 000043_outgoing_messages.up.sql
-- REQ-043: per-tenant log of every free-form message the operator sends out
-- through the in-app composer. Powers /api/messages/send and the StaffDetail
-- / ChildDetail "Messages" tab.
--
-- One row per (channel, recipient). If a single composer submission sends to
-- 2 recipients across email + sms, that's 4 rows. This shape gives us:
--   * per-channel status (sent / failed) without one row hiding the other,
--   * cheap per-recipient history reads (single index on recipient + time),
--   * a clean append-only audit trail that lines up with how SES + Twilio
--     track sends, so future delivery webhooks can update a row in place.
--
-- Distinct from chase_events (the chase engine's outbound log) and from
-- staff_nag_sends (the manual nag log). Free-form messages are not a chase
-- and not a templated nag — they're the "I just want to text the whole
-- staff that we're closed Friday" lever.

CREATE TABLE outgoing_messages (
  id              TEXT PRIMARY KEY,
  provider_id     TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  sender_user_id  TEXT REFERENCES users(id) ON DELETE SET NULL,
  recipient_kind  TEXT NOT NULL,
  recipient_id    TEXT NOT NULL,
  channel         TEXT NOT NULL,
  subject         TEXT,
  body            TEXT NOT NULL,
  status          TEXT NOT NULL,
  error           TEXT,
  sent_at         TEXT,
  created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT outgoing_messages_kind_chk    CHECK (recipient_kind IN ('staff','parent')),
  CONSTRAINT outgoing_messages_channel_chk CHECK (channel IN ('email','sms')),
  CONSTRAINT outgoing_messages_status_chk  CHECK (status IN ('queued','sent','failed'))
);

CREATE INDEX outgoing_messages_recipient_idx
  ON outgoing_messages (provider_id, recipient_kind, recipient_id, created_at DESC);

CREATE INDEX outgoing_messages_provider_idx
  ON outgoing_messages (provider_id, created_at DESC);
