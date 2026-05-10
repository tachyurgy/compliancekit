-- 000026b down: clear the FL seed rows. Safe because all rows are
-- referenced by deterministic IDs.
DELETE FROM document_requirements WHERE state_code = 'FL';
