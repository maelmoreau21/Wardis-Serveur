package auth

import (
	"time"
)

type Permission struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Role struct {
	ID          int          `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	RoleID       *int      `json:"role_id,omitempty"`
	Role         *Role     `json:"role,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type MeResponse struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

type EntityPermission struct {
	EntityID       string `json:"entity_id"`
	EntityType     string `json:"entity_type"`
	PermissionName string `json:"permission_name"`
	Allowed        bool   `json:"allowed"`
}

type UpdatePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	RoleName string `json:"role_name"`
}

type SaveEntityPermissionsRequest struct {
	EntityPermissions []EntityPermission `json:"entity_permissions"`
}

