package accesscontrol

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Handler struct {
	service Service
	log     *zap.Logger
}

func NewHandler(service Service, log *zap.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
	}
}

func (h *Handler) ListDoors(w http.ResponseWriter, r *http.Request) {
	doors, err := h.service.ListDoors(r.Context())
	if err != nil {
		h.log.Error("failed to list doors", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	h.respondWithJSON(w, http.StatusOK, doors)
}

func (h *Handler) AssignBadge(w http.ResponseWriter, r *http.Request) {
	var req AssignBadgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BadgeNumber == "" || req.UserID == "" {
		h.respondWithError(w, http.StatusBadRequest, "badge_number and user_id are required")
		return
	}

	badge, err := h.service.AssignBadge(r.Context(), req.BadgeNumber, req.UserID)
	if err != nil {
		h.log.Error("failed to assign badge", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to assign badge")
		return
	}

	h.respondWithJSON(w, http.StatusOK, badge)
}

func (h *Handler) OpenDoor(w http.ResponseWriter, r *http.Request) {
	doorID := chi.URLParam(r, "id")
	if doorID == "" {
		h.respondWithError(w, http.StatusBadRequest, "door ID is required")
		return
	}

	err := h.service.OpenDoor(r.Context(), doorID)
	if err != nil {
		h.log.Error("failed to open door", zap.String("door_id", doorID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to open door")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "door opened successfully",
		"status":  "open",
	})
}

func (h *Handler) CloseDoor(w http.ResponseWriter, r *http.Request) {
	doorID := chi.URLParam(r, "id")
	if doorID == "" {
		h.respondWithError(w, http.StatusBadRequest, "door ID is required")
		return
	}

	err := h.service.CloseDoor(r.Context(), doorID)
	if err != nil {
		h.log.Error("failed to close door", zap.String("door_id", doorID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to close door")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "door closed successfully",
		"status":  "closed",
	})
}

func (h *Handler) SwipeBadge(w http.ResponseWriter, r *http.Request) {
	doorID := chi.URLParam(r, "id")
	if doorID == "" {
		h.respondWithError(w, http.StatusBadRequest, "door ID is required")
		return
	}

	var req SwipeBadgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BadgeNumber == "" {
		h.respondWithError(w, http.StatusBadRequest, "badge_number is required")
		return
	}

	resp, err := h.service.SwipeBadge(r.Context(), doorID, req.BadgeNumber)
	if err != nil {
		h.log.Error("failed to swipe badge", zap.String("door_id", doorID), zap.String("badge_number", req.BadgeNumber), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	statusCode := http.StatusOK
	if !resp.AccessGranted {
		statusCode = http.StatusForbidden
	}

	h.respondWithJSON(w, statusCode, resp)
}

func (h *Handler) ListAccessLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := h.service.ListAccessLogs(r.Context())
	if err != nil {
		h.log.Error("failed to list access logs", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	h.respondWithJSON(w, http.StatusOK, logs)
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
