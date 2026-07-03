package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrRoleNotFound = errors.New("role not found")
)

type Repository interface {
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	CreateUser(ctx context.Context, email, passwordHash, roleName string) (*User, error)
}

type postgresRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT u.id, u.email, u.password_hash, u.role_id, u.created_at, u.updated_at,
		       r.id, r.name, r.description
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
		WHERE u.email = $1`

	var user User
	var roleID sql.NullInt32
	var rID sql.NullInt32
	var rName sql.NullString
	var rDesc sql.NullString

	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&roleID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&rID,
		&rName,
		&rDesc,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	if roleID.Valid {
		idVal := int(roleID.Int32)
		user.RoleID = &idVal
		if rID.Valid {
			user.Role = &Role{
				ID:          int(rID.Int32),
				Name:        rName.String,
				Description: rDesc.String,
			}
			// Fetch permissions for role
			perms, err := r.getPermissionsForRole(ctx, user.Role.ID)
			if err != nil {
				return nil, err
			}
			user.Role.Permissions = perms
		}
	}

	return &user, nil
}

func (r *postgresRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT u.id, u.email, u.password_hash, u.role_id, u.created_at, u.updated_at,
		       r.id, r.name, r.description
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
		WHERE u.id = $1`

	var user User
	var roleID sql.NullInt32
	var rID sql.NullInt32
	var rName sql.NullString
	var rDesc sql.NullString

	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&roleID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&rID,
		&rName,
		&rDesc,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	if roleID.Valid {
		idVal := int(roleID.Int32)
		user.RoleID = &idVal
		if rID.Valid {
			user.Role = &Role{
				ID:          int(rID.Int32),
				Name:        rName.String,
				Description: rDesc.String,
			}
			// Fetch permissions for role
			perms, err := r.getPermissionsForRole(ctx, user.Role.ID)
			if err != nil {
				return nil, err
			}
			user.Role.Permissions = perms
		}
	}

	return &user, nil
}

func (r *postgresRepository) CreateUser(ctx context.Context, email, passwordHash, roleName string) (*User, error) {
	// First get the role ID
	var roleID int
	err := r.db.QueryRow(ctx, "SELECT id FROM roles WHERE name = $1", roleName).Scan(&roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to find role %s: %w", roleName, err)
	}

	query := `
		INSERT INTO users (email, password_hash, role_id)
		VALUES ($1, $2, $3)
		RETURNING id, email, role_id, created_at, updated_at`

	var user User
	err = r.db.QueryRow(ctx, query, email, passwordHash, roleID).Scan(
		&user.ID,
		&user.Email,
		&user.RoleID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	return r.GetUserByID(ctx, user.ID)
}

func (r *postgresRepository) getPermissionsForRole(ctx context.Context, roleID int) ([]Permission, error) {
	query := `
		SELECT p.id, p.name, p.description, p.created_at
		FROM role_permissions rp
		JOIN permissions p ON rp.permission_id = p.id
		WHERE rp.role_id = $1`

	rows, err := r.db.Query(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to query role permissions: %w", err)
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan permission row: %w", err)
		}
		permissions = append(permissions, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during role permissions iteration: %w", err)
	}

	return permissions, nil
}
