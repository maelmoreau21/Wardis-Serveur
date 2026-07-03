package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"

	"wardis-server/internal/validation"
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

	// Strict email and password validation
	if !validation.IsEmail(req.Email) {
		h.audit.Log(r.Context(), r, "login", "auth", "", "failed", map[string]interface{}{"email": req.Email, "reason": "invalid email format"})
		h.respondWithError(w, http.StatusBadRequest, "invalid email format")
		return
	}
	if req.Password == "" || len(req.Password) < 6 || len(req.Password) > 100 {
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
