-- Up Migration

-- Create roles table if not exists
CREATE TABLE IF NOT EXISTS roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create permissions table if not exists
CREATE TABLE IF NOT EXISTS permissions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create role_permissions link table if not exists
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id INT REFERENCES roles(id) ON DELETE CASCADE,
    permission_id INT REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- Create user_roles link table
CREATE TABLE IF NOT EXISTS user_roles (
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    role_id INT REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

-- Create entity_permissions table
CREATE TABLE IF NOT EXISTS entity_permissions (
    id SERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    role_id INT REFERENCES roles(id) ON DELETE CASCADE,
    entity_type VARCHAR(50) NOT NULL,
    entity_id UUID NOT NULL,
    permission_id INT REFERENCES permissions(id) ON DELETE CASCADE,
    allowed BOOLEAN NOT NULL,
    CONSTRAINT chk_entity_type CHECK (entity_type IN ('camera', 'door')),
    CONSTRAINT chk_user_or_role CHECK (
        (user_id IS NOT NULL AND role_id IS NULL) OR
        (user_id IS NULL AND role_id IS NOT NULL)
    ),
    CONSTRAINT uq_entity_permission_user UNIQUE (user_id, entity_type, entity_id, permission_id),
    CONSTRAINT uq_entity_permission_role UNIQUE (role_id, entity_type, entity_id, permission_id)
);

-- Seed new advanced permissions
INSERT INTO permissions (name, description) VALUES
('camera:live', 'Access live video stream of a camera'),
('camera:archive', 'Access archive/recorded video of a camera'),
('door:unlock', 'Unlock access control doors')
ON CONFLICT (name) DO NOTHING;

-- Populate user_roles with existing user roles
INSERT INTO user_roles (user_id, role_id)
SELECT id, role_id FROM users
WHERE role_id IS NOT NULL
ON CONFLICT DO NOTHING;
