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
	query := `SELECT id, nom, url_rtsp, site_id, statut, created_at, ip, port, username, password_encrypted, ptz_supported, profile_token FROM cameras ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query cameras: %w", err)
	}
	defer rows.Close()

	var cameras []Camera
	for rows.Next() {
		var c Camera
		var siteID sql.NullString
		var ip, username, passwordEncrypted, profileToken sql.NullString
		var port sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Nom, &c.URLRTSP, &siteID, &c.Statut, &c.CreatedAt, &ip, &port, &username, &passwordEncrypted, &c.PTZSupported, &profileToken); err != nil {
			return nil, fmt.Errorf("failed to scan camera row: %w", err)
		}
		if siteID.Valid {
			c.SiteID = &siteID.String
		}
		if ip.Valid {
			c.IP = &ip.String
		}
		if port.Valid {
			p := int(port.Int64)
			c.Port = &p
		}
		if username.Valid {
			c.Username = &username.String
		}
		if passwordEncrypted.Valid {
			c.PasswordEncrypted = &passwordEncrypted.String
		}
		if profileToken.Valid {
			c.ProfileToken = &profileToken.String
		}
		cameras = append(cameras, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during cameras iteration: %w", err)
	}

	return cameras, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id string) (*Camera, error) {
	query := `SELECT id, nom, url_rtsp, site_id, statut, created_at, ip, port, username, password_encrypted, ptz_supported, profile_token FROM cameras WHERE id = $1`
	var c Camera
	var siteID sql.NullString
	var ip, username, passwordEncrypted, profileToken sql.NullString
	var port sql.NullInt64
	err := r.db.QueryRow(ctx, query, id).Scan(&c.ID, &c.Nom, &c.URLRTSP, &siteID, &c.Statut, &c.CreatedAt, &ip, &port, &username, &passwordEncrypted, &c.PTZSupported, &profileToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCameraNotFound
		}
		return nil, fmt.Errorf("failed to get camera by id: %w", err)
	}
	if siteID.Valid {
		c.SiteID = &siteID.String
	}
	if ip.Valid {
		c.IP = &ip.String
	}
	if port.Valid {
		p := int(port.Int64)
		c.Port = &p
	}
	if username.Valid {
		c.Username = &username.String
	}
	if passwordEncrypted.Valid {
		c.PasswordEncrypted = &passwordEncrypted.String
	}
	if profileToken.Valid {
		c.ProfileToken = &profileToken.String
	}
	return &c, nil
}

func (r *postgresRepository) Create(ctx context.Context, c *Camera) (*Camera, error) {
	query := `
		INSERT INTO cameras (nom, url_rtsp, site_id, statut, ip, port, username, password_encrypted, ptz_supported, profile_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`
	
	var siteID sql.NullString
	if c.SiteID != nil {
		siteID.String = *c.SiteID
		siteID.Valid = true
	}

	var ip, username, passwordEncrypted, profileToken sql.NullString
	var port sql.NullInt64

	if c.IP != nil {
		ip.String = *c.IP
		ip.Valid = true
	}
	if c.Port != nil {
		port.Int64 = int64(*c.Port)
		port.Valid = true
	}
	if c.Username != nil {
		username.String = *c.Username
		username.Valid = true
	}
	if c.PasswordEncrypted != nil {
		passwordEncrypted.String = *c.PasswordEncrypted
		passwordEncrypted.Valid = true
	}
	if c.ProfileToken != nil {
		profileToken.String = *c.ProfileToken
		profileToken.Valid = true
	}

	err := r.db.QueryRow(ctx, query, c.Nom, c.URLRTSP, siteID, c.Statut, ip, port, username, passwordEncrypted, c.PTZSupported, profileToken).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create camera: %w", err)
	}

	return c, nil
}

func (r *postgresRepository) Update(ctx context.Context, c *Camera) (*Camera, error) {
	query := `
		UPDATE cameras
		SET nom = $1, url_rtsp = $2, site_id = $3, statut = $4, ip = $5, port = $6, username = $7, password_encrypted = $8, ptz_supported = $9, profile_token = $10
		WHERE id = $11`

	var siteID sql.NullString
	if c.SiteID != nil {
		siteID.String = *c.SiteID
		siteID.Valid = true
	}

	var ip, username, passwordEncrypted, profileToken sql.NullString
	var port sql.NullInt64

	if c.IP != nil {
		ip.String = *c.IP
		ip.Valid = true
	}
	if c.Port != nil {
		port.Int64 = int64(*c.Port)
		port.Valid = true
	}
	if c.Username != nil {
		username.String = *c.Username
		username.Valid = true
	}
	if c.PasswordEncrypted != nil {
		passwordEncrypted.String = *c.PasswordEncrypted
		passwordEncrypted.Valid = true
	}
	if c.ProfileToken != nil {
		profileToken.String = *c.ProfileToken
		profileToken.Valid = true
	}

	tag, err := r.db.Exec(ctx, query, c.Nom, c.URLRTSP, siteID, c.Statut, ip, port, username, passwordEncrypted, c.PTZSupported, profileToken, c.ID)
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
