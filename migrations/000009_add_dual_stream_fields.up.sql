-- Up Migration

ALTER TABLE cameras ADD COLUMN IF NOT EXISTS main_stream_url VARCHAR(255);
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS sub_stream_url VARCHAR(255);

-- Backward compatibility: default main_stream_url to url_rtsp
UPDATE cameras SET main_stream_url = url_rtsp WHERE main_stream_url IS NULL;
