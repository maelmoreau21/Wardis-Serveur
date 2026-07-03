-- Down Migration

DROP TABLE IF EXISTS events_log;

ALTER TABLE cameras DROP COLUMN IF EXISTS zone_id;
ALTER TABLE doors DROP COLUMN IF EXISTS zone_id;
