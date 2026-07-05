-- Up Migration

-- 1. Add recording settings to cameras table
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS recording_mode VARCHAR(50) DEFAULT 'none';
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS retention_days INTEGER DEFAULT 30;

-- 2. Create bookmarks table
CREATE TABLE IF NOT EXISTS bookmarks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    notes TEXT,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for quick bookmark retrieval
CREATE INDEX IF NOT EXISTS idx_bookmarks_camera_time ON bookmarks(camera_id, timestamp);

-- 3. Create saved_views table
CREATE TABLE IF NOT EXISTS saved_views (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    grid_layout VARCHAR(100) NOT NULL DEFAULT '2x2',
    slots JSONB NOT NULL DEFAULT '[]',
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for searching layouts by user
CREATE INDEX IF NOT EXISTS idx_saved_views_user ON saved_views(user_id);
