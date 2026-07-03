package intrusion

import (
	"time"
)

type Zone struct {
	ID          string    `json:"id"`
	Nom         string    `json:"nom"`
	Description *string   `json:"description,omitempty"`
	Statut      string    `json:"statut"` // "arme", "desarme"
	CreatedAt   time.Time `json:"created_at"`
}

type Capteur struct {
	ID        string    `json:"id"`
	ZoneID    string    `json:"zone_id"`
	Nom       string    `json:"nom"`
	Type      string    `json:"type"` // e.g. "ouverture", "mouvement"
	Statut    string    `json:"statut"` // "ok", "declenche"
	CreatedAt time.Time `json:"created_at"`
}

type Alarme struct {
	ID           string     `json:"id"`
	ZoneID       string     `json:"zone_id"`
	CapteurID    string     `json:"capteur_id"`
	Statut       string     `json:"statut"` // "active", "acquittee"
	DeclencheeA  time.Time  `json:"declenchee_a"`
	AcquitteeA   *time.Time `json:"acquittee_a,omitempty"`
	AcquitteePar *string    `json:"acquittee_par,omitempty"`
}

type HistoriqueAlarme struct {
	ID        string    `json:"id"`
	AlarmeID  *string   `json:"alarme_id,omitempty"`
	ZoneID    *string   `json:"zone_id,omitempty"`
	CapteurID *string   `json:"capteur_id,omitempty"`
	Evenement string    `json:"evenement"` // "zone_armee", "zone_desarmee", "capteur_declenche", "alarme_declenchee"
	Details   *string   `json:"details,omitempty"`
	CreeLe    time.Time `json:"cree_le"`
}
