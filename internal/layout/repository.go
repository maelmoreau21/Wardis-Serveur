package layout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrViewNotFound = errors.New("saved view not found")
)

type Repository interface {
	List(ctx context.Context, userID string) ([]SavedView, error)
	GetByID(ctx context.Context, id string) (*SavedView, error)
	Create(ctx context.Context, v *SavedView) (*SavedView, error)
	Update(ctx context.Context, id string, v *SavedView) (*SavedView, error)
	Delete(ctx context.Context, id string) error
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) List(ctx context.Context, userID string) ([]SavedView, error) {
	query := `SELECT id, name, grid_layout, slots, user_id, created_at, updated_at FROM saved_views WHERE user_id = $1 OR user_id IS NULL ORDER BY name ASC`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query saved views: %w", err)
	}
	defer rows.Close()

	var list []SavedView
	for rows.Next() {
		var v SavedView
		var uID sql.NullString
		if err := rows.Scan(&v.ID, &v.Name, &v.GridLayout, &v.Slots, &uID, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan saved view row: %w", err)
		}
		if uID.Valid {
			v.UserID = &uID.String
		}
		list = append(list, v)
	}
	return list, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id string) (*SavedView, error) {
	query := `SELECT id, name, grid_layout, slots, user_id, created_at, updated_at FROM saved_views WHERE id = $1`
	var v SavedView
	var uID sql.NullString
	err := r.db.QueryRow(ctx, query, id).Scan(&v.ID, &v.Name, &v.GridLayout, &v.Slots, &uID, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrViewNotFound
		}
		return nil, fmt.Errorf("failed to get saved view by id: %w", err)
	}
	if uID.Valid {
		v.UserID = &uID.String
	}
	return &v, nil
}

func (r *postgresRepository) Create(ctx context.Context, v *SavedView) (*SavedView, error) {
	query := `
		INSERT INTO saved_views (name, grid_layout, slots, user_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`
	var uID sql.NullString
	if v.UserID != nil {
		uID.String = *v.UserID
		uID.Valid = true
	}
	err := r.db.QueryRow(ctx, query, v.Name, v.GridLayout, v.Slots, uID).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create saved view: %w", err)
	}
	return v, nil
}

func (r *postgresRepository) Update(ctx context.Context, id string, v *SavedView) (*SavedView, error) {
	query := `
		UPDATE saved_views
		SET name = $1, grid_layout = $2, slots = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $4`
	tag, err := r.db.Exec(ctx, query, v.Name, v.GridLayout, v.Slots, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update saved view: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrViewNotFound
	}
	v.ID = id
	return r.GetByID(ctx, id)
}

func (r *postgresRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM saved_views WHERE id = $1`
	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete saved view: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrViewNotFound
	}
	return nil
}
