package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEventNotFound = errors.New("event not found")
)

type Repository interface {
	InsertEvent(ctx context.Context, event *CorrelatedEvent) (*CorrelatedEvent, error)
	ListEvents(ctx context.Context) ([]CorrelatedEvent, error)
	GetClosestCameraForZone(ctx context.Context, zoneID string) (*ClosestCamera, error)
	GetClosestCameraForDoor(ctx context.Context, doorID string) (*ClosestCamera, error)
	GetLastBadgeSwipeInZone(ctx context.Context, zoneID string) (*LastBadgeSwipe, error)
	GetZoneIDForDoor(ctx context.Context, doorID string) (string, error)
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) InsertEvent(ctx context.Context, event *CorrelatedEvent) (*CorrelatedEvent, error) {
	detailsJSON, err := json.Marshal(event.Details)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event details: %w", err)
	}

	query := `
		INSERT INTO events_log (event_type, source_id, zone_id, capteur_id, door_id, badge_number, camera_id, timestamp, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`

	err = r.db.QueryRow(ctx, query,
		event.EventType,
		event.SourceID,
		event.ZoneID,
		event.CapteurID,
		event.DoorID,
		event.BadgeNumber,
		event.CameraID,
		event.Timestamp,
		detailsJSON,
	).Scan(&event.ID, &event.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to insert correlated event: %w", err)
	}

	return event, nil
}

func (r *postgresRepository) ListEvents(ctx context.Context) ([]CorrelatedEvent, error) {
	query := `
		SELECT id, event_type, source_id, zone_id, capteur_id, door_id, badge_number, camera_id, timestamp, details, created_at
		FROM events_log
		ORDER BY timestamp DESC
		LIMIT 100
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []CorrelatedEvent
	for rows.Next() {
		var e CorrelatedEvent
		var detailsBytes []byte
		var zoneID, capteurID, doorID, badgeNumber, cameraID sql.NullString

		err := rows.Scan(
			&e.ID,
			&e.EventType,
			&e.SourceID,
			&zoneID,
			&capteurID,
			&doorID,
			&badgeNumber,
			&cameraID,
			&e.Timestamp,
			&detailsBytes,
			&e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		if zoneID.Valid {
			e.ZoneID = &zoneID.String
		}
		if capteurID.Valid {
			e.CapteurID = &capteurID.String
		}
		if doorID.Valid {
			e.DoorID = &doorID.String
		}
		if badgeNumber.Valid {
			e.BadgeNumber = &badgeNumber.String
		}
		if cameraID.Valid {
			e.CameraID = &cameraID.String
		}

		if err := json.Unmarshal(detailsBytes, &e.Details); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event details: %w", err)
		}

		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during events iteration: %w", err)
	}

	return events, nil
}

func (r *postgresRepository) GetClosestCameraForZone(ctx context.Context, zoneID string) (*ClosestCamera, error) {
	query := `
		SELECT id, nom, url_rtsp, statut
		FROM cameras
		WHERE zone_id = $1 AND statut = 'active'
		LIMIT 1
	`

	var cam ClosestCamera
	err := r.db.QueryRow(ctx, query, zoneID).Scan(&cam.ID, &cam.Nom, &cam.URLRTSP, &cam.Statut)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get closest camera for zone: %w", err)
	}

	return &cam, nil
}

func (r *postgresRepository) GetClosestCameraForDoor(ctx context.Context, doorID string) (*ClosestCamera, error) {
	query := `
		SELECT id, nom, url_rtsp, statut
		FROM cameras
		WHERE zone_id = (SELECT zone_id FROM doors WHERE id = $1) AND statut = 'active'
		LIMIT 1
	`

	var cam ClosestCamera
	err := r.db.QueryRow(ctx, query, doorID).Scan(&cam.ID, &cam.Nom, &cam.URLRTSP, &cam.Statut)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get closest camera for door: %w", err)
	}

	return &cam, nil
}

func (r *postgresRepository) GetLastBadgeSwipeInZone(ctx context.Context, zoneID string) (*LastBadgeSwipe, error) {
	query := `
		SELECT al.id, al.badge_number, al.door_id, al.user_id, al.access_type, al.created_at
		FROM access_logs al
		JOIN doors d ON al.door_id = d.id
		WHERE d.zone_id = $1
		ORDER BY al.created_at DESC
		LIMIT 1
	`

	var swipe LastBadgeSwipe
	var doorID, userID sql.NullString
	err := r.db.QueryRow(ctx, query, zoneID).Scan(
		&swipe.LogID,
		&swipe.BadgeNumber,
		&doorID,
		&userID,
		&swipe.AccessType,
		&swipe.Timestamp,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get last badge swipe in zone: %w", err)
	}

	if doorID.Valid {
		swipe.DoorID = &doorID.String
	}
	if userID.Valid {
		swipe.UserID = &userID.String
	}

	return &swipe, nil
}

func (r *postgresRepository) GetZoneIDForDoor(ctx context.Context, doorID string) (string, error) {
	query := `SELECT zone_id FROM doors WHERE id = $1`
	var zoneID sql.NullString
	err := r.db.QueryRow(ctx, query, doorID).Scan(&zoneID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get zone_id for door: %w", err)
	}
	if !zoneID.Valid {
		return "", nil
	}
	return zoneID.String, nil
}
