package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTokenExpired       = errors.New("token is expired")
	ErrInvalidToken       = errors.New("invalid token")
)

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type Service interface {
	Login(ctx context.Context, email, password string) (*LoginResponse, error)
	ValidateToken(tokenStr string) (*Claims, error)
	HashPassword(password string) (string, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	CheckEntityPermission(ctx context.Context, userID string, permissionName string, entityType string, entityID string) (bool, error)
	UpdatePassword(ctx context.Context, id string, currentPassword, newPassword string) error
	ListUsers(ctx context.Context) ([]User, error)
	CreateUser(ctx context.Context, email, password, roleName string) (*User, error)
	DeleteUser(ctx context.Context, id string) error
	ListRoles(ctx context.Context) ([]Role, error)
	ListPermissions(ctx context.Context) ([]Permission, error)
	GetEntityPermissionsForUser(ctx context.Context, userID string) ([]EntityPermission, error)
	SaveEntityPermissions(ctx context.Context, userID string, entityPermissions []EntityPermission) error
}

type service struct {
	repo      Repository
	jwtSecret []byte
	jwtExpiry time.Duration
}

func NewService(repo Repository, jwtSecret string, jwtExpiry time.Duration) Service {
	return &service{
		repo:      repo,
		jwtSecret: []byte(jwtSecret),
		jwtExpiry: jwtExpiry,
	}
}

func (s *service) Login(ctx context.Context, email, password string) (*LoginResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to fetch user during login: %w", err)
	}

	// Compare passwords
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Role details
	roleName := ""
	if user.Role != nil {
		roleName = user.Role.Name
	}

	// Create JWT token
	expiresAt := time.Now().Add(s.jwtExpiry)
	claims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   roleName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to sign token: %w", err)
	}

	return &LoginResponse{
		Token:     tokenString,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *service) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (s *service) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

func (s *service) GetUserByID(ctx context.Context, id string) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

func (s *service) CheckEntityPermission(ctx context.Context, userID string, permissionName string, entityType string, entityID string) (bool, error) {
	return s.repo.CheckEntityPermission(ctx, userID, permissionName, entityType, entityID)
}

func (s *service) UpdatePassword(ctx context.Context, id string, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword))
	if err != nil {
		return ErrInvalidCredentials
	}

	hashed, err := s.HashPassword(newPassword)
	if err != nil {
		return err
	}

	return s.repo.UpdatePassword(ctx, id, hashed)
}

func (s *service) ListUsers(ctx context.Context) ([]User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *service) CreateUser(ctx context.Context, email, password, roleName string) (*User, error) {
	hashed, err := s.HashPassword(password)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateUser(ctx, email, hashed, roleName)
}

func (s *service) DeleteUser(ctx context.Context, id string) error {
	return s.repo.DeleteUser(ctx, id)
}

func (s *service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.repo.ListRoles(ctx)
}

func (s *service) ListPermissions(ctx context.Context) ([]Permission, error) {
	return s.repo.ListPermissions(ctx)
}

func (s *service) GetEntityPermissionsForUser(ctx context.Context, userID string) ([]EntityPermission, error) {
	return s.repo.GetEntityPermissionsForUser(ctx, userID)
}

func (s *service) SaveEntityPermissions(ctx context.Context, userID string, entityPermissions []EntityPermission) error {
	return s.repo.SaveEntityPermissions(ctx, userID, entityPermissions)
}
