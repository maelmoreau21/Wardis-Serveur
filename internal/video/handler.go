package video

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

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

func (h *Handler) ListCameras(w http.ResponseWriter, r *http.Request) {
	cameras, err := h.service.ListCameras(r.Context())
	if err != nil {
		h.log.Error("failed to list cameras", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	h.respondWithJSON(w, http.StatusOK, cameras)
}

func (h *Handler) GetCameraByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	cam, err := h.service.GetCameraByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrCameraNotFound) {
			h.respondWithError(w, http.StatusNotFound, "camera not found")
			return
		}
		h.log.Error("failed to get camera by id", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.respondWithJSON(w, http.StatusOK, cam)
}

func (h *Handler) CreateCamera(w http.ResponseWriter, r *http.Request) {
	var req CreateCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Strict validations
	if !validation.IsName(req.Nom, 1, 100) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera name (must be 1-100 characters)")
		return
	}
	if !validation.IsRTSPURL(req.URLRTSP) {
		h.respondWithError(w, http.StatusBadRequest, "invalid RTSP URL format")
		return
	}
	if req.SiteID != nil && *req.SiteID != "" && !validation.IsUUID(*req.SiteID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid site ID format")
		return
	}
	if req.Statut != "active" && req.Statut != "inactive" {
		h.respondWithError(w, http.StatusBadRequest, "status must be 'active' or 'inactive'")
		return
	}

	cam, err := h.service.CreateCamera(r.Context(), req)
	if err != nil {
		h.audit.Log(r.Context(), r, "create_camera", "camera", "", "failed", map[string]interface{}{"name": req.Nom, "error": err.Error()})
		h.log.Error("failed to create camera", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to create camera")
		return
	}

	h.audit.Log(r.Context(), r, "create_camera", "camera", cam.ID, "success", map[string]interface{}{"name": cam.Nom})
	h.respondWithJSON(w, http.StatusCreated, cam)
}

func (h *Handler) UpdateCamera(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	var req UpdateCameraRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Strict validations
	if !validation.IsName(req.Nom, 1, 100) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera name (must be 1-100 characters)")
		return
	}
	if !validation.IsRTSPURL(req.URLRTSP) {
		h.respondWithError(w, http.StatusBadRequest, "invalid RTSP URL format")
		return
	}
	if req.SiteID != nil && *req.SiteID != "" && !validation.IsUUID(*req.SiteID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid site ID format")
		return
	}
	if req.Statut != "active" && req.Statut != "inactive" {
		h.respondWithError(w, http.StatusBadRequest, "status must be 'active' or 'inactive'")
		return
	}

	cam, err := h.service.UpdateCamera(r.Context(), id, req)
	if err != nil {
		h.audit.Log(r.Context(), r, "update_camera", "camera", id, "failed", map[string]interface{}{"name": req.Nom, "error": err.Error()})
		if errors.Is(err, ErrCameraNotFound) {
			h.respondWithError(w, http.StatusNotFound, "camera not found")
			return
		}
		h.log.Error("failed to update camera", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to update camera")
		return
	}

	h.audit.Log(r.Context(), r, "update_camera", "camera", id, "success", map[string]interface{}{"name": cam.Nom})
	h.respondWithJSON(w, http.StatusOK, cam)
}

func (h *Handler) DeleteCamera(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	err := h.service.DeleteCamera(r.Context(), id)
	if err != nil {
		h.audit.Log(r.Context(), r, "delete_camera", "camera", id, "failed", map[string]interface{}{"error": err.Error()})
		if errors.Is(err, ErrCameraNotFound) {
			h.respondWithError(w, http.StatusNotFound, "camera not found")
			return
		}
		h.log.Error("failed to delete camera", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to delete camera")
		return
	}

	h.audit.Log(r.Context(), r, "delete_camera", "camera", id, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "camera deleted successfully",
	})
}

func (h *Handler) GenerateStreamToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	token, err := h.service.GenerateStreamToken(r.Context(), id)
	if err != nil {
		h.audit.Log(r.Context(), r, "generate_stream_token", "camera", id, "failed", map[string]interface{}{"error": err.Error()})
		if errors.Is(err, ErrCameraNotFound) {
			h.respondWithError(w, http.StatusNotFound, "camera not found")
			return
		}
		h.log.Error("failed to generate stream token", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to generate stream token")
		return
	}

	h.audit.Log(r.Context(), r, "generate_stream_token", "camera", id, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"token": token,
	})
}

