-- Up Migration

-- Create zones table
CREATE TABLE IF NOT EXISTS zones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nom VARCHAR(255) NOT NULL,
    description TEXT,
    statut VARCHAR(50) NOT NULL DEFAULT 'desarme', -- 'arme', 'desarme'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create capteurs (sensors) table
CREATE TABLE IF NOT EXISTS capteurs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone_id UUID REFERENCES zones(id) ON DELETE CASCADE,
    nom VARCHAR(255) NOT NULL,
    type VARCHAR(100) NOT NULL, -- e.g. 'mouvement', 'ouverture', etc.
    statut VARCHAR(50) NOT NULL DEFAULT 'ok', -- 'ok', 'declenche'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create alarmes table
CREATE TABLE IF NOT EXISTS alarmes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone_id UUID REFERENCES zones(id) ON DELETE CASCADE,
    capteur_id UUID REFERENCES capteurs(id) ON DELETE CASCADE,
    statut VARCHAR(50) NOT NULL DEFAULT 'active', -- 'active', 'acquittee', 'resolue'
    declenchee_a TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    acquittee_a TIMESTAMP WITH TIME ZONE,
    acquittee_par UUID REFERENCES users(id) ON DELETE SET NULL
);

-- Create historique_alarmes table
CREATE TABLE IF NOT EXISTS historique_alarmes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alarme_id UUID REFERENCES alarmes(id) ON DELETE SET NULL,
    zone_id UUID REFERENCES zones(id) ON DELETE SET NULL,
    capteur_id UUID REFERENCES capteurs(id) ON DELETE SET NULL,
    evenement VARCHAR(255) NOT NULL, -- e.g. 'zone_armee', 'zone_desarmee', 'capteur_declenche', 'alarme_declenchee', 'alarme_acquittee'
    details TEXT,
    cree_le TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Seed some default zones and sensors
INSERT INTO zones (id, nom, description, statut)
VALUES 
('e0000000-0000-0000-0000-000000000001', 'Zone Entrée', 'Zone couvrant l''entrée principale', 'desarme'),
('e0000000-0000-0000-0000-000000000002', 'Zone Bureaux', 'Zone couvrant les bureaux du RDC', 'desarme')
ON CONFLICT (id) DO NOTHING;

INSERT INTO capteurs (id, zone_id, nom, type, statut)
VALUES
('c0000000-0000-0000-0000-000000000001', 'e0000000-0000-0000-0000-000000000001', 'Capteur Porte Entrée', 'ouverture', 'ok'),
('c0000000-0000-0000-0000-000000000002', 'e0000000-0000-0000-0000-000000000001', 'Détecteur Mouvement Hall', 'mouvement', 'ok'),
('c0000000-0000-0000-0000-000000000003', 'e0000000-0000-0000-0000-000000000002', 'Détecteur Bureaux RDC', 'mouvement', 'ok')
ON CONFLICT (id) DO NOTHING;
