-- Down-migration for 000029_inspector_mode.

DROP TRIGGER IF EXISTS posting_photos_set_updated_at;
DROP INDEX  IF EXISTS posting_photos_facility_verified_idx;
DROP INDEX  IF EXISTS posting_photos_facility_kind_idx;
DROP TABLE  IF EXISTS posting_photos;

DROP TRIGGER IF EXISTS inspection_events_set_updated_at;
DROP INDEX  IF EXISTS inspection_events_facility_started_idx;
DROP TABLE  IF EXISTS inspection_events;