func (h *Handler) ListActiveStreams(w http.ResponseWriter, r *http.Request) {
	streams, err := h.service.ListActiveStreams(r.Context())
	if err != nil {
		h.log.Error("failed to list active streams", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to list active streams")
		return
	}
	h.respondWithJSON(w, http.StatusOK, streams)
}

func (h *Handler) MediaMtxAuth(w http.ResponseWriter, r *http.Request) {
	var req MediaMtxAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("failed to decode MediaMTX auth payload", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Strictly validate input parameters
	if req.Action != "publish" && req.Action != "read" {
		h.log.Warn("MediaMTX auth rejected: invalid action", zap.String("action", req.Action))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !validation.IsUUID(req.Path) {
		h.log.Warn("MediaMTX auth rejected: invalid path/camera ID format", zap.String("path", req.Path))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Logging authorization request for debugging
	h.log.Debug("MediaMTX authentication request received",
		zap.String("action", req.Action),
		zap.String("path", req.Path),
		zap.String("query", req.Query),
		zap.String("ip", req.IP),
	)

	// If MediaMTX is trying to publish/connect to a camera source, we verify it exists in the DB
	if req.Action == "publish" {
		_, err := h.service.GetCameraByID(r.Context(), req.Path)
		if err != nil {
			if errors.Is(err, ErrCameraNotFound) {
				h.log.Warn("MediaMTX publish rejected: camera ID not found in database", zap.String("path", req.Path))
				w.WriteHeader(http.StatusNotFound)
				return
			}
			h.log.Error("MediaMTX publish auth database lookup failed", zap.String("path", req.Path), zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Camera exists and is allowed to publish
		w.WriteHeader(http.StatusOK)
		return
	}

	// If a client is trying to read/play the stream, they must present a valid temporary token
	if req.Action == "read" {
		vals, err := url.ParseQuery(req.Query)
		if err != nil {
			h.log.Warn("MediaMTX read rejected: failed to parse query parameters", zap.String("query", req.Query))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		tokenStr := vals.Get("token")
		if tokenStr == "" {
			tokenStr = vals.Get("jwt")
		}

		if tokenStr == "" {
			h.log.Warn("MediaMTX read rejected: missing stream token in query", zap.String("path", req.Path))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Validate that the token matches the requested camera path (which is the camera UUID)
		if err := h.service.ValidateStreamToken(tokenStr, req.Path); err != nil {
			h.log.Warn("MediaMTX read rejected: token validation failed", zap.String("path", req.Path), zap.Error(err))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Validation successful
		w.WriteHeader(http.StatusOK)
		return
	}

	// Default allow for other API operations
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) DiscoverCameras(w http.ResponseWriter, r *http.Request) {
	var req DiscoverCamerasRequest
	req.Timeout = 5

	if r.Body != http.NoBody && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.respondWithError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if req.Timeout <= 0 || req.Timeout > 30 {
		req.Timeout = 5
	}

	discovered, err := h.service.DiscoverCameras(r.Context(), req.Username, req.Password, time.Duration(req.Timeout)*time.Second)
	if err != nil {
		h.audit.Log(r.Context(), r, "discover_cameras", "camera", "", "failed", map[string]interface{}{"error": err.Error()})
		h.log.Error("failed to discover cameras", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to discover cameras: "+err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "discover_cameras", "camera", "", "success", map[string]interface{}{"count": len(discovered)})
	h.respondWithJSON(w, http.StatusOK, discovered)
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

func (h *Handler) SyncRecording(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	// Parse Multipart Form (max size 50MB)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		h.log.Error("failed to parse multipart form", zap.Error(err))
		h.respondWithError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	startTimeStr := r.FormValue("start_time")
	endTimeStr := r.FormValue("end_time")

	if startTimeStr == "" || endTimeStr == "" {
		h.respondWithError(w, http.StatusBadRequest, "missing start_time or end_time")
		return
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid start_time format (must be RFC3339)")
		return
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid end_time format (must be RFC3339)")
		return
	}

	if !startTime.Before(endTime) {
		h.respondWithError(w, http.StatusBadRequest, "start_time must be before end_time")
		return
	}

	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "missing video file in form field 'video'")
		return
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		h.log.Error("failed to read uploaded file", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to read uploaded file")
		return
	}

	filename := fileHeader.Filename

	rec, err := h.service.SyncRecording(r.Context(), id, startTime, endTime, fileData, filename)
	if err != nil {
		h.audit.Log(r.Context(), r, "sync_recording", "camera", id, "failed", map[string]interface{}{"error": err.Error()})
		h.log.Error("failed to sync video recording", zap.String("camera_id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to sync recording: "+err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "sync_recording", "camera", id, "success", map[string]interface{}{
		"start_time": startTimeStr,
		"end_time":   endTimeStr,
		"reconciled": rec != nil,
	})

	if rec != nil {
		h.respondWithJSON(w, http.StatusCreated, rec)
	} else {
		h.respondWithJSON(w, http.StatusOK, map[string]string{
			"message": "recording received but fully redundant/skipped",
		})
	}
}
