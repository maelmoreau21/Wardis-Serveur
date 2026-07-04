-- Down Migration

ALTER TABLE cameras DROP COLUMN IF EXISTS ip;
ALTER TABLE cameras DROP COLUMN IF EXISTS port;
ALTER TABLE cameras DROP COLUMN IF EXISTS username;
ALTER TABLE cameras DROP COLUMN IF EXISTS password_encrypted;
ALTER TABLE cameras DROP COLUMN IF EXISTS ptz_supported;
ALTER TABLE cameras DROP COLUMN IF EXISTS profile_token;
