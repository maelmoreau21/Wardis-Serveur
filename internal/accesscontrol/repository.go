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
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) ListDoors(ctx context.Context) ([]Door, error) {
	query := `SELECT id, site_id, name, description, status, created_at FROM doors ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query doors: %w", err)
	}
	defer rows.Close()

	var doors []Door
	for rows.Next() {
		var d Door
		if err := rows.Scan(&d.ID, &d.SiteID, &d.Name, &d.Description, &d.Status, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan door row: %w", err)
		}
		doors = append(doors, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during doors iteration: %w", err)
	}

	return doors, nil
}

func (r *postgresRepository) GetDoorByID(ctx context.Context, id string) (*Door, error) {
	query := `SELECT id, site_id, name, description, status, created_at FROM doors WHERE id = $1`
	var d Door
	err := r.db.QueryRow(ctx, query, id).Scan(&d.ID, &d.SiteID, &d.Name, &d.Description, &d.Status, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDoorNotFound
		}
		return nil, fmt.Errorf("failed to get door by id: %w", err)
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
	query := `SELECT id, number, user_id, status, created_at FROM badges WHERE number = $1`
	var b Badge
	var userID sql.NullString
	err := r.db.QueryRow(ctx, query, number).Scan(&b.ID, &b.Number, &userID, &b.Status, &b.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBadgeNotFound
		}
		return nil, fmt.Errorf("failed to get badge by number: %w", err)
	}
	if userID.Valid {
		b.UserID = &userID.String
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
		INSERT INTO access_logs (badge_id, badge_number, door_id, site_id, user_id, access_type, denied_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, badge_id, badge_number, door_id, site_id, user_id, access_type, denied_reason, created_at`
	
	var res AccessLog
	var badgeID, doorID, siteID, userID sql.NullString
	var deniedReason sql.NullString

	err := r.db.QueryRow(ctx, query,
		log.BadgeID,
		log.BadgeNumber,
		log.DoorID,
		log.SiteID,
		log.UserID,
		log.AccessType,
		log.DeniedReason,
	).Scan(
		&res.ID,
		&badgeID,
		&res.BadgeNumber,
		&doorID,
		&siteID,
		&userID,
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
	if deniedReason.Valid {
		res.DeniedReason = &deniedReason.String
	}

	return &res, nil
}

func (r *postgresRepository) ListAccessLogs(ctx context.Context) ([]AccessLog, error) {
	query := `
		SELECT id, badge_id, badge_number, door_id, site_id, user_id, access_type, denied_reason, created_at 
		FROM access_logs 
		ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query access logs: %w", err)
	}
	defer rows.Close()

	var logs []AccessLog
	for rows.Next() {
		var log AccessLog
		var badgeID, doorID, siteID, userID sql.NullString
		var deniedReason sql.NullString

		err := rows.Scan(
			&log.ID,
			&badgeID,
			&log.BadgeNumber,
			&doorID,
			&siteID,
			&userID,
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
