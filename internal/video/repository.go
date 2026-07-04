package video

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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

	// Recording management methods
	SaveRecording(ctx context.Context, rec *VideoRecording) error
	GetOverlappingRecordings(ctx context.Context, cameraID string, start, end time.Time) ([]VideoRecording, error)
	DeleteRecording(ctx context.Context, id string) error
	GetLocalRecordingsOlderThan(ctx context.Context, threshold time.Time) ([]VideoRecording, error)
	UpdateRecordingStorageType(ctx context.Context, id string, storageType string, filepath string) error
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) List(ctx context.Context) ([]Camera, error) {
	query := `SELECT id, nom, url_rtsp, main_stream_url, sub_stream_url, site_id, statut, created_at, ip, port, username, password_encrypted, ptz_supported, profile_token FROM cameras ORDER BY created_at ASC`
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
		var mainStreamURL, subStreamURL sql.NullString
		if err := rows.Scan(&c.ID, &c.Nom, &c.URLRTSP, &mainStreamURL, &subStreamURL, &siteID, &c.Statut, &c.CreatedAt, &ip, &port, &username, &passwordEncrypted, &c.PTZSupported, &profileToken); err != nil {
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
		if mainStreamURL.Valid {
			c.MainStreamURL = mainStreamURL.String
		}
		if subStreamURL.Valid {
			c.SubStreamURL = subStreamURL.String
		}
		cameras = append(cameras, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during cameras iteration: %w", err)
	}

	return cameras, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id string) (*Camera, error) {
	query := `SELECT id, nom, url_rtsp, main_stream_url, sub_stream_url, site_id, statut, created_at, ip, port, username, password_encrypted, ptz_supported, profile_token FROM cameras WHERE id = $1`
	var c Camera
	var siteID sql.NullString
	var ip, username, passwordEncrypted, profileToken sql.NullString
	var port sql.NullInt64
	var mainStreamURL, subStreamURL sql.NullString
	err := r.db.QueryRow(ctx, query, id).Scan(&c.ID, &c.Nom, &c.URLRTSP, &mainStreamURL, &subStreamURL, &siteID, &c.Statut, &c.CreatedAt, &ip, &port, &username, &passwordEncrypted, &c.PTZSupported, &profileToken)
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
	if mainStreamURL.Valid {
		c.MainStreamURL = mainStreamURL.String
	}
	if subStreamURL.Valid {
		c.SubStreamURL = subStreamURL.String
	}
	return &c, nil
}

func (r *postgresRepository) Create(ctx context.Context, c *Camera) (*Camera, error) {
	query := `
		INSERT INTO cameras (nom, url_rtsp, main_stream_url, sub_stream_url, site_id, statut, ip, port, username, password_encrypted, ptz_supported, profile_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at`
	
	var siteID sql.NullString
	if c.SiteID != nil {
		siteID.String = *c.SiteID
		siteID.Valid = true
	}

	var ip, username, passwordEncrypted, profileToken sql.NullString
	var port sql.NullInt64
	var mainStreamURL, subStreamURL sql.NullString

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
	if c.MainStreamURL != "" {
		mainStreamURL.String = c.MainStreamURL
		mainStreamURL.Valid = true
	}
	if c.SubStreamURL != "" {
		subStreamURL.String = c.SubStreamURL
		subStreamURL.Valid = true
	}

	err := r.db.QueryRow(ctx, query, c.Nom, c.URLRTSP, mainStreamURL, subStreamURL, siteID, c.Statut, ip, port, username, passwordEncrypted, c.PTZSupported, profileToken).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create camera: %w", err)
	}

	return c, nil
}

func (r *postgresRepository) Update(ctx context.Context, c *Camera) (*Camera, error) {
	query := `
		UPDATE cameras
		SET nom = $1, url_rtsp = $2, main_stream_url = $3, sub_stream_url = $4, site_id = $5, statut = $6, ip = $7, port = $8, username = $9, password_encrypted = $10, ptz_supported = $11, profile_token = $12
		WHERE id = $13`

	var siteID sql.NullString
	if c.SiteID != nil {
		siteID.String = *c.SiteID
		siteID.Valid = true
	}

	var ip, username, passwordEncrypted, profileToken sql.NullString
	var port sql.NullInt64
	var mainStreamURL, subStreamURL sql.NullString

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
	if c.MainStreamURL != "" {
		mainStreamURL.String = c.MainStreamURL
		mainStreamURL.Valid = true
	}
	if c.SubStreamURL != "" {
		subStreamURL.String = c.SubStreamURL
		subStreamURL.Valid = true
	}

	tag, err := r.db.Exec(ctx, query, c.Nom, c.URLRTSP, mainStreamURL, subStreamURL, siteID, c.Statut, ip, port, username, passwordEncrypted, c.PTZSupported, profileToken, c.ID)
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

func (r *postgresRepository) SaveRecording(ctx context.Context, rec *VideoRecording) error {
	query := `
		INSERT INTO video_recordings (camera_id, start_time, end_time, filepath, storage_type, file_size)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`
	err := r.db.QueryRow(ctx, query, rec.CameraID, rec.StartTime, rec.EndTime, rec.Filepath, rec.StorageType, rec.FileSize).
		Scan(&rec.ID, &rec.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save video recording: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetOverlappingRecordings(ctx context.Context, cameraID string, start, end time.Time) ([]VideoRecording, error) {
	query := `
		SELECT id, camera_id, start_time, end_time, filepath, storage_type, file_size, created_at
		FROM video_recordings
		WHERE camera_id = $1 AND start_time < $3 AND end_time > $2
		ORDER BY start_time ASC`
	rows, err := r.db.Query(ctx, query, cameraID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query overlapping recordings: %w", err)
	}
	defer rows.Close()

	var list []VideoRecording
	for rows.Next() {
		var rec VideoRecording
		err := rows.Scan(&rec.ID, &rec.CameraID, &rec.StartTime, &rec.EndTime, &rec.Filepath, &rec.StorageType, &rec.FileSize, &rec.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan video recording row: %w", err)
		}
		list = append(list, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during video recordings iteration: %w", err)
	}

	return list, nil
}

func (r *postgresRepository) DeleteRecording(ctx context.Context, id string) error {
	query := `DELETE FROM video_recordings WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete video recording: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetLocalRecordingsOlderThan(ctx context.Context, threshold time.Time) ([]VideoRecording, error) {
	query := `
		SELECT id, camera_id, start_time, end_time, filepath, storage_type, file_size, created_at
		FROM video_recordings
		WHERE storage_type = 'local' AND start_time < $1
		ORDER BY start_time ASC`
	rows, err := r.db.Query(ctx, query, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to query old local recordings: %w", err)
	}
	defer rows.Close()

	var list []VideoRecording
	for rows.Next() {
		var rec VideoRecording
		err := rows.Scan(&rec.ID, &rec.CameraID, &rec.StartTime, &rec.EndTime, &rec.Filepath, &rec.StorageType, &rec.FileSize, &rec.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan video recording row: %w", err)
		}
		list = append(list, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during old local recordings iteration: %w", err)
	}

	return list, nil
}

func (r *postgresRepository) UpdateRecordingStorageType(ctx context.Context, id string, storageType string, filepath string) error {
	query := `
		UPDATE video_recordings
		SET storage_type = $2, filepath = $3
		WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id, storageType, filepath)
	if err != nil {
		return fmt.Errorf("failed to update video recording storage type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("recording not found: %s", id)
	}
	return nil
}
