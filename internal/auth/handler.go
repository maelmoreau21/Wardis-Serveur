package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type AuditLogger interface {
	Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{})
}

type Handler struct {
	service      Service
	log          *zap.Logger
	cookieSecure bool
	audit        AuditLogger
}

func NewHandler(service Service, log *zap.Logger, cookieSecure bool, audit AuditLogger) *Handler {
	return &Handler{
		service:      service,
		log:          log,
		cookieSecure: cookieSecure,
		audit:        audit,
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Strict username/email and password validation
	if req.Email == "" || len(req.Email) < 3 || len(req.Email) > 254 {
		h.audit.Log(r.Context(), r, "login", "auth", "", "failed", map[string]interface{}{"email": req.Email, "reason": "invalid identifier format"})
		h.respondWithError(w, http.StatusBadRequest, "invalid identifier format")
		return
	}
	if req.Password == "" || len(req.Password) < 4 || len(req.Password) > 100 {
		h.audit.Log(r.Context(), r, "login", "auth", "", "failed", map[string]interface{}{"email": req.Email, "reason": "invalid password length"})
		h.respondWithError(w, http.StatusBadRequest, "invalid password length")
		return
	}

	resp, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			h.audit.Log(r.Context(), r, "login", "auth", "", "failed", map[string]interface{}{"email": req.Email, "reason": "invalid credentials"})
			h.respondWithError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		h.audit.Log(r.Context(), r, "login", "auth", "", "failed", map[string]interface{}{"email": req.Email, "reason": "internal server error"})
		h.log.Error("login error", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Set HttpOnly cookie for the token
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    resp.Token,
		Expires:  resp.ExpiresAt,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	h.audit.Log(r.Context(), r, "login", "auth", req.Email, "success", nil)
	h.respondWithJSON(w, http.StatusOK, resp)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// Log logout before clearing context/cookie if claims exist
	var userEmail string
	if claims, ok := UserClaimsFromContext(r.Context()); ok {
		userEmail = claims.Email
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Expires:  time.Unix(0, 0),
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	h.audit.Log(r.Context(), r, "logout", "auth", userEmail, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "successfully logged out"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		h.respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.service.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			h.respondWithError(w, http.StatusNotFound, "user not found")
			return
		}
		h.log.Error("me error fetching user", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	roleName := "user"
	var permissions []string
	if user.Role != nil {
		roleName = user.Role.Name
		for _, p := range user.Role.Permissions {
			permissions = append(permissions, p.Name)
		}
	}

	resp := MeResponse{
		ID:          user.ID,
		Email:       user.Email,
		Role:        roleName,
		Permissions: permissions,
	}

	h.respondWithJSON(w, http.StatusOK, resp)
}

func (h *Handler) respondWithError(w http.ResponseWriter, code int, message string) {
	h.respondWithJSON(w, code, map[string]string{"error": message})
}

func (h *Handler) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to encode response"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func (h *Handler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		h.respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req UpdatePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.NewPassword) < 4 {
		h.respondWithError(w, http.StatusBadRequest, "new password must be at least 4 characters")
		return
	}

	err := h.service.UpdatePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			h.respondWithError(w, http.StatusBadRequest, "invalid current password")
			return
		}
		h.log.Error("failed to update password", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "password updated successfully"})
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.ListUsers(r.Context())
	if err != nil {
		h.log.Error("failed to list users", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	h.respondWithJSON(w, http.StatusOK, users)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Email) < 3 || len(req.Password) < 4 {
		h.respondWithError(w, http.StatusBadRequest, "invalid email or password length")
		return
	}

	user, err := h.service.CreateUser(r.Context(), req.Email, req.Password, req.RoleName)
	if err != nil {
		h.log.Error("failed to create user", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	h.respondWithJSON(w, http.StatusCreated, user)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		h.respondWithError(w, http.StatusBadRequest, "missing user id")
		return
	}

	err := h.service.DeleteUser(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to delete user", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "user deleted successfully"})
}

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.service.ListRoles(r.Context())
	if err != nil {
		h.log.Error("failed to list roles", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to list roles")
		return
	}
	h.respondWithJSON(w, http.StatusOK, roles)
}

func (h *Handler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.service.ListPermissions(r.Context())
	if err != nil {
		h.log.Error("failed to list permissions", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to list permissions")
		return
	}
	h.respondWithJSON(w, http.StatusOK, perms)
}

func (h *Handler) GetEntityPermissionsForUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		h.respondWithError(w, http.StatusBadRequest, "missing user id")
		return
	}

	perms, err := h.service.GetEntityPermissionsForUser(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to get entity permissions", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to get entity permissions")
		return
	}
	h.respondWithJSON(w, http.StatusOK, perms)
}

func (h *Handler) SaveEntityPermissions(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		h.respondWithError(w, http.StatusBadRequest, "missing user id")
		return
	}

	var req SaveEntityPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.service.SaveEntityPermissions(r.Context(), userID, req.EntityPermissions)
	if err != nil {
		h.log.Error("failed to save entity permissions", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to save entity permissions")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "permissions saved successfully"})
}
