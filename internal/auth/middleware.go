package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	claimsContextKey contextKey = "user_claims"
)

func AuthMiddleware(authService Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := extractToken(r)
			if tokenString == "" {
				http.Error(w, `{"error": "unauthorized: missing token"}`, http.StatusUnauthorized)
				return
			}

			claims, err := authService.ValidateToken(tokenString)
			if err != nil {
				http.Error(w, fmtJSONError(err.Error()), http.StatusUnauthorized)
				return
			}

			// Store claims in context
			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	// 1. Try Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	// 2. Try cookie
	cookie, err := r.Cookie("token")
	if err == nil {
		return cookie.Value
	}

	// 3. Try query parameter (useful for SSE/EventSource)
	if tokenParam := r.URL.Query().Get("token"); tokenParam != "" {
		return tokenParam
	}

	return ""
}

func UserClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(*Claims)
	return claims, ok
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	claims, ok := UserClaimsFromContext(ctx)
	if !ok {
		return "", false
	}
	return claims.UserID, true
}

func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := UserClaimsFromContext(r.Context())
			if !ok || claims.Role != role {
				http.Error(w, `{"error": "forbidden: insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func fmtJSONError(msg string) string {
	// Simple JSON error formatting
	return `{"error": "` + msg + `"}`
}
