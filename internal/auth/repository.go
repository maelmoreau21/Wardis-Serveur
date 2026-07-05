package auth

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

	// MFA TOTP Support
	UpdateMfaSecret(ctx context.Context, userID string, secret string) error
	EnableMfa(ctx context.Context, userID string) error
	DisableMfa(ctx context.Context, userID string) error

	// Custom Roles CRUD
	CreateRole(ctx context.Context, name, description string, permissions []string) (*Role, error)
	UpdateRole(ctx context.Context, roleID int, description string, permissions []string) error
	DeleteRole(ctx context.Context, roleID int) error

	// Active Sessions
	CreateSession(ctx context.Context, userID, tokenHash, ip, userAgent string, expiresAt time.Time) (string, error)
	IsSessionActive(ctx context.Context, sessionID string) (bool, error)
	RevokeSession(ctx context.Context, sessionID string) error
	RevokeUserSessions(ctx context.Context, userID string) error
	ListActiveSessions(ctx context.Context) ([]Session, error)
	ListUserActiveSessions(ctx context.Context, userID string) ([]Session, error)
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
		       r.id, r.name, r.description, u.mfa_enabled, COALESCE(u.totp_secret, '')
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
		&user.MfaEnabled,
		&user.TotpSecret,
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
		       r.id, r.name, r.description, u.mfa_enabled, COALESCE(u.totp_secret, '')
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
		&user.MfaEnabled,
		&user.TotpSecret,
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
	// First, check if there is a direct override for the specific camera/door
	var overrideVal bool
	
	queryOverride := `
		SELECT ep.allowed 
		FROM entity_permissions ep
		JOIN permissions p ON ep.permission_id = p.id
		WHERE (ep.user_id = $1 OR ep.role_id IN (
			SELECT role_id FROM user_roles WHERE user_id = $1
			UNION
			SELECT role_id FROM users WHERE id = $1 AND role_id IS NOT NULL
		))
		AND ep.entity_type = $2 
		AND ep.entity_id = $3
		AND p.name = $4
		ORDER BY ep.user_id ASC NULLS LAST -- User overrides take precedence over role overrides
		LIMIT 1`
		
	err := r.db.QueryRow(ctx, queryOverride, userID, entityType, entityID, permissionName).Scan(&overrideVal)
	if err == nil {
		// Specific override exists for this entity! (user or role override)
		return overrideVal, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("failed to check direct entity permission: %w", err)
	}

	// No direct override on the camera/door.
	// If the entity is a camera or door, check if there is a site override.
	if entityType == "camera" || entityType == "door" {
		var siteID sql.NullString
		var siteQuery string
		if entityType == "camera" {
			siteQuery = "SELECT site_id FROM cameras WHERE id = $1"
		} else {
			siteQuery = "SELECT site_id FROM doors WHERE id = $1"
		}
		
		err = r.db.QueryRow(ctx, siteQuery, entityID).Scan(&siteID)
		if err == nil && siteID.Valid && siteID.String != "" {
			// Check if there is an override for the site of this entity (using site:view permission)
			var siteOverrideVal bool
			errSiteOverride := r.db.QueryRow(ctx, queryOverride, userID, "site", siteID.String, "site:view").Scan(&siteOverrideVal)
			if errSiteOverride == nil {
				// Site-level override found!
				return siteOverrideVal, nil
			}
			if !errors.Is(errSiteOverride, pgx.ErrNoRows) {
				return false, fmt.Errorf("failed to check site permission override: %w", errSiteOverride)
			}
		}
	}

	// If no direct or site overrides exist, fall back to global role permission check
	queryGlobal := `
		WITH user_roles_list AS (
			SELECT role_id FROM user_roles WHERE user_id = $1
			UNION
			SELECT role_id FROM users WHERE id = $1 AND role_id IS NOT NULL
		)
		SELECT EXISTS (
			SELECT 1
			FROM role_permissions rp
			JOIN permissions p ON rp.permission_id = p.id
			WHERE rp.role_id IN (SELECT role_id FROM user_roles_list)
			AND p.name = $2
		)`
	var allowedGlobal bool
	err = r.db.QueryRow(ctx, queryGlobal, userID, permissionName).Scan(&allowedGlobal)
	if err != nil {
		return false, fmt.Errorf("failed to check global role permission: %w", err)
	}

	return allowedGlobal, nil
}

