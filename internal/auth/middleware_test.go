package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"wardis-server/internal/auth"
)

func generateTestToken(t *testing.T, userID, email, role, secret string) string {
	claims := &auth.Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return tokenString
}

func TestRequireEntityPermission(t *testing.T) {
	secret := "test-secret-key-12345"
	repo := &mockRepository{
		users: make(map[string]*auth.User),
	}

	authService := auth.NewService(repo, secret, 5*time.Minute)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	tests := []struct {
		name           string
		userID         string
		permissionName string
		entityID       string
		idParamName    string
		setupClaims    bool
		expectedStatus int
	}{
		{
			name:           "Valid Access - User authorized",
			userID:         "authorized-user",
			permissionName: "camera:live",
			entityID:       "123e4567-e89b-12d3-a456-426614174000",
			idParamName:    "id",
			setupClaims:    true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Forbidden Access - User denied",
			userID:         "forbidden-user",
			permissionName: "camera:live",
			entityID:       "123e4567-e89b-12d3-a456-426614174000",
			idParamName:    "id",
			setupClaims:    true,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Unauthorized - Missing token",
			userID:         "",
			permissionName: "camera:live",
			entityID:       "123e4567-e89b-12d3-a456-426614174000",
			idParamName:    "id",
			setupClaims:    false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Bad Request - Missing path parameter name or entity ID",
			userID:         "authorized-user",
			permissionName: "camera:live",
			entityID:       "",
			idParamName:    "id",
			setupClaims:    true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Internal Server Error - DB error",
			userID:         "authorized-user",
			permissionName: "error-permission",
			entityID:       "123e4567-e89b-12d3-a456-426614174000",
			idParamName:    "id",
			setupClaims:    true,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := chi.NewRouter()
			router.Use(auth.AuthMiddleware(authService))
			
			mw := auth.RequireEntityPermission(authService, tt.permissionName, "camera", tt.idParamName)
			
			router.Handle("/cameras/{"+tt.idParamName+"}", mw(okHandler))
			router.Handle("/cameras/", mw(okHandler))

			var url string
			if tt.entityID != "" {
				url = "/cameras/" + tt.entityID
			} else {
				url = "/cameras/"
			}

			req := httptest.NewRequest("GET", url, nil)

			if tt.setupClaims {
				token := generateTestToken(t, tt.userID, "test@example.com", "user", secret)
				req.Header.Set("Authorization", "Bearer "+token)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d. Response: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}
