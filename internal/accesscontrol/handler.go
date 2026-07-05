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

func (h *Handler) ListCardholders(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.ListCardholders(r.Context())
	if err != nil {
		h.log.Error("failed to list cardholders", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to list cardholders")
		return
	}
	h.respondWithJSON(w, http.StatusOK, list)
}

func (h *Handler) CreateCardholder(w http.ResponseWriter, r *http.Request) {
	var req CardholderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.FirstName == "" || req.LastName == "" {
		h.respondWithError(w, http.StatusBadRequest, "first_name and last_name are required")
		return
	}

	c := &Cardholder{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Company:     req.Company,
		Email:       req.Email,
		Photo:       req.Photo,
		AccessGroup: req.AccessGroup,
		Schedule:    req.Schedule,
		BadgeNumber: req.BadgeNumber,
	}

	res, err := h.service.CreateCardholder(r.Context(), c)
	if err != nil {
		h.log.Error("failed to create cardholder", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "create_cardholder", "cardholder", res.ID, "success", map[string]interface{}{"name": res.FirstName + " " + res.LastName})
	h.respondWithJSON(w, http.StatusCreated, res)
}

func (h *Handler) UpdateCardholder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid cardholder ID format")
		return
	}

	var req CardholderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.FirstName == "" || req.LastName == "" {
		h.respondWithError(w, http.StatusBadRequest, "first_name and last_name are required")
		return
	}

	c := &Cardholder{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Company:     req.Company,
		Email:       req.Email,
		Photo:       req.Photo,
		AccessGroup: req.AccessGroup,
		Schedule:    req.Schedule,
		BadgeNumber: req.BadgeNumber,
	}

	res, err := h.service.UpdateCardholder(r.Context(), id, c)
	if err != nil {
		h.log.Error("failed to update cardholder", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "update_cardholder", "cardholder", id, "success", map[string]interface{}{"name": res.FirstName + " " + res.LastName})
	h.respondWithJSON(w, http.StatusOK, res)
}

func (h *Handler) DeleteCardholder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid cardholder ID format")
		return
	}

	err := h.service.DeleteCardholder(r.Context(), id)
	if err != nil {
		h.log.Error("failed to delete cardholder", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "delete_cardholder", "cardholder", id, "success", nil)
	w.WriteHeader(http.StatusNoContent)
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

func (h *Handler) GetCardholderByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid cardholder ID format")
		return
	}

	cardholder, err := h.service.GetCardholderByID(r.Context(), id)
	if err != nil {
		h.respondWithError(w, http.StatusNotFound, "cardholder not found")
		return
	}

	h.respondWithJSON(w, http.StatusOK, cardholder)
}

func (h *Handler) GetDoorByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid door ID format")
		return
	}

	door, err := h.service.GetDoorByID(r.Context(), id)
	if err != nil {
		h.respondWithError(w, http.StatusNotFound, "door not found")
		return
	}

	h.respondWithJSON(w, http.StatusOK, door)
}

func (h *Handler) CreateDoor(w http.ResponseWriter, r *http.Request) {
	var req DoorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.SiteID == "" || !validation.IsUUID(req.SiteID) {
		h.respondWithError(w, http.StatusBadRequest, "valid name and site_id are required")
		return
	}

	d := &Door{
		SiteID:      req.SiteID,
		ZoneID:      req.ZoneID,
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
	}

	res, err := h.service.CreateDoor(r.Context(), d)
	if err != nil {
		h.log.Error("failed to create door", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "create_door", "door", res.ID, "success", map[string]interface{}{"name": res.Name})
	h.respondWithJSON(w, http.StatusCreated, res)
}

func (h *Handler) UpdateDoor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid door ID format")
		return
	}

	var req DoorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.SiteID == "" || !validation.IsUUID(req.SiteID) {
		h.respondWithError(w, http.StatusBadRequest, "valid name and site_id are required")
		return
	}

	d := &Door{
		SiteID:      req.SiteID,
		ZoneID:      req.ZoneID,
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
	}

	res, err := h.service.UpdateDoor(r.Context(), id, d)
	if err != nil {
		h.log.Error("failed to update door", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "update_door", "door", id, "success", map[string]interface{}{"name": res.Name})
	h.respondWithJSON(w, http.StatusOK, res)
}

func (h *Handler) DeleteDoor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid door ID format")
		return
	}

	err := h.service.DeleteDoor(r.Context(), id)
	if err != nil {
		h.log.Error("failed to delete door", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "delete_door", "door", id, "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListSites(w http.ResponseWriter, r *http.Request) {
	sites, err := h.service.ListSites(r.Context())
	if err != nil {
		h.log.Error("failed to list sites", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to list sites")
		return
	}
	h.respondWithJSON(w, http.StatusOK, sites)
}

func (h *Handler) GetSiteByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid site ID format")
		return
	}

	site, err := h.service.GetSiteByID(r.Context(), id)
	if err != nil {
		h.respondWithError(w, http.StatusNotFound, "site not found")
		return
	}

	h.respondWithJSON(w, http.StatusOK, site)
}

func (h *Handler) CreateSite(w http.ResponseWriter, r *http.Request) {
	var req SiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		h.respondWithError(w, http.StatusBadRequest, "name is required")
		return
	}

	s := &Site{
		Name:        req.Name,
		Description: req.Description,
	}

	res, err := h.service.CreateSite(r.Context(), s)
	if err != nil {
		h.log.Error("failed to create site", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "create_site", "site", res.ID, "success", map[string]interface{}{"name": res.Name})
	h.respondWithJSON(w, http.StatusCreated, res)
}

func (h *Handler) UpdateSite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid site ID format")
		return
	}

	var req SiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		h.respondWithError(w, http.StatusBadRequest, "name is required")
		return
	}

	s := &Site{
		Name:        req.Name,
		Description: req.Description,
	}

	res, err := h.service.UpdateSite(r.Context(), id, s)
	if err != nil {
		h.log.Error("failed to update site", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "update_site", "site", id, "success", map[string]interface{}{"name": res.Name})
	h.respondWithJSON(w, http.StatusOK, res)
}

func (h *Handler) DeleteSite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid site ID format")
		return
	}

	err := h.service.DeleteSite(r.Context(), id)
	if err != nil {
		h.log.Error("failed to delete site", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "delete_site", "site", id, "success", nil)
	w.WriteHeader(http.StatusNoContent)
}
