-- 000004_documents.down.sql

BEGIN;

DROP TABLE IF EXISTS document_unassigned_photos;
DROP TABLE IF EXISTS document_ocr_results;
DROP TABLE IF EXISTS documents;

COMMIT;
