-- Down Migration

ALTER TABLE cameras DROP COLUMN IF EXISTS main_stream_url;
ALTER TABLE cameras DROP COLUMN IF EXISTS sub_stream_url;
