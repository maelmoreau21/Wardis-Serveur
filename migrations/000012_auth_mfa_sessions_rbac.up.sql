-- Up Migration

-- 1. Add MFA columns to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_enabled BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret VARCHAR(100);

-- 2. Create active_sessions table
CREATE TABLE IF NOT EXISTS active_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    ip_address VARCHAR(45),
    user_agent TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- 3. Update check constraint on entity_permissions to allow 'site'
ALTER TABLE entity_permissions DROP CONSTRAINT IF EXISTS chk_entity_type;
ALTER TABLE entity_permissions ADD CONSTRAINT chk_entity_type CHECK (entity_type IN ('camera', 'door', 'site'));

-- 4. Seed base roles
INSERT INTO roles (name, description) VALUES
('admin', 'Administrator role with full access'),
('supervisor', 'Supervisor role with most control permissions'),
('operator', 'Operator role with standard live view and alarm capabilities'),
('guest', 'Guest role with read-only access')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

-- 5. Seed granular permissions
INSERT INTO permissions (name, description) VALUES
('camera:live', 'Access live video stream of a camera'),
('camera:archive', 'Access archive/recorded video of a camera'),
('door:unlock', 'Unlock access control doors'),
('door:control', 'Open and close doors from control panel'),
('zone:arm', 'Arm intrusion zones'),
('zone:disarm', 'Disarm intrusion zones'),
('alarm:acquit', 'Acknowledge intrusion alarms'),
('audit:view', 'View system audit logs'),
('export:video', 'Export recorded video files'),
('session:manage', 'List and revoke active operator sessions'),
('site:view', 'Access entities belonging to a specific site')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

-- 6. Link permissions to roles
-- Clean existing associations first to avoid duplicate errors and align perfectly
DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE name IN ('admin', 'supervisor', 'operator', 'guest'));

-- Admin role links
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'admin' AND p.name IN (
    'admin.all', 'auth.me', 'camera:live', 'camera:archive', 'door:unlock', 'door:control',
    'zone:arm', 'zone:disarm', 'alarm:acquit', 'audit:view', 'export:video', 'session:manage', 'site:view'
) ON CONFLICT DO NOTHING;

-- Supervisor role links
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'supervisor' AND p.name IN (
    'auth.me', 'camera:live', 'camera:archive', 'door:unlock', 'door:control',
    'zone:arm', 'zone:disarm', 'alarm:acquit', 'audit:view', 'export:video'
) ON CONFLICT DO NOTHING;

-- Operator role links
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'operator' AND p.name IN (
    'auth.me', 'camera:live', 'camera:archive', 'door:unlock',
    'zone:arm', 'zone:disarm', 'alarm:acquit'
) ON CONFLICT DO NOTHING;

-- Guest role links
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'guest' AND p.name IN (
    'auth.me', 'camera:live'
) ON CONFLICT DO NOTHING;
