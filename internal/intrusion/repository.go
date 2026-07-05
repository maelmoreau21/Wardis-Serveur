package intrusion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrZoneNotFound   = errors.New("zone not found")
	ErrSensorNotFound = errors.New("sensor not found")
	ErrAlarmNotFound  = errors.New("alarm not found")
)

type Repository interface {
	ListZones(ctx context.Context) ([]Zone, error)
	GetZoneByID(ctx context.Context, id string) (*Zone, error)
	CreateZone(ctx context.Context, z *Zone) (*Zone, error)
	UpdateZone(ctx context.Context, id string, z *Zone) (*Zone, error)
	DeleteZone(ctx context.Context, id string) error
	UpdateZoneStatus(ctx context.Context, id string, statut string) error
	ListSensors(ctx context.Context) ([]Capteur, error)
	GetSensorByID(ctx context.Context, id string) (*Capteur, error)
	CreateSensor(ctx context.Context, c *Capteur) (*Capteur, error)
	UpdateSensor(ctx context.Context, id string, c *Capteur) (*Capteur, error)
	DeleteSensor(ctx context.Context, id string) error
	UpdateSensorStatus(ctx context.Context, id string, statut string) error
	CreateAlarm(ctx context.Context, alarm *Alarme) (*Alarme, error)
	ListActiveAlarms(ctx context.Context) ([]Alarme, error)
	ListAllAlarms(ctx context.Context) ([]Alarme, error)
	CreateHistoryLog(ctx context.Context, log *HistoriqueAlarme) (*HistoriqueAlarme, error)
	GetAlarmByID(ctx context.Context, id string) (*Alarme, error)
	UpdateAlarmStatus(ctx context.Context, id string, statut string, userID *string) error
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) ListZones(ctx context.Context) ([]Zone, error) {
	query := `SELECT id, nom, description, statut, created_at FROM zones ORDER BY nom ASC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query zones: %w", err)
	}
	defer rows.Close()

	var zones []Zone
	for rows.Next() {
		var z Zone
		var desc sql.NullString
		if err := rows.Scan(&z.ID, &z.Nom, &desc, &z.Statut, &z.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan zone row: %w", err)
		}
		if desc.Valid {
			z.Description = &desc.String
		}
		zones = append(zones, z)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during zones iteration: %w", err)
	}

	return zones, nil
}

func (r *postgresRepository) GetZoneByID(ctx context.Context, id string) (*Zone, error) {
	query := `SELECT id, nom, description, statut, created_at FROM zones WHERE id = $1`
	var z Zone
	var desc sql.NullString
	err := r.db.QueryRow(ctx, query, id).Scan(&z.ID, &z.Nom, &desc, &z.Statut, &z.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrZoneNotFound
		}
		return nil, fmt.Errorf("failed to get zone by id: %w", err)
	}
	if desc.Valid {
		z.Description = &desc.String
	}
	return &z, nil
}

func (r *postgresRepository) UpdateZoneStatus(ctx context.Context, id string, statut string) error {
	query := `UPDATE zones SET statut = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, query, statut, id)
	if err != nil {
		return fmt.Errorf("failed to update zone status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrZoneNotFound
	}
	return nil
}

func (r *postgresRepository) ListSensors(ctx context.Context) ([]Capteur, error) {
	query := `SELECT id, zone_id, nom, type, statut, created_at FROM capteurs ORDER BY nom ASC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query sensors: %w", err)
	}
	defer rows.Close()

	var sensors []Capteur
	for rows.Next() {
		var c Capteur
		if err := rows.Scan(&c.ID, &c.ZoneID, &c.Nom, &c.Type, &c.Statut, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan sensor row: %w", err)
		}
		sensors = append(sensors, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during sensors iteration: %w", err)
	}

	return sensors, nil
}

func (r *postgresRepository) GetSensorByID(ctx context.Context, id string) (*Capteur, error) {
	query := `SELECT id, zone_id, nom, type, statut, created_at FROM capteurs WHERE id = $1`
	var c Capteur
	err := r.db.QueryRow(ctx, query, id).Scan(&c.ID, &c.ZoneID, &c.Nom, &c.Type, &c.Statut, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSensorNotFound
		}
		return nil, fmt.Errorf("failed to get sensor by id: %w", err)
	}
	return &c, nil
}

func (r *postgresRepository) UpdateSensorStatus(ctx context.Context, id string, statut string) error {
	query := `UPDATE capteurs SET statut = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, query, statut, id)
	if err != nil {
		return fmt.Errorf("failed to update sensor status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSensorNotFound
	}
	return nil
}

