package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Handler struct {
	service      Service
	log          *zap.Logger
	cookieSecure bool
}

func NewHandler(service Service, log *zap.Logger, cookieSecure bool) *Handler {
	return &Handler{
		service:      service,
		log:          log,
		cookieSecure: cookieSecure,
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		h.respondWithError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	resp, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			h.respondWithError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
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

	h.respondWithJSON(w, http.StatusOK, resp)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
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
