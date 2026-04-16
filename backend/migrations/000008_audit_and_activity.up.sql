-- 000008_audit_and_activity.up.sql
-- Immutable audit log. Written by every mutating handler.

BEGIN;

-- ---------------------------------------------------------------------------
-- audit_log: high-volume, append-only. We do NOT use a FK on provider_id
-- with CASCADE — instead ON DELETE SET NULL so that if/when we eventually
-- purge a churned provider's data after the grace period, we retain the
-- anonymized audit trail (legally required in several states for 3-5 yrs,
-- we keep 7 yrs as company policy).
--
-- actor_id is polymorphic (user_id, magic_link_token_id, webhook id, etc);
-- actor_kind disambiguates. No FK for the same reason.
--
-- Partitioning by month was considered and deferred: Postgres 16 native
-- partitioning adds operational complexity we don't need at MVP scale
-- (<10M rows/yr). Revisit when the table exceeds ~50GB.
-- ---------------------------------------------------------------------------
CREATE TABLE audit_log (
    id           TEXT PRIMARY KEY,
    provider_id  TEXT REFERENCES providers(id) ON DELETE SET NULL,
    actor_kind   TEXT NOT NULL,
    actor_id     TEXT,
    action       TEXT NOT NULL,
    target_kind  TEXT,
    target_id    TEXT,
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip           INET,
    user_agent   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT audit_log_actor_kind_chk CHECK (
        actor_kind IN ('system','provider_admin','staff','parent','webhook')
    )
);

-- Dashboard: "what happened at this provider lately?"
CREATE INDEX audit_log_provider_created_idx ON audit_log (provider_id, created_at DESC);
-- Ops: "when did we last successfully process stripe webhooks?"
CREATE INDEX audit_log_action_created_idx   ON audit_log (action, created_at DESC);
-- Investigation: "everything that happened to this child"
CREATE INDEX audit_log_target_idx           ON audit_log (target_kind, target_id)
    WHERE target_kind IS NOT NULL;

-- Belt-and-suspenders: revoke UPDATE/DELETE in application role at deploy
-- time (see infra/scripts/bootstrap-droplet.sh). The DB role the app uses
-- gets only INSERT and SELECT on this table.

COMMIT;
