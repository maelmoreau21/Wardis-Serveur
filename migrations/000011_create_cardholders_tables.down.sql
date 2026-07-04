-- Down Migration

-- 1. Remove seeded access permissions
DELETE FROM access_permissions WHERE badge_id IN ('b0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000002');

-- 2. Remove seeded badges
DELETE FROM badges WHERE id IN ('b0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000002');

-- 3. Remove seeded cardholders
DELETE FROM cardholders WHERE id IN ('c0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000002');

-- 4. Drop columns added to access_logs and badges
ALTER TABLE access_logs DROP COLUMN IF EXISTS cardholder_id;
ALTER TABLE badges DROP COLUMN IF EXISTS cardholder_id;

-- 5. Drop cardholders table
DROP TABLE IF EXISTS cardholders;
