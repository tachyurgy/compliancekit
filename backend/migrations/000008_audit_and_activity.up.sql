-- 000008_audit_and_activity.up.sql
-- Immutable audit log. Written by every mutating handler.
-- SQLite dialect (see ADR-017).

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
-- metadata is TEXT JSON, validated. ip is TEXT (was INET under PG).
--
-- Partitioning by month was considered and deferred. At SQLite scale the
-- write pattern is single-writer + WAL; we revisit if the file exceeds
-- ~10 GB or INSERT latency degrades.
-- ---------------------------------------------------------------------------
CREATE TABLE audit_log (
    id           TEXT PRIMARY KEY,
    provider_id  TEXT REFERENCES providers(id) ON DELETE SET NULL,
    actor_kind   TEXT NOT NULL,
    actor_id     TEXT,
    action       TEXT NOT NULL,
    target_kind  TEXT,
    target_id    TEXT,
    metadata     TEXT NOT NULL DEFAULT '{}',
    ip           TEXT,
    user_agent   TEXT,
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT audit_log_actor_kind_chk CHECK (
        actor_kind IN ('system','provider_admin','staff','parent','webhook')
    ),
    CONSTRAINT audit_log_metadata_chk CHECK (json_valid(metadata))
);

-- Dashboard: "what happened at this provider lately?"
CREATE INDEX audit_log_provider_created_idx ON audit_log (provider_id, created_at DESC);
-- Ops: "when did we last successfully process stripe webhooks?"
CREATE INDEX audit_log_action_created_idx   ON audit_log (action, created_at DESC);
-- Investigation: "everything that happened to this child"
CREATE INDEX audit_log_target_idx           ON audit_log (target_kind, target_id)
    WHERE target_kind IS NOT NULL;

-- Belt-and-suspenders mutation protection cannot be done with GRANT under
-- SQLite (no per-table roles). Writes to audit_log go through a helper in
-- `internal/audit` that only exposes Insert; no UPDATE/DELETE is exported.
