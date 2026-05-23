-- 000041_providers_is_demo.up.sql
-- REQ-038 (demo as second facility): mark the demo facility itself so the
-- workspace switcher + the demo banner can light up without scanning the
-- staff/children tables.
--
-- REQ-024 (000038) added is_demo=1 to staff/children/rooms/documents but
-- not to providers — the original design seeded the operator's REAL
-- provider with demo rows. REQ-038 turns the model on its head: a SECOND
-- provider row carries is_demo=1, and every row inside that provider is
-- automatically demo by inheritance. We keep the per-row is_demo=1 on
-- staff/children/rooms so the staff_status/score handlers can still cheap-
-- filter to "non-demo only" if a callsite wants to.

ALTER TABLE providers ADD COLUMN is_demo INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS providers_is_demo_idx ON providers (is_demo) WHERE is_demo = 1;
