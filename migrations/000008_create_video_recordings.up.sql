-- Up Migration

CREATE TABLE IF NOT EXISTS video_recordings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    filepath VARCHAR(512) NOT NULL,
    storage_type VARCHAR(50) NOT NULL DEFAULT 'local', -- 'local' or 'cloud'
    file_size BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for searching recordings by camera and time overlap
CREATE INDEX IF NOT EXISTS idx_video_recordings_camera_time ON video_recordings(camera_id, start_time, end_time);
