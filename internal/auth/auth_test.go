package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"wardis-server/internal/auth"
)

type mockRepository struct {
	users map[string]*auth.User
}

func (m *mockRepository) GetUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, auth.ErrUserNotFound
}

func (m *mockRepository) GetUserByID(ctx context.Context, id string) (*auth.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return u, nil
}

func (m *mockRepository) CreateUser(ctx context.Context, email, passwordHash, roleName string) (*auth.User, error) {
	u := &auth.User{
		ID:           "test-uuid-1",
		Email:        email,
		PasswordHash: passwordHash,
		Role: &auth.Role{
			Name: roleName,
			Permissions: []auth.Permission{
				{Name: "auth.me"},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.users[u.ID] = u
	return u, nil
}

func (m *mockRepository) CheckEntityPermission(ctx context.Context, userID string, permissionName string, entityType string, entityID string) (bool, error) {
	if userID == "forbidden-user" {
		return false, nil
	}
	if permissionName == "error-permission" {
		return false, errors.New("mock db error")
	}
	return true, nil
}

func TestAuthService(t *testing.T) {
	ctx := context.Background()
	secret := "test-secret-key-12345"
	expiry := 5 * time.Minute

	// Hash a password for seeding
	password := "correctpassword"
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	hashedPassword := string(hashedBytes)

	// Seed mock DB
	repo := &mockRepository{
		users: map[string]*auth.User{
			"test-uuid-1": {
				ID:           "test-uuid-1",
				Email:        "user@example.com",
				PasswordHash: hashedPassword,
				Role: &auth.Role{
					Name: "user",
					Permissions: []auth.Permission{
						{Name: "auth.me"},
					},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	service := auth.NewService(repo, secret, expiry)

	t.Run("Successful Login", func(t *testing.T) {
		resp, err := service.Login(ctx, "user@example.com", "correctpassword")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if resp.Token == "" {
			t.Error("expected non-empty token")
		}

		if resp.ExpiresAt.Before(time.Now()) {
			t.Error("expected token expiration to be in the future")
		}

		// Validate the token
		claims, err := service.ValidateToken(resp.Token)
		if err != nil {
			t.Fatalf("expected token validation to succeed, got: %v", err)
		}

		if claims.Email != "user@example.com" {
			t.Errorf("expected email 'user@example.com', got '%s'", claims.Email)
		}

		if claims.Role != "user" {
			t.Errorf("expected role 'user', got '%s'", claims.Role)
		}

		if claims.UserID != "test-uuid-1" {
			t.Errorf("expected user ID 'test-uuid-1', got '%s'", claims.UserID)
		}
	})

	t.Run("Failed Login - Wrong Password", func(t *testing.T) {
		_, err := service.Login(ctx, "user@example.com", "wrongpassword")
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Errorf("expected ErrInvalidCredentials, got: %v", err)
		}
	})

	t.Run("Failed Login - Non-existent User", func(t *testing.T) {
		_, err := service.Login(ctx, "nonexistent@example.com", "password")
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Errorf("expected ErrInvalidCredentials, got: %v", err)
		}
	})

	t.Run("Invalid Token Verification", func(t *testing.T) {
		_, err := service.ValidateToken("invalid.jwt.token")
		if !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("expected ErrInvalidToken, got: %v", err)
		}
	})
}
