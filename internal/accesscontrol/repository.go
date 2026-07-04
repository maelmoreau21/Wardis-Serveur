package accesscontrol

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDoorNotFound  = errors.New("door not found")
	ErrBadgeNotFound = errors.New("badge not found")
)

type Repository interface {
	ListDoors(ctx context.Context) ([]Door, error)
	GetDoorByID(ctx context.Context, id string) (*Door, error)
	UpdateDoorStatus(ctx context.Context, id string, status string) error
	GetBadgeByNumber(ctx context.Context, number string) (*Badge, error)
	AssignBadge(ctx context.Context, number string, userID string) (*Badge, error)
	CheckAccessPermission(ctx context.Context, badgeID string, doorID string) (bool, error)
	CreateAccessLog(ctx context.Context, log *AccessLog) (*AccessLog, error)
	ListAccessLogs(ctx context.Context) ([]AccessLog, error)
	ListCardholders(ctx context.Context) ([]Cardholder, error)
	GetCardholderByID(ctx context.Context, id string) (*Cardholder, error)
	CreateCardholder(ctx context.Context, c *Cardholder) (*Cardholder, error)
	UpdateCardholder(ctx context.Context, id string, c *Cardholder) (*Cardholder, error)
	DeleteCardholder(ctx context.Context, id string) error
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) ListDoors(ctx context.Context) ([]Door, error) {
	query := `SELECT id, site_id, zone_id, name, description, status, created_at FROM doors ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query doors: %w", err)
	}
	defer rows.Close()

	var doors []Door
	for rows.Next() {
		var d Door
		var zoneID sql.NullString
		if err := rows.Scan(&d.ID, &d.SiteID, &zoneID, &d.Name, &d.Description, &d.Status, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan door row: %w", err)
		}
		if zoneID.Valid {
			d.ZoneID = &zoneID.String
		}
		doors = append(doors, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during doors iteration: %w", err)
	}

	return doors, nil
}

func (r *postgresRepository) GetDoorByID(ctx context.Context, id string) (*Door, error) {
	query := `SELECT id, site_id, zone_id, name, description, status, created_at FROM doors WHERE id = $1`
	var d Door
	var zoneID sql.NullString
	err := r.db.QueryRow(ctx, query, id).Scan(&d.ID, &d.SiteID, &zoneID, &d.Name, &d.Description, &d.Status, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDoorNotFound
		}
		return nil, fmt.Errorf("failed to get door by id: %w", err)
	}
	if zoneID.Valid {
		d.ZoneID = &zoneID.String
	}
	return &d, nil
}

func (r *postgresRepository) UpdateDoorStatus(ctx context.Context, id string, status string) error {
	query := `UPDATE doors SET status = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update door status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDoorNotFound
	}
	return nil
}

func (r *postgresRepository) GetBadgeByNumber(ctx context.Context, number string) (*Badge, error) {
	query := `SELECT id, number, user_id, cardholder_id, status, created_at FROM badges WHERE number = $1`
	var b Badge
	var userID, cardholderID sql.NullString
	err := r.db.QueryRow(ctx, query, number).Scan(&b.ID, &b.Number, &userID, &cardholderID, &b.Status, &b.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBadgeNotFound
		}
		return nil, fmt.Errorf("failed to get badge by number: %w", err)
	}
	if userID.Valid {
		b.UserID = &userID.String
	}
	if cardholderID.Valid {
		b.CardholderID = &cardholderID.String
	}
	return &b, nil
}

func (r *postgresRepository) AssignBadge(ctx context.Context, number string, userID string) (*Badge, error) {
	query := `
		INSERT INTO badges (number, user_id, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (number) DO UPDATE SET user_id = $2, status = 'active'
		RETURNING id, number, user_id, status, created_at`
	var b Badge
	var uID sql.NullString
	err := r.db.QueryRow(ctx, query, number, userID).Scan(&b.ID, &b.Number, &uID, &b.Status, &b.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to assign badge: %w", err)
	}
	if uID.Valid {
		b.UserID = &uID.String
	}
	return &b, nil
}

func (r *postgresRepository) CheckAccessPermission(ctx context.Context, badgeID string, doorID string) (bool, error) {
	query := `SELECT allowed FROM access_permissions WHERE badge_id = $1 AND door_id = $2`
	var allowed bool
	err := r.db.QueryRow(ctx, query, badgeID, doorID).Scan(&allowed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check access permission: %w", err)
	}
	return allowed, nil
}

