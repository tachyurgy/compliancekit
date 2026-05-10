-- 000028_children_portal_ocr.down.sql
-- Reverse 000028. SQLite cannot DROP COLUMN before 3.35; we use the modern
-- syntax. Drop the auxiliary tables and the columns we added.

-- document_requirements is owned by migration 000026; we only seeded rows
-- here. Remove the rows we added; do NOT drop the table.
DELETE FROM document_requirements
 WHERE id IN (
   'req_fl_child_dh680', 'req_fl_child_dh3040', 'req_fl_child_5219',
   'req_fl_child_emerg', 'req_fl_child_pickup', 'req_fl_child_allergy',
   'req_fl_child_ack_kycf', 'req_fl_child_ack_flu', 'req_fl_child_ack_dist',
   'req_fl_child_ack_disc', 'req_fl_child_ack_food'
 );

DROP INDEX IF EXISTS upload_audit_token_idx;
DROP INDEX IF EXISTS upload_audit_document_idx;
DROP INDEX IF EXISTS upload_audit_created_idx;
DROP TABLE IF EXISTS upload_audit;

DROP INDEX IF EXISTS documents_confirmation_gate_idx;

ALTER TABLE documents DROP COLUMN does_not_expire;
ALTER TABLE documents DROP COLUMN expiration_confirmed_by_user_id;
ALTER TABLE documents DROP COLUMN expiration_confirmed_at;
ALTER TABLE documents DROP COLUMN extraction_confidence;
ALTER TABLE documents DROP COLUMN document_type_extracted_value;
ALTER TABLE documents DROP COLUMN expiration_extracted_value;

ALTER TABLE children DROP COLUMN withdrawn_at;
ALTER TABLE children DROP COLUMN medical_conditions;
ALTER TABLE children DROP COLUMN parent_name;
