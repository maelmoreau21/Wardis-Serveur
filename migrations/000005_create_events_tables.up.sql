-- Up Migration

-- 1. Add zone_id columns to link doors and cameras to physical intrusion zones
ALTER TABLE doors ADD COLUMN zone_id UUID REFERENCES zones(id) ON DELETE SET NULL;
ALTER TABLE cameras ADD COLUMN zone_id UUID REFERENCES zones(id) ON DELETE SET NULL;

-- 2. Link the default "Main Entrance" door to the default "Zone Entrée"
UPDATE doors 
SET zone_id = 'e0000000-0000-0000-0000-000000000001' 
WHERE id = 'd0000000-0000-0000-0000-000000000001';

-- 3. Seed a default camera watching the Main Entrance area
INSERT INTO cameras (id, nom, url_rtsp, site_id, zone_id, statut)
VALUES ('ca000000-0000-0000-0000-000000000001', 'Caméra Entrée', 'rtsp://localhost:8554/ca000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'e0000000-0000-0000-0000-000000000001', 'active')
ON CONFLICT (id) DO NOTHING;

-- 4. Create the events_log table to store correlated events
CREATE TABLE IF NOT EXISTS events_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL, -- 'alarm.triggered', 'access.granted', 'access.denied', etc.
    source_id UUID NOT NULL,
    zone_id UUID REFERENCES zones(id) ON DELETE SET NULL,
    capteur_id UUID REFERENCES capteurs(id) ON DELETE SET NULL,
    door_id UUID REFERENCES doors(id) ON DELETE SET NULL,
    badge_number VARCHAR(100),
    camera_id UUID REFERENCES cameras(id) ON DELETE SET NULL,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    details JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
