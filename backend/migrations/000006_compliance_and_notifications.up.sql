-- 000006_compliance_and_notifications.up.sql
-- Materialized compliance snapshots (for dashboard + history chart),
-- the chase worker's outbound event log, and hard suppressions.

BEGIN;

-- ---------------------------------------------------------------------------
-- compliance_snapshots: periodic point-in-time compliance score. We recompute
-- on every document change AND on a nightly cron (catches time-based
-- transitions like certificates that aged into "expired" overnight).
--
-- payload is the full detail dump (per-child, per-staff, per-facility
-- findings) — cheap to store, expensive to recompute, so we keep them.
-- ---------------------------------------------------------------------------
CREATE TABLE compliance_snapshots (
    id              TEXT PRIMARY KEY,
    provider_id     TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    score           SMALLINT NOT NULL,
    violation_count INTEGER NOT NULL DEFAULT 0,
    critical_count  INTEGER NOT NULL DEFAULT 0,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    computed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT compliance_snapshots_score_chk CHECK (score BETWEEN 0 AND 100)
);

CREATE INDEX compliance_snapshots_provider_idx       ON compliance_snapshots (provider_id, computed_at DESC);

-- ---------------------------------------------------------------------------
-- chase_events: every time we fire a "your X is missing/expiring" to a
-- parent/staff/provider via email/sms/in-app. One row per send attempt so
-- we have reply-to/bounce audit for suppressions.
--
-- sent_at vs failed_at: exactly one is set on terminal state. Row is created
-- with both NULL, updated to one-or-the-other by the delivery worker.
-- ---------------------------------------------------------------------------
CREATE TABLE chase_events (
    id                TEXT PRIMARY KEY,
    provider_id       TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    target_kind       TEXT NOT NULL,
    target_id         TEXT NOT NULL,
    document_type     TEXT NOT NULL,
    trigger           TEXT NOT NULL,
    channel           TEXT NOT NULL,
    recipient_contact TEXT NOT NULL,
    sent_at           TIMESTAMPTZ NULL,
    failed_at         TIMESTAMPTZ NULL,
    failure_reason    TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chase_events_target_kind_chk CHECK (target_kind IN ('child','staff','facility')),
    CONSTRAINT chase_events_trigger_chk     CHECK (trigger IN ('6w','4w','2w','1w','3d','overdue')),
    CONSTRAINT chase_events_channel_chk     CHECK (channel IN ('email','sms','inapp')),
    CONSTRAINT chase_events_terminal_chk    CHECK (
        NOT (sent_at IS NOT NULL AND failed_at IS NOT NULL)
    )
);

CREATE INDEX chase_events_provider_idx ON chase_events (provider_id, created_at DESC);
CREATE INDEX chase_events_target_idx   ON chase_events (target_kind, target_id, document_type);
-- Dedupe: don't fire the same (target, doc_type, trigger, channel) twice.
CREATE UNIQUE INDEX chase_events_dedupe_uidx ON chase_events
    (provider_id, target_kind, target_id, document_type, trigger, channel)
    WHERE sent_at IS NOT NULL;

-- ---------------------------------------------------------------------------
-- notification_suppressions: append-only list of addresses we must never
-- message again (SES complaint, Twilio STOP, explicit unsubscribe).
-- Checked at send time by the chase worker.
-- ---------------------------------------------------------------------------
CREATE TABLE notification_suppressions (
    email_or_phone TEXT PRIMARY KEY,
    reason         TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notif_supp_reason_chk CHECK (reason IN ('unsubscribed','hard_bounce','complaint'))
);

COMMIT;
