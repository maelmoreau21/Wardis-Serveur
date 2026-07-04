-- Down Migration

DROP TABLE IF EXISTS entity_permissions;
DROP TABLE IF EXISTS user_roles;

DELETE FROM permissions WHERE name IN ('camera:live', 'camera:archive', 'door:unlock');
