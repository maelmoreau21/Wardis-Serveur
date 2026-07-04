package intrusion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"wardis-server/internal/auth"
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

func (h *Handler) ListZones(w http.ResponseWriter, r *http.Request) {
	zones, err := h.service.ListZones(r.Context())
	if err != nil {
		h.log.Error("failed to list zones", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	h.respondWithJSON(w, http.StatusOK, zones)
}

func (h *Handler) ArmZone(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "id")
	if zoneID == "" || !validation.IsUUID(zoneID) {
		h.audit.Log(r.Context(), r, "arm_zone", "zone", zoneID, "failed", map[string]interface{}{"reason": "invalid zone ID format"})
		h.respondWithError(w, http.StatusBadRequest, "invalid zone ID format")
		return
	}

	err := h.service.ArmZone(r.Context(), zoneID)
	if err != nil {
		h.audit.Log(r.Context(), r, "arm_zone", "zone", zoneID, "failed", map[string]interface{}{"error": err.Error()})
		if errors.Is(err, ErrZoneNotFound) {
			h.respondWithError(w, http.StatusNotFound, "zone not found")
			return
		}
		h.log.Error("failed to arm zone", zap.String("zone_id", zoneID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to arm zone")
		return
	}

	h.audit.Log(r.Context(), r, "arm_zone", "zone", zoneID, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "zone armed successfully",
		"statut":  "arme",
	})
}

func (h *Handler) DisarmZone(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "id")
	if zoneID == "" || !validation.IsUUID(zoneID) {
		h.audit.Log(r.Context(), r, "disarm_zone", "zone", zoneID, "failed", map[string]interface{}{"reason": "invalid zone ID format"})
		h.respondWithError(w, http.StatusBadRequest, "invalid zone ID format")
		return
	}

	err := h.service.DisarmZone(r.Context(), zoneID)
	if err != nil {
		h.audit.Log(r.Context(), r, "disarm_zone", "zone", zoneID, "failed", map[string]interface{}{"error": err.Error()})
		if errors.Is(err, ErrZoneNotFound) {
			h.respondWithError(w, http.StatusNotFound, "zone not found")
			return
		}
		h.log.Error("failed to disarm zone", zap.String("zone_id", zoneID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to disarm zone")
		return
	}

	h.audit.Log(r.Context(), r, "disarm_zone", "zone", zoneID, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "zone disarmed successfully",
		"statut":  "desarme",
	})
}

func (h *Handler) ListSensors(w http.ResponseWriter, r *http.Request) {
	sensors, err := h.service.ListSensors(r.Context())
	if err != nil {
		h.log.Error("failed to list sensors", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	h.respondWithJSON(w, http.StatusOK, sensors)
}

func (h *Handler) TriggerSensor(w http.ResponseWriter, r *http.Request) {
	sensorID := chi.URLParam(r, "id")
	if sensorID == "" || !validation.IsUUID(sensorID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid sensor ID format")
		return
	}

	alarm, err := h.service.TriggerSensor(r.Context(), sensorID)
	if err != nil {
		h.audit.Log(r.Context(), r, "trigger_sensor", "sensor", sensorID, "failed", map[string]interface{}{"error": err.Error()})
		if errors.Is(err, ErrSensorNotFound) {
			h.respondWithError(w, http.StatusNotFound, "sensor not found")
			return
		}
		h.log.Error("failed to trigger sensor", zap.String("sensor_id", sensorID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to trigger sensor")
		return
	}

	if alarm != nil {
		h.audit.Log(r.Context(), r, "trigger_sensor", "sensor", sensorID, "success", map[string]interface{}{"alarm_triggered": true, "alarm_id": alarm.ID})
		h.respondWithJSON(w, http.StatusCreated, map[string]interface{}{
			"triggered": true,
			"message":   "sensor triggered and alarm active",
			"alarm":     alarm,
		})
		return
	}

	h.audit.Log(r.Context(), r, "trigger_sensor", "sensor", sensorID, "success", map[string]interface{}{"alarm_triggered": false})
	h.respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"triggered": true,
		"message":   "sensor triggered, but zone is disarmed",
		"alarm":     nil,
	})
}

func (h *Handler) ListActiveAlarms(w http.ResponseWriter, r *http.Request) {
	alarms, err := h.service.ListActiveAlarms(r.Context())
	if err != nil {
		h.log.Error("failed to list active alarms", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	h.respondWithJSON(w, http.StatusOK, alarms)
}

func (h *Handler) AcknowledgeAlarm(w http.ResponseWriter, r *http.Request) {
	alarmID := chi.URLParam(r, "id")
	if alarmID == "" || !validation.IsUUID(alarmID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid alarm ID format")
		return
	}

	claims, ok := auth.UserClaimsFromContext(r.Context())
	if !ok {
		h.respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.service.AcknowledgeAlarm(r.Context(), alarmID, claims.UserID, claims.Email, req.Reason)
	if err != nil {
		h.log.Error("failed to acknowledge alarm", zap.String("alarm_id", alarmID), zap.Error(err))
		if errors.Is(err, ErrAlarmNotFound) {
			h.respondWithError(w, http.StatusNotFound, "alarm not found")
			return
		}
		h.respondWithError(w, http.StatusInternalServerError, "failed to acknowledge alarm")
		return
	}

	h.audit.Log(r.Context(), r, "acknowledge_alarm", "alarm", alarmID, "success", map[string]interface{}{"reason": req.Reason})
	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "alarm acknowledged successfully"})
}

func (h *Handler) TransferAlarm(w http.ResponseWriter, r *http.Request) {
	alarmID := chi.URLParam(r, "id")
	if alarmID == "" || !validation.IsUUID(alarmID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid alarm ID format")
		return
	}

	claims, ok := auth.UserClaimsFromContext(r.Context())
	if !ok {
		h.respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Recipient string `json:"recipient"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Recipient == "" {
		h.respondWithError(w, http.StatusBadRequest, "recipient is required")
		return
	}

	err := h.service.TransferAlarm(r.Context(), alarmID, claims.UserID, claims.Email, req.Recipient, req.Reason)
	if err != nil {
		h.log.Error("failed to transfer alarm", zap.String("alarm_id", alarmID), zap.Error(err))
		if errors.Is(err, ErrAlarmNotFound) {
			h.respondWithError(w, http.StatusNotFound, "alarm not found")
			return
		}
		h.respondWithError(w, http.StatusInternalServerError, "failed to transfer alarm")
		return
	}

	h.audit.Log(r.Context(), r, "transfer_alarm", "alarm", alarmID, "success", map[string]interface{}{
		"recipient": req.Recipient,
		"reason":    req.Reason,
	})
	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "alarm transferred successfully"})
}

func (h *Handler) SnoozeAlarm(w http.ResponseWriter, r *http.Request) {
	alarmID := chi.URLParam(r, "id")
	if alarmID == "" || !validation.IsUUID(alarmID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid alarm ID format")
		return
	}

	claims, ok := auth.UserClaimsFromContext(r.Context())
	if !ok {
		h.respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		DurationMinutes int    `json:"duration_minutes"`
		Reason          string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DurationMinutes <= 0 {
		h.respondWithError(w, http.StatusBadRequest, "duration_minutes must be positive")
		return
	}

	err := h.service.SnoozeAlarm(r.Context(), alarmID, claims.UserID, claims.Email, req.DurationMinutes, req.Reason)
	if err != nil {
		h.log.Error("failed to snooze alarm", zap.String("alarm_id", alarmID), zap.Error(err))
		if errors.Is(err, ErrAlarmNotFound) {
			h.respondWithError(w, http.StatusNotFound, "alarm not found")
			return
		}
		h.respondWithError(w, http.StatusInternalServerError, "failed to snooze alarm")
		return
	}

	h.audit.Log(r.Context(), r, "snooze_alarm", "alarm", alarmID, "success", map[string]interface{}{
		"duration_minutes": req.DurationMinutes,
		"reason":           req.Reason,
	})
	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "alarm snoozed successfully"})
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

