-- Up Migration

-- 1. Create cardholders table
CREATE TABLE IF NOT EXISTS cardholders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    company VARCHAR(255),
    email VARCHAR(255),
    photo TEXT, -- Base64 encoded or predefined path
    access_group VARCHAR(255) DEFAULT 'Standard',
    schedule VARCHAR(255) DEFAULT '24h/24',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 2. Alter badges table to link to cardholders
ALTER TABLE badges ADD COLUMN IF NOT EXISTS cardholder_id UUID REFERENCES cardholders(id) ON DELETE SET NULL;

-- 3. Alter access_logs table to link to cardholders
ALTER TABLE access_logs ADD COLUMN IF NOT EXISTS cardholder_id UUID REFERENCES cardholders(id) ON DELETE SET NULL;

-- No demo data: cardholders, badges and access permissions are created via the admin UI or API.