func (r *postgresRepository) UpdatePassword(ctx context.Context, id string, passwordHash string) error {
	query := `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, passwordHash, id)
	return err
}

func (r *postgresRepository) ListUsers(ctx context.Context) ([]User, error) {
	query := `
		SELECT u.id, u.email, u.password_hash, u.role_id, u.created_at, u.updated_at,
		       r.id, r.name, r.description, u.mfa_enabled
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
			&user.MfaEnabled,
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

	// Fetch permissions for each role
	for i := range roles {
		perms, err := r.getPermissionsForRole(ctx, roles[i].ID)
		if err == nil {
			roles[i].Permissions = perms
		}
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

// MFA TOTP Support Implementation
func (r *postgresRepository) UpdateMfaSecret(ctx context.Context, userID string, secret string) error {
	query := `UPDATE users SET totp_secret = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, secret, userID)
	return err
}

func (r *postgresRepository) EnableMfa(ctx context.Context, userID string) error {
	query := `UPDATE users SET mfa_enabled = true WHERE id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

func (r *postgresRepository) DisableMfa(ctx context.Context, userID string) error {
	query := `UPDATE users SET mfa_enabled = false, totp_secret = NULL WHERE id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

// Custom Roles CRUD Implementation
func (r *postgresRepository) CreateRole(ctx context.Context, name, description string, permissions []string) (*Role, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var roleID int
	err = tx.QueryRow(ctx, "INSERT INTO roles (name, description) VALUES ($1, $2) RETURNING id", name, description).Scan(&roleID)
	if err != nil {
		return nil, err
	}

	for _, permName := range permissions {
		var permID int
		err = tx.QueryRow(ctx, "SELECT id FROM permissions WHERE name = $1", permName).Scan(&permID)
		if err != nil {
			return nil, fmt.Errorf("permission %s not found: %w", permName, err)
		}
		_, err = tx.Exec(ctx, "INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)", roleID, permID)
		if err != nil {
			return nil, err
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return &Role{
		ID:          roleID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
	}, nil
}

func (r *postgresRepository) UpdateRole(ctx context.Context, roleID int, description string, permissions []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "UPDATE roles SET description = $1 WHERE id = $2", description, roleID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, "DELETE FROM role_permissions WHERE role_id = $1", roleID)
	if err != nil {
		return err
	}

	for _, permName := range permissions {
		var permID int
		err = tx.QueryRow(ctx, "SELECT id FROM permissions WHERE name = $1", permName).Scan(&permID)
		if err != nil {
			return fmt.Errorf("permission %s not found: %w", permName, err)
		}
		_, err = tx.Exec(ctx, "INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)", roleID, permID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *postgresRepository) DeleteRole(ctx context.Context, roleID int) error {
	var name string
	err := r.db.QueryRow(ctx, "SELECT name FROM roles WHERE id = $1", roleID).Scan(&name)
	if err != nil {
		return err
	}
	if name == "admin" || name == "supervisor" || name == "operator" || name == "guest" {
		return errors.New("cannot delete system default role")
	}

	_, err = r.db.Exec(ctx, "DELETE FROM roles WHERE id = $1", roleID)
	return err
}

// Active Sessions Implementation
func (r *postgresRepository) CreateSession(ctx context.Context, userID, tokenHash, ip, userAgent string, expiresAt time.Time) (string, error) {
	query := `
		INSERT INTO active_sessions (user_id, token_hash, ip_address, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`
	var sessionID string
	err := r.db.QueryRow(ctx, query, userID, tokenHash, ip, userAgent, expiresAt).Scan(&sessionID)
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

func (r *postgresRepository) IsSessionActive(ctx context.Context, sessionID string) (bool, error) {
	query := `SELECT is_active AND expires_at > NOW() FROM active_sessions WHERE id = $1`
	var active bool
	err := r.db.QueryRow(ctx, query, sessionID).Scan(&active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return active, nil
}

func (r *postgresRepository) RevokeSession(ctx context.Context, sessionID string) error {
	query := `UPDATE active_sessions SET is_active = false WHERE id = $1`
	_, err := r.db.Exec(ctx, query, sessionID)
	return err
}

func (r *postgresRepository) RevokeUserSessions(ctx context.Context, userID string) error {
	query := `UPDATE active_sessions SET is_active = false WHERE user_id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

func (r *postgresRepository) ListActiveSessions(ctx context.Context) ([]Session, error) {
	query := `
		SELECT s.id, s.user_id, u.email, COALESCE(s.ip_address, ''), COALESCE(s.user_agent, ''), s.is_active, s.created_at, s.expires_at
		FROM active_sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.is_active = true AND s.expires_at > NOW()
		ORDER BY s.created_at DESC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		err := rows.Scan(&s.ID, &s.UserID, &s.UserEmail, &s.IPAddress, &s.UserAgent, &s.IsActive, &s.CreatedAt, &s.ExpiresAt)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (r *postgresRepository) ListUserActiveSessions(ctx context.Context, userID string) ([]Session, error) {
	query := `
		SELECT s.id, s.user_id, u.email, COALESCE(s.ip_address, ''), COALESCE(s.user_agent, ''), s.is_active, s.created_at, s.expires_at
		FROM active_sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.user_id = $1 AND s.is_active = true AND s.expires_at > NOW()
		ORDER BY s.created_at DESC`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		err := rows.Scan(&s.ID, &s.UserID, &s.UserEmail, &s.IPAddress, &s.UserAgent, &s.IsActive, &s.CreatedAt, &s.ExpiresAt)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}