func (r *postgresRepository) CreateAlarm(ctx context.Context, alarm *Alarme) (*Alarme, error) {
	query := `
		INSERT INTO alarmes (zone_id, capteur_id, statut)
		VALUES ($1, $2, $3)
		RETURNING id, zone_id, capteur_id, statut, declenchee_a, acquittee_a, acquittee_par`
	
	var res Alarme
	var acqA sql.NullTime
	var acqPar sql.NullString

	err := r.db.QueryRow(ctx, query, alarm.ZoneID, alarm.CapteurID, alarm.Statut).Scan(
		&res.ID, &res.ZoneID, &res.CapteurID, &res.Statut, &res.DeclencheeA, &acqA, &acqPar,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create alarm: %w", err)
	}

	if acqA.Valid {
		res.AcquitteeA = &acqA.Time
	}
	if acqPar.Valid {
		res.AcquitteePar = &acqPar.String
	}

	return &res, nil
}

func (r *postgresRepository) ListActiveAlarms(ctx context.Context) ([]Alarme, error) {
	query := `
		SELECT id, zone_id, capteur_id, statut, declenchee_a, acquittee_a, acquittee_par 
		FROM alarmes 
		WHERE statut = 'active'
		ORDER BY declenchee_a DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active alarms: %w", err)
	}
	defer rows.Close()

	var alarms []Alarme
	for rows.Next() {
		var al Alarme
		var acqA sql.NullTime
		var acqPar sql.NullString

		if err := rows.Scan(&al.ID, &al.ZoneID, &al.CapteurID, &al.Statut, &al.DeclencheeA, &acqA, &acqPar); err != nil {
			return nil, fmt.Errorf("failed to scan alarm row: %w", err)
		}

		if acqA.Valid {
			al.AcquitteeA = &acqA.Time
		}
		if acqPar.Valid {
			al.AcquitteePar = &acqPar.String
		}

		alarms = append(alarms, al)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during active alarms iteration: %w", err)
	}

	return alarms, nil
}

func (r *postgresRepository) CreateHistoryLog(ctx context.Context, log *HistoriqueAlarme) (*HistoriqueAlarme, error) {
	query := `
		INSERT INTO historique_alarmes (alarme_id, zone_id, capteur_id, evenement, details)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, alarme_id, zone_id, capteur_id, evenement, details, cree_le`
	
	var res HistoriqueAlarme
	var alID, zID, cID sql.NullString
	var det sql.NullString

	err := r.db.QueryRow(ctx, query, log.AlarmeID, log.ZoneID, log.CapteurID, log.Evenement, log.Details).Scan(
		&res.ID, &alID, &zID, &cID, &res.Evenement, &det, &res.CreeLe,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create history log: %w", err)
	}

	if alID.Valid {
		res.AlarmeID = &alID.String
	}
	if zID.Valid {
		res.ZoneID = &zID.String
	}
	if cID.Valid {
		res.CapteurID = &cID.String
	}
	if det.Valid {
		res.Details = &det.String
	}

	return &res, nil
}

func (r *postgresRepository) GetAlarmByID(ctx context.Context, id string) (*Alarme, error) {
	query := `
		SELECT id, zone_id, capteur_id, statut, declenchee_a, acquittee_a, acquittee_par 
		FROM alarmes 
		WHERE id = $1`
	
	var al Alarme
	var acqA sql.NullTime
	var acqPar sql.NullString

	err := r.db.QueryRow(ctx, query, id).Scan(&al.ID, &al.ZoneID, &al.CapteurID, &al.Statut, &al.DeclencheeA, &acqA, &acqPar)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlarmNotFound
		}
		return nil, fmt.Errorf("failed to get alarm by id: %w", err)
	}

	if acqA.Valid {
		al.AcquitteeA = &acqA.Time
	}
	if acqPar.Valid {
		al.AcquitteePar = &acqPar.String
	}

	return &al, nil
}

func (r *postgresRepository) UpdateAlarmStatus(ctx context.Context, id string, statut string, userID *string) error {
	var query string
	var err error
	if userID != nil {
		query = `UPDATE alarmes SET statut = $1, acquittee_a = CURRENT_TIMESTAMP, acquittee_par = $2 WHERE id = $3`
		_, err = r.db.Exec(ctx, query, statut, *userID, id)
	} else {
		query = `UPDATE alarmes SET statut = $1 WHERE id = $2`
		_, err = r.db.Exec(ctx, query, statut, id)
	}

	if err != nil {
		return fmt.Errorf("failed to update alarm status: %w", err)
	}
	return nil
}

func (r *postgresRepository) CreateZone(ctx context.Context, z *Zone) (*Zone, error) {
	query := `
		INSERT INTO zones (nom, description, statut)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	var desc sql.NullString
	if z.Description != nil {
		desc.String = *z.Description
		desc.Valid = true
	}
	if z.Statut == "" {
		z.Statut = "desarme"
	}
	err := r.db.QueryRow(ctx, query, z.Nom, desc, z.Statut).Scan(&z.ID, &z.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create zone: %w", err)
	}
	return z, nil
}

