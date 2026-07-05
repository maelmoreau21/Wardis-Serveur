-- Down Migration

-- 1. Drop active_sessions table
DROP TABLE IF EXISTS active_sessions;

-- 2. Remove columns from users
ALTER TABLE users DROP COLUMN IF EXISTS mfa_enabled;
ALTER TABLE users DROP COLUMN IF EXISTS totp_secret;

-- 3. Revert check constraint on entity_permissions
ALTER TABLE entity_permissions DROP CONSTRAINT IF EXISTS chk_entity_type;
ALTER TABLE entity_permissions ADD CONSTRAINT chk_entity_type CHECK (entity_type IN ('camera', 'door'));
