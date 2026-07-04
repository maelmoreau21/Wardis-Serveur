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

-- 4. Seed default cardholders
INSERT INTO cardholders (id, first_name, last_name, company, email, photo, access_group, schedule)
VALUES 
('c0000000-0000-0000-0000-000000000001', 'Jean', 'Dupont', 'Wardis Security', 'jean.dupont@wardis.local', 'jean_dupont', 'Tous les accès', '24h/24'),
('c0000000-0000-0000-0000-000000000002', 'Marie', 'Martin', 'IT Infrastructure', 'marie.martin@wardis.local', 'marie_martin', 'IT Staff', 'Heures de bureau 08h-18h')
ON CONFLICT (id) DO NOTHING;

-- 5. Create default badges for the cardholders
INSERT INTO badges (id, number, cardholder_id, status)
VALUES
('b0000000-0000-0000-0000-000000000001', 'BADGE123', 'c0000000-0000-0000-0000-000000000001', 'active'),
('b0000000-0000-0000-0000-000000000002', 'BADGE456', 'c0000000-0000-0000-0000-000000000002', 'active')
ON CONFLICT (number) DO UPDATE SET cardholder_id = EXCLUDED.cardholder_id, status = 'active';

-- 6. Grant access permissions for the seeded badges on all seeded doors
INSERT INTO access_permissions (badge_id, door_id, allowed)
VALUES
('b0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', true), -- Jean Dupont -> Main Entrance
('b0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000002', true), -- Jean Dupont -> Server Room
('b0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000001', true), -- Marie Martin -> Main Entrance
('b0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000002', true)  -- Marie Martin -> Server Room
ON CONFLICT (badge_id, door_id) DO NOTHING;
