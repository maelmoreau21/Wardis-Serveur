package video

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrCameraNotFound = errors.New("camera not found")
)

type Repository interface {
	List(ctx context.Context) ([]Camera, error)
	GetByID(ctx context.Context, id string) (*Camera, error)
	Create(ctx context.Context, c *Camera) (*Camera, error)
	Update(ctx context.Context, c *Camera) (*Camera, error)
	Delete(ctx context.Context, id string) error
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) List(ctx context.Context) ([]Camera, error) {
	query := `SELECT id, nom, url_rtsp, site_id, statut, created_at FROM cameras ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query cameras: %w", err)
	}
	defer rows.Close()

	var cameras []Camera
	for rows.Next() {
		var c Camera
		var siteID sql.NullString
		if err := rows.Scan(&c.ID, &c.Nom, &c.URLRTSP, &siteID, &c.Statut, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan camera row: %w", err)
		}
		if siteID.Valid {
			c.SiteID = &siteID.String
		}
		cameras = append(cameras, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during cameras iteration: %w", err)
	}

	return cameras, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id string) (*Camera, error) {
	query := `SELECT id, nom, url_rtsp, site_id, statut, created_at FROM cameras WHERE id = $1`
	var c Camera
	var siteID sql.NullString
	err := r.db.QueryRow(ctx, query, id).Scan(&c.ID, &c.Nom, &c.URLRTSP, &siteID, &c.Statut, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCameraNotFound
		}
		return nil, fmt.Errorf("failed to get camera by id: %w", err)
	}
	if siteID.Valid {
		c.SiteID = &siteID.String
	}
	return &c, nil
}

func (r *postgresRepository) Create(ctx context.Context, c *Camera) (*Camera, error) {
	query := `
		INSERT INTO cameras (nom, url_rtsp, site_id, statut)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`
	
	var siteID sql.NullString
	if c.SiteID != nil {
		siteID.String = *c.SiteID
		siteID.Valid = true
	}

	err := r.db.QueryRow(ctx, query, c.Nom, c.URLRTSP, siteID, c.Statut).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create camera: %w", err)
	}

	return c, nil
}

func (r *postgresRepository) Update(ctx context.Context, c *Camera) (*Camera, error) {
	query := `
		UPDATE cameras
		SET nom = $1, url_rtsp = $2, site_id = $3, statut = $4
		WHERE id = $5`

	var siteID sql.NullString
	if c.SiteID != nil {
		siteID.String = *c.SiteID
		siteID.Valid = true
	}

	tag, err := r.db.Exec(ctx, query, c.Nom, c.URLRTSP, siteID, c.Statut, c.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update camera: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return nil, ErrCameraNotFound
	}

	return c, nil
}

func (r *postgresRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM cameras WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete camera: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCameraNotFound
	}
	return nil
}
