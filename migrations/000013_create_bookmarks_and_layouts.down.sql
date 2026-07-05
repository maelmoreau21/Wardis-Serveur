-- Down Migration

DROP TABLE IF EXISTS saved_views;
DROP TABLE IF EXISTS bookmarks;

ALTER TABLE cameras DROP COLUMN IF EXISTS recording_mode;
ALTER TABLE cameras DROP COLUMN IF EXISTS retention_days;