func (r *postgresRepository) CreateAccessLog(ctx context.Context, log *AccessLog) (*AccessLog, error) {
	query := `
		INSERT INTO access_logs (badge_id, badge_number, door_id, site_id, user_id, cardholder_id, access_type, denied_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, badge_id, badge_number, door_id, site_id, user_id, cardholder_id, access_type, denied_reason, created_at`
	
	var res AccessLog
	var badgeID, doorID, siteID, userID, cardholderID sql.NullString
	var deniedReason sql.NullString

	err := r.db.QueryRow(ctx, query,
		log.BadgeID,
		log.BadgeNumber,
		log.DoorID,
		log.SiteID,
		log.UserID,
		log.CardholderID,
		log.AccessType,
		log.DeniedReason,
	).Scan(
		&res.ID,
		&badgeID,
		&res.BadgeNumber,
		&doorID,
		&siteID,
		&userID,
		&cardholderID,
		&res.AccessType,
		&deniedReason,
		&res.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create access log: %w", err)
	}

	if badgeID.Valid {
		res.BadgeID = &badgeID.String
	}
	if doorID.Valid {
		res.DoorID = &doorID.String
	}
	if siteID.Valid {
		res.SiteID = &siteID.String
	}
	if userID.Valid {
		res.UserID = &userID.String
	}
	if cardholderID.Valid {
		res.CardholderID = &cardholderID.String
	}
	if deniedReason.Valid {
		res.DeniedReason = &deniedReason.String
	}

	return &res, nil
}

func (r *postgresRepository) ListAccessLogs(ctx context.Context) ([]AccessLog, error) {
	query := `
		SELECT 
			al.id, al.badge_id, al.badge_number, al.door_id, al.site_id, al.user_id, al.cardholder_id, 
			COALESCE(c.first_name || ' ' || c.last_name, '') as cardholder_name, COALESCE(c.photo, '') as cardholder_photo,
			al.access_type, al.denied_reason, al.created_at 
		FROM access_logs al
		LEFT JOIN cardholders c ON al.cardholder_id = c.id
		ORDER BY al.created_at DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query access logs: %w", err)
	}
	defer rows.Close()

	var logs []AccessLog
	for rows.Next() {
		var log AccessLog
		var badgeID, doorID, siteID, userID, cardholderID sql.NullString
		var cardholderName, cardholderPhoto sql.NullString
		var deniedReason sql.NullString

		err := rows.Scan(
			&log.ID,
			&badgeID,
			&log.BadgeNumber,
			&doorID,
			&siteID,
			&userID,
			&cardholderID,
			&cardholderName,
			&cardholderPhoto,
			&log.AccessType,
			&deniedReason,
			&log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan access log row: %w", err)
		}

		if badgeID.Valid {
			log.BadgeID = &badgeID.String
		}
		if doorID.Valid {
			log.DoorID = &doorID.String
		}
		if siteID.Valid {
			log.SiteID = &siteID.String
		}
		if userID.Valid {
			log.UserID = &userID.String
		}
		if cardholderID.Valid {
			log.CardholderID = &cardholderID.String
		}
		if cardholderName.Valid && cardholderName.String != "" {
			log.CardholderName = &cardholderName.String
		}
		if cardholderPhoto.Valid && cardholderPhoto.String != "" {
			log.CardholderPhoto = &cardholderPhoto.String
		}
		if deniedReason.Valid {
			log.DeniedReason = &deniedReason.String
		}

		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during access logs iteration: %w", err)
	}

	return logs, nil
}

func (r *postgresRepository) ListCardholders(ctx context.Context) ([]Cardholder, error) {
	query := `
		SELECT c.id, c.first_name, c.last_name, c.company, c.email, c.photo, c.access_group, c.schedule, COALESCE(b.number, '') as badge_number, c.created_at, c.updated_at
		FROM cardholders c
		LEFT JOIN badges b ON b.cardholder_id = c.id AND b.status = 'active'
		ORDER BY c.created_at DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list cardholders: %w", err)
	}
	defer rows.Close()

	var list []Cardholder
	for rows.Next() {
		var c Cardholder
		err := rows.Scan(
			&c.ID, &c.FirstName, &c.LastName, &c.Company, &c.Email, &c.Photo, &c.AccessGroup, &c.Schedule, &c.BadgeNumber, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cardholder row: %w", err)
		}
		list = append(list, c)
	}
	return list, nil
}

func (r *postgresRepository) GetCardholderByID(ctx context.Context, id string) (*Cardholder, error) {
	query := `
		SELECT c.id, c.first_name, c.last_name, c.company, c.email, c.photo, c.access_group, c.schedule, COALESCE(b.number, '') as badge_number, c.created_at, c.updated_at
		FROM cardholders c
		LEFT JOIN badges b ON b.cardholder_id = c.id AND b.status = 'active'
		WHERE c.id = $1`
	var c Cardholder
	err := r.db.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.FirstName, &c.LastName, &c.Company, &c.Email, &c.Photo, &c.AccessGroup, &c.Schedule, &c.BadgeNumber, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("cardholder not found")
		}
		return nil, fmt.Errorf("failed to get cardholder: %w", err)
	}
	return &c, nil
}