func (r *postgresRepository) UpdateZone(ctx context.Context, id string, z *Zone) (*Zone, error) {
	query := `
		UPDATE zones
		SET nom = $1, description = $2, statut = $3
		WHERE id = $4`
	var desc sql.NullString
	if z.Description != nil {
		desc.String = *z.Description
		desc.Valid = true
	}
	tag, err := r.db.Exec(ctx, query, z.Nom, desc, z.Statut, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update zone: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrZoneNotFound
	}
	z.ID = id
	return z, nil
}

func (r *postgresRepository) DeleteZone(ctx context.Context, id string) error {
	query := `DELETE FROM zones WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete zone: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrZoneNotFound
	}
	return nil
}

func (r *postgresRepository) CreateSensor(ctx context.Context, c *Capteur) (*Capteur, error) {
	query := `
		INSERT INTO capteurs (zone_id, nom, type, statut)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`
	if c.Statut == "" {
		c.Statut = "ok"
	}
	err := r.db.QueryRow(ctx, query, c.ZoneID, c.Nom, c.Type, c.Statut).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create sensor: %w", err)
	}
	return c, nil
}

func (r *postgresRepository) UpdateSensor(ctx context.Context, id string, c *Capteur) (*Capteur, error) {
	query := `
		UPDATE capteurs
		SET zone_id = $1, nom = $2, type = $3, statut = $4
		WHERE id = $5`
	tag, err := r.db.Exec(ctx, query, c.ZoneID, c.Nom, c.Type, c.Statut, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update sensor: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrSensorNotFound
	}
	c.ID = id
	return c, nil
}

func (r *postgresRepository) DeleteSensor(ctx context.Context, id string) error {
	query := `DELETE FROM capteurs WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete sensor: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSensorNotFound
	}
	return nil
}

func (r *postgresRepository) ListAllAlarms(ctx context.Context) ([]Alarme, error) {
	query := `
		SELECT id, zone_id, capteur_id, statut, declenchee_a, acquittee_a, acquittee_par
		FROM alarmes
		ORDER BY declenchee_a DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all alarms: %w", err)
	}
	defer rows.Close()

	var list []Alarme
	for rows.Next() {
		var al Alarme
		var acqA sql.NullTime
		var acqPar sql.NullString
		err := rows.Scan(&al.ID, &al.ZoneID, &al.CapteurID, &al.Statut, &al.DeclencheeA, &acqA, &acqPar)
		if err != nil {
			return nil, fmt.Errorf("failed to scan alarm row: %w", err)
		}
		if acqA.Valid {
			al.AcquitteeA = &acqA.Time
		}
		if acqPar.Valid {
			al.AcquitteePar = &acqPar.String
		}
		list = append(list, al)
	}
	return list, nil
}

