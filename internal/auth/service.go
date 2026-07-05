package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	ErrMfaInvalidCode     = errors.New("invalid verification code")
)

type Claims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	SessionID string `json:"session_id,omitempty"`
	jwt.RegisteredClaims
}

type MfaClaims struct {
	UserID     string `json:"user_id"`
	MfaPending bool   `json:"mfa_pending"`
	jwt.RegisteredClaims
}

type Service interface {
	Login(ctx context.Context, email, password, ip, userAgent string) (*LoginResponse, error)
	ValidateMfa(ctx context.Context, mfaToken, code, ip, userAgent string) (*LoginResponse, error)
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

	// MFA TOTP
	SetupMfa(ctx context.Context, userID string) (*MfaSetupResponse, error)
	EnableMfa(ctx context.Context, userID, code string) error
	DisableMfa(ctx context.Context, userID string) error

	// Custom Roles CRUD
	CreateRole(ctx context.Context, name, description string, permissions []string) (*Role, error)
	UpdateRole(ctx context.Context, roleID int, description string, permissions []string) error
	DeleteRole(ctx context.Context, roleID int) error

	// Active Sessions
	ListActiveSessions(ctx context.Context) ([]Session, error)
	ListUserActiveSessions(ctx context.Context, userID string) ([]Session, error)
	RevokeSession(ctx context.Context, sessionID string) error
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

func (s *service) Login(ctx context.Context, email, password, ip, userAgent string) (*LoginResponse, error) {
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

	// Handle MFA Flow
	if user.MfaEnabled {
		// Generate temporary token for MFA verification step (5 mins expiry)
		mfaExpiresAt := time.Now().Add(5 * time.Minute)
		claims := &MfaClaims{
			UserID:     user.ID,
			MfaPending: true,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(mfaExpiresAt),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Subject:   user.ID,
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		mfaTokenStr, err := token.SignedString(s.jwtSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to sign mfa token: %w", err)
		}

		return &LoginResponse{
			MfaRequired: true,
			MfaToken:    mfaTokenStr,
		}, nil
	}

	// No MFA - create database active session
	tokenHashVal := s.hashToken(email + time.Now().String())
	expiresAt := time.Now().Add(s.jwtExpiry)
	sessionID, err := s.repo.CreateSession(ctx, user.ID, tokenHashVal, ip, userAgent, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	tokenString, finalExpiry, err := s.generateToken(user, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResponse{
		Token:       tokenString,
		ExpiresAt:   finalExpiry,
		MfaRequired: false,
	}, nil
}

func (s *service) ValidateMfa(ctx context.Context, mfaToken, code, ip, userAgent string) (*LoginResponse, error) {
	// Parse temporary MFA token
	token, err := jwt.ParseWithClaims(mfaToken, &MfaClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*MfaClaims)
	if !ok || !token.Valid || !claims.MfaPending {
		return nil, ErrInvalidToken
	}

	// Fetch user details
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user for MFA validation: %w", err)
	}

	// Validate TOTP code
	if !ValidateTOTP(user.TotpSecret, code) {
		return nil, ErrMfaInvalidCode
	}

	// Create database active session
	tokenHashVal := s.hashToken(user.Email + time.Now().String())
	expiresAt := time.Now().Add(s.jwtExpiry)
	sessionID, err := s.repo.CreateSession(ctx, user.ID, tokenHashVal, ip, userAgent, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	tokenString, finalExpiry, err := s.generateToken(user, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResponse{
		Token:       tokenString,
		ExpiresAt:   finalExpiry,
		MfaRequired: false,
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

	// Enforce DB-level session status check for remote invalidation
	if claims.SessionID != "" {
		active, err := s.repo.IsSessionActive(context.Background(), claims.SessionID)
		if err != nil || !active {
			return nil, errors.New("session is invalid or has been remotely revoked")
		}
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

// MFA Setup & Configuration
func (s *service) SetupMfa(ctx context.Context, userID string) (*MfaSetupResponse, error) {
	secret, err := GenerateSecret()
	if err != nil {
		return nil, err
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	err = s.repo.UpdateMfaSecret(ctx, userID, secret)
	if err != nil {
		return nil, err
	}

	uri := GetProvisioningURI(user.Email, secret)
	return &MfaSetupResponse{
		Secret: secret,
		QRCode: uri,
	}, nil
}

func (s *service) EnableMfa(ctx context.Context, userID, code string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if user.TotpSecret == "" {
		return errors.New("MFA has not been setup first")
	}

	if !ValidateTOTP(user.TotpSecret, code) {
		return ErrMfaInvalidCode
	}

	return s.repo.EnableMfa(ctx, userID)
}

func (s *service) DisableMfa(ctx context.Context, userID string) error {
	return s.repo.DisableMfa(ctx, userID)
}

// Custom Roles CRUD
func (s *service) CreateRole(ctx context.Context, name, description string, permissions []string) (*Role, error) {
	return s.repo.CreateRole(ctx, name, description, permissions)
}

func (s *service) UpdateRole(ctx context.Context, roleID int, description string, permissions []string) error {
	return s.repo.UpdateRole(ctx, roleID, description, permissions)
}

func (s *service) DeleteRole(ctx context.Context, roleID int) error {
	return s.repo.DeleteRole(ctx, roleID)
}

// Active Sessions
func (s *service) ListActiveSessions(ctx context.Context) ([]Session, error) {
	return s.repo.ListActiveSessions(ctx)
}

func (s *service) ListUserActiveSessions(ctx context.Context, userID string) ([]Session, error) {
	return s.repo.ListUserActiveSessions(ctx, userID)
}

func (s *service) RevokeSession(ctx context.Context, sessionID string) error {
	return s.repo.RevokeSession(ctx, sessionID)
}

// Helper methods
func (s *service) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *service) generateToken(user *User, sessionID string) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.jwtExpiry)
	roleName := "user"
	if user.Role != nil {
		roleName = user.Role.Name
	}

	claims := &Claims{
		UserID:    user.ID,
		Email:     user.Email,
		Role:      roleName,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}
