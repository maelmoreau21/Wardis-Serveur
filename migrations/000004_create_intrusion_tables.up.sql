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

-- No demo data: zones and sensors are created via the admin UI or API.
