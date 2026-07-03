package accesscontrol

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"wardis-server/internal/validation"
)

type AuditLogger interface {
	Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{})
}

type Handler struct {
	service Service
	log     *zap.Logger
	audit   AuditLogger
}

func NewHandler(service Service, log *zap.Logger, audit AuditLogger) *Handler {
	return &Handler{
		service: service,
		log:     log,
		audit:   audit,
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

	// Validation
	if !validation.IsAlphanumeric(req.BadgeNumber, 3, 50) {
		h.audit.Log(r.Context(), r, "assign_badge", "badge", req.BadgeNumber, "failed", map[string]interface{}{"reason": "invalid badge_number format or length"})
		h.respondWithError(w, http.StatusBadRequest, "invalid badge_number format or length")
		return
	}
	if !validation.IsUUID(req.UserID) {
		h.audit.Log(r.Context(), r, "assign_badge", "badge", req.BadgeNumber, "failed", map[string]interface{}{"reason": "invalid user_id UUID format"})
		h.respondWithError(w, http.StatusBadRequest, "invalid user_id UUID format")
		return
	}

	badge, err := h.service.AssignBadge(r.Context(), req.BadgeNumber, req.UserID)
	if err != nil {
		h.audit.Log(r.Context(), r, "assign_badge", "badge", req.BadgeNumber, "failed", map[string]interface{}{"error": err.Error()})
		h.log.Error("failed to assign badge", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to assign badge")
		return
	}

	h.audit.Log(r.Context(), r, "assign_badge", "badge", req.BadgeNumber, "success", map[string]interface{}{"user_id": req.UserID})
	h.respondWithJSON(w, http.StatusOK, badge)
}

func (h *Handler) OpenDoor(w http.ResponseWriter, r *http.Request) {
	doorID := chi.URLParam(r, "id")
	if doorID == "" || !validation.IsUUID(doorID) {
		h.audit.Log(r.Context(), r, "open_door", "door", doorID, "failed", map[string]interface{}{"reason": "invalid door ID format"})
		h.respondWithError(w, http.StatusBadRequest, "invalid door ID format")
		return
	}

	err := h.service.OpenDoor(r.Context(), doorID)
	if err != nil {
		h.audit.Log(r.Context(), r, "open_door", "door", doorID, "failed", map[string]interface{}{"error": err.Error()})
		h.log.Error("failed to open door", zap.String("door_id", doorID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to open door")
		return
	}

	h.audit.Log(r.Context(), r, "open_door", "door", doorID, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "door opened successfully",
		"status":  "open",
	})
}

func (h *Handler) CloseDoor(w http.ResponseWriter, r *http.Request) {
	doorID := chi.URLParam(r, "id")
	if doorID == "" || !validation.IsUUID(doorID) {
		h.audit.Log(r.Context(), r, "close_door", "door", doorID, "failed", map[string]interface{}{"reason": "invalid door ID format"})
		h.respondWithError(w, http.StatusBadRequest, "invalid door ID format")
		return
	}

	err := h.service.CloseDoor(r.Context(), doorID)
	if err != nil {
		h.audit.Log(r.Context(), r, "close_door", "door", doorID, "failed", map[string]interface{}{"error": err.Error()})
		h.log.Error("failed to close door", zap.String("door_id", doorID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to close door")
		return
	}

	h.audit.Log(r.Context(), r, "close_door", "door", doorID, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "door closed successfully",
		"status":  "closed",
	})
}

func (h *Handler) SwipeBadge(w http.ResponseWriter, r *http.Request) {
	doorID := chi.URLParam(r, "id")
	if doorID == "" || !validation.IsUUID(doorID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid door ID format")
		return
	}

	var req SwipeBadgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validation
	if !validation.IsAlphanumeric(req.BadgeNumber, 3, 50) {
		h.audit.Log(r.Context(), r, "swipe_badge", "door", doorID, "failed", map[string]interface{}{"reason": "invalid badge_number format or length"})
		h.respondWithError(w, http.StatusBadRequest, "invalid badge_number format or length")
		return
	}

	resp, err := h.service.SwipeBadge(r.Context(), doorID, req.BadgeNumber)
	if err != nil {
		h.audit.Log(r.Context(), r, "swipe_badge", "door", doorID, "failed", map[string]interface{}{"badge_number": req.BadgeNumber, "error": err.Error()})
		h.log.Error("failed to swipe badge", zap.String("door_id", doorID), zap.String("badge_number", req.BadgeNumber), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	statusCode := http.StatusOK
	statusStr := "success"
	if !resp.AccessGranted {
		statusCode = http.StatusForbidden
		statusStr = "failed"
	}

	h.audit.Log(r.Context(), r, "swipe_badge", "door", doorID, statusStr, map[string]interface{}{
		"badge_number":   req.BadgeNumber,
		"access_granted": resp.AccessGranted,
		"message":        resp.Message,
	})

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
