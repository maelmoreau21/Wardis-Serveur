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
	CheckEntityPermission(ctx context.Context, userID string, permissionName string, entityType string, entityID string) (bool, error)
	UpdatePassword(ctx context.Context, id string, passwordHash string) error
	ListUsers(ctx context.Context) ([]User, error)
	DeleteUser(ctx context.Context, id string) error
	ListRoles(ctx context.Context) ([]Role, error)
	ListPermissions(ctx context.Context) ([]Permission, error)
	GetEntityPermissionsForUser(ctx context.Context, userID string) ([]EntityPermission, error)
	SaveEntityPermissions(ctx context.Context, userID string, entityPermissions []EntityPermission) error
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

func (r *postgresRepository) CheckEntityPermission(ctx context.Context, userID string, permissionName string, entityType string, entityID string) (bool, error) {
	query := `
		WITH user_roles_list AS (
			SELECT role_id FROM user_roles WHERE user_id = $1
			UNION
			SELECT role_id FROM users WHERE id = $1 AND role_id IS NOT NULL
		),
		perm_id AS (
			SELECT id FROM permissions WHERE name = $2
		),
		user_override AS (
			SELECT allowed
			FROM entity_permissions ep
			JOIN perm_id p ON ep.permission_id = p.id
			WHERE ep.user_id = $1
			  AND ep.entity_type = $3
			  AND ep.entity_id = $4
		),
		role_overrides AS (
			SELECT allowed
			FROM entity_permissions ep
			JOIN perm_id p ON ep.permission_id = p.id
			WHERE ep.role_id IN (SELECT role_id FROM user_roles_list)
			  AND ep.entity_type = $3
			  AND ep.entity_id = $4
		),
		global_permission AS (
			SELECT EXISTS (
				SELECT 1
				FROM role_permissions rp
				JOIN perm_id p ON rp.permission_id = p.id
				WHERE rp.role_id IN (SELECT role_id FROM user_roles_list)
			) AS allowed
		)
		SELECT COALESCE(
			(SELECT allowed FROM user_override LIMIT 1),
			(
				SELECT CASE 
					WHEN bool_and(allowed) = false THEN false
					ELSE bool_or(allowed)
				END
				FROM role_overrides
			),
			(SELECT allowed FROM global_permission)
		) AS has_permission;`

	var hasPermission bool
	err := r.db.QueryRow(ctx, query, userID, permissionName, entityType, entityID).Scan(&hasPermission)
	if err != nil {
		return false, fmt.Errorf("failed to check entity permission: %w", err)
	}

	return hasPermission, nil
}

func (r *postgresRepository) UpdatePassword(ctx context.Context, id string, passwordHash string) error {
	query := `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, passwordHash, id)
	return err
}

func (r *postgresRepository) ListUsers(ctx context.Context) ([]User, error) {
	query := `
		SELECT u.id, u.email, u.password_hash, u.role_id, u.created_at, u.updated_at,
		       r.id, r.name, r.description
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
		ORDER BY u.email ASC`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var roleID sql.NullInt32
		var rID sql.NullInt32
		var rName sql.NullString
		var rDesc sql.NullString

		err := rows.Scan(
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
			return nil, fmt.Errorf("failed to scan user row: %w", err)
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
			}
		}
		users = append(users, user)
	}
	return users, nil
}

func (r *postgresRepository) DeleteUser(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	return err
}

func (r *postgresRepository) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := r.db.Query(ctx, "SELECT id, name, description, created_at FROM roles ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, nil
}

func (r *postgresRepository) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := r.db.Query(ctx, "SELECT id, name, description, created_at FROM permissions ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		permissions = append(permissions, p)
	}
	return permissions, nil
}

func (r *postgresRepository) GetEntityPermissionsForUser(ctx context.Context, userID string) ([]EntityPermission, error) {
	query := `
		SELECT ep.entity_id, ep.entity_type, p.name, ep.allowed
		FROM entity_permissions ep
		JOIN permissions p ON ep.permission_id = p.id
		WHERE ep.user_id = $1`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []EntityPermission
	for rows.Next() {
		var ep EntityPermission
		if err := rows.Scan(&ep.EntityID, &ep.EntityType, &ep.PermissionName, &ep.Allowed); err != nil {
			return nil, err
		}
		perms = append(perms, ep)
	}
	return perms, nil
}

func (r *postgresRepository) SaveEntityPermissions(ctx context.Context, userID string, entityPermissions []EntityPermission) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "DELETE FROM entity_permissions WHERE user_id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to clean existing entity permissions: %w", err)
	}

	for _, ep := range entityPermissions {
		var permID int
		err := tx.QueryRow(ctx, "SELECT id FROM permissions WHERE name = $1", ep.PermissionName).Scan(&permID)
		if err != nil {
			return fmt.Errorf("permission %s not found: %w", ep.PermissionName, err)
		}

		query := `
			INSERT INTO entity_permissions (user_id, entity_type, entity_id, permission_id, allowed)
			VALUES ($1, $2, $3, $4, $5)`
		_, err = tx.Exec(ctx, query, userID, ep.EntityType, ep.EntityID, permID, ep.Allowed)
		if err != nil {
			return fmt.Errorf("failed to insert entity permission override: %w", err)
		}
	}

	return tx.Commit(ctx)
}