func (r *postgresRepository) CreateCardholder(ctx context.Context, c *Cardholder) (*Cardholder, error) {
	query := `
		INSERT INTO cardholders (first_name, last_name, company, email, photo, access_group, schedule)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`
	err := r.db.QueryRow(ctx, query, c.FirstName, c.LastName, c.Company, c.Email, c.Photo, c.AccessGroup, c.Schedule).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create cardholder: %w", err)
	}

	if c.BadgeNumber != "" {
		_, _ = r.db.Exec(ctx, `UPDATE badges SET cardholder_id = NULL WHERE number = $1`, c.BadgeNumber)
		badgeQuery := `
			INSERT INTO badges (number, cardholder_id, status)
			VALUES ($1, $2, 'active')
			ON CONFLICT (number) DO UPDATE SET cardholder_id = $2, status = 'active'`
		_, err = r.db.Exec(ctx, badgeQuery, c.BadgeNumber, c.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to associate badge on cardholder create: %w", err)
		}

		doorsRows, err := r.db.Query(ctx, `SELECT id FROM doors`)
		if err == nil {
			defer doorsRows.Close()
			var badgeUUID string
			err = r.db.QueryRow(ctx, `SELECT id FROM badges WHERE number = $1`, c.BadgeNumber).Scan(&badgeUUID)
			if err == nil {
				for doorsRows.Next() {
					var doorID string
					if err := doorsRows.Scan(&doorID); err == nil {
						_, _ = r.db.Exec(ctx, `INSERT INTO access_permissions (badge_id, door_id, allowed) VALUES ($1, $2, true) ON CONFLICT (badge_id, door_id) DO NOTHING`, badgeUUID, doorID)
					}
				}
			}
		}
	}

	return c, nil
}

func (r *postgresRepository) UpdateCardholder(ctx context.Context, id string, c *Cardholder) (*Cardholder, error) {
	query := `
		UPDATE cardholders
		SET first_name = $1, last_name = $2, company = $3, email = $4, photo = $5, access_group = $6, schedule = $7, updated_at = CURRENT_TIMESTAMP
		WHERE id = $8
		RETURNING created_at, updated_at`
	err := r.db.QueryRow(ctx, query, c.FirstName, c.LastName, c.Company, c.Email, c.Photo, c.AccessGroup, c.Schedule, id).Scan(&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to update cardholder: %w", err)
	}
	c.ID = id

	_, _ = r.db.Exec(ctx, `UPDATE badges SET cardholder_id = NULL WHERE cardholder_id = $1`, id)

	if c.BadgeNumber != "" {
		_, _ = r.db.Exec(ctx, `UPDATE badges SET cardholder_id = NULL WHERE number = $1`, c.BadgeNumber)
		badgeQuery := `
			INSERT INTO badges (number, cardholder_id, status)
			VALUES ($1, $2, 'active')
			ON CONFLICT (number) DO UPDATE SET cardholder_id = $2, status = 'active'`
		_, err = r.db.Exec(ctx, badgeQuery, c.BadgeNumber, id)
		if err != nil {
			return nil, fmt.Errorf("failed to associate badge on cardholder update: %w", err)
		}

		doorsRows, err := r.db.Query(ctx, `SELECT id FROM doors`)
		if err == nil {
			defer doorsRows.Close()
			var badgeUUID string
			err = r.db.QueryRow(ctx, `SELECT id FROM badges WHERE number = $1`, c.BadgeNumber).Scan(&badgeUUID)
			if err == nil {
				for doorsRows.Next() {
					var doorID string
					if err := doorsRows.Scan(&doorID); err == nil {
						_, _ = r.db.Exec(ctx, `INSERT INTO access_permissions (badge_id, door_id, allowed) VALUES ($1, $2, true) ON CONFLICT (badge_id, door_id) DO NOTHING`, badgeUUID, doorID)
					}
				}
			}
		}
	}

	return c, nil
}

func (r *postgresRepository) DeleteCardholder(ctx context.Context, id string) error {
	_, _ = r.db.Exec(ctx, `UPDATE badges SET cardholder_id = NULL WHERE cardholder_id = $1`, id)
	query := `DELETE FROM cardholders WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete cardholder: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("cardholder not found")
	}
	return nil
}
