package intrusion

import (
	"encoding/json"
	"errors"
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
	if zoneID == "" {
		h.respondWithError(w, http.StatusBadRequest, "zone ID is required")
		return
	}

	err := h.service.ArmZone(r.Context(), zoneID)
	if err != nil {
		if errors.Is(err, ErrZoneNotFound) {
			h.respondWithError(w, http.StatusNotFound, "zone not found")
			return
		}
		h.log.Error("failed to arm zone", zap.String("zone_id", zoneID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to arm zone")
		return
	}

	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "zone armed successfully",
		"statut":  "arme",
	})
}

func (h *Handler) DisarmZone(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "id")
	if zoneID == "" {
		h.respondWithError(w, http.StatusBadRequest, "zone ID is required")
		return
	}

	err := h.service.DisarmZone(r.Context(), zoneID)
	if err != nil {
		if errors.Is(err, ErrZoneNotFound) {
			h.respondWithError(w, http.StatusNotFound, "zone not found")
			return
		}
		h.log.Error("failed to disarm zone", zap.String("zone_id", zoneID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to disarm zone")
		return
	}

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
	if sensorID == "" {
		h.respondWithError(w, http.StatusBadRequest, "sensor ID is required")
		return
	}

	alarm, err := h.service.TriggerSensor(r.Context(), sensorID)
	if err != nil {
		if errors.Is(err, ErrSensorNotFound) {
			h.respondWithError(w, http.StatusNotFound, "sensor not found")
			return
		}
		h.log.Error("failed to trigger sensor", zap.String("sensor_id", sensorID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to trigger sensor")
		return
	}

	if alarm != nil {
		h.respondWithJSON(w, http.StatusCreated, map[string]interface{}{
			"triggered": true,
			"message":   "sensor triggered and alarm active",
			"alarm":     alarm,
		})
		return
	}

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
