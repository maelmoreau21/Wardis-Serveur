package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

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
	if req.URLRTSP == "" && req.MainStreamURL != "" {
		req.URLRTSP = req.MainStreamURL
	}
	if req.MainStreamURL == "" && req.URLRTSP != "" {
		req.MainStreamURL = req.URLRTSP
	}

	if !validation.IsRTSPURL(req.URLRTSP) {
		h.respondWithError(w, http.StatusBadRequest, "invalid RTSP URL format")
		return
	}
	if req.MainStreamURL != "" && !validation.IsRTSPURL(req.MainStreamURL) {
		h.respondWithError(w, http.StatusBadRequest, "invalid main stream RTSP URL format")
		return
	}
	if req.SubStreamURL != "" && !validation.IsRTSPURL(req.SubStreamURL) {
		h.respondWithError(w, http.StatusBadRequest, "invalid sub stream RTSP URL format")
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
	if req.URLRTSP == "" && req.MainStreamURL != "" {
		req.URLRTSP = req.MainStreamURL
	}
	if req.MainStreamURL == "" && req.URLRTSP != "" {
		req.MainStreamURL = req.URLRTSP
	}

	if !validation.IsRTSPURL(req.URLRTSP) {
		h.respondWithError(w, http.StatusBadRequest, "invalid RTSP URL format")
		return
	}
	if req.MainStreamURL != "" && !validation.IsRTSPURL(req.MainStreamURL) {
		h.respondWithError(w, http.StatusBadRequest, "invalid main stream RTSP URL format")
		return
	}
	if req.SubStreamURL != "" && !validation.IsRTSPURL(req.SubStreamURL) {
		h.respondWithError(w, http.StatusBadRequest, "invalid sub stream RTSP URL format")
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

func (h *Handler) ConfigureWHEPStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	err := h.service.ConfigureWHEPStream(r.Context(), id)
	if err != nil {
		h.respondWithError(w, http.StatusInternalServerError, "failed to configure WHEP stream: "+err.Error())
		return
	}

	// Generate stream token for access authorization
	token, err := h.service.GenerateStreamToken(r.Context(), id)
	if err != nil {
		h.log.Error("failed to generate stream token for WHEP URL", zap.Error(err))
		// We still return success but without the token/url
		h.respondWithJSON(w, http.StatusOK, map[string]string{
			"message": "WHEP stream configured successfully in MediaMTX",
		})
		return
	}

	// Construct WHEP URL (defaulting to localhost:8889 if unable to parse config)
	whepURL := fmt.Sprintf("http://localhost:8889/%s/whep?token=%s", id, token)

	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message":  "WHEP stream configured successfully in MediaMTX",
		"whep_url": whepURL,
	})
}

func (h *Handler) ExportVideo(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	cameraID := r.URL.Query().Get("camera_id")
	startTimeStr := r.URL.Query().Get("start_time")
	endTimeStr := r.URL.Query().Get("end_time")

	if cameraID == "" || !validation.IsUUID(cameraID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera_id format")
		return
	}

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

	// Retrieve operator ID from context
	operatorID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		operatorID = "unknown"
	}

	// Call video service
	zipBytes, filename, err := h.service.ExportVideo(r.Context(), cameraID, startTime, endTime, operatorID)
	if err != nil {
		h.audit.Log(r.Context(), r, "export_video", "camera", cameraID, "failed", map[string]interface{}{"error": err.Error()})
		if errors.Is(err, ErrCameraNotFound) {
			h.respondWithError(w, http.StatusNotFound, "camera not found")
			return
		}
		if errors.Is(err, ErrNoRecordingsFound) {
			h.respondWithError(w, http.StatusNotFound, "no recordings found for the specified period")
			return
		}
		h.log.Error("failed to export video", zap.String("camera_id", cameraID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to export video: "+err.Error())
		return
	}

	// Successful export audit log
	h.audit.Log(r.Context(), r, "export_video", "camera", cameraID, "success", map[string]interface{}{
		"start_time":  startTimeStr,
		"end_time":    endTimeStr,
		"operator_id": operatorID,
		"filename":    filename,
		"size_bytes":  len(zipBytes),
	})

	// Stream the ZIP bytes
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipBytes)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipBytes); err != nil {
		h.log.Error("failed to write export ZIP to response stream", zap.Error(err))
	}
}

type PTZRequest struct {
	Pan  float64 `json:"pan"`
	Tilt float64 `json:"tilt"`
	Zoom float64 `json:"zoom"`
}

func (h *Handler) SendPTZCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	var req PTZRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := h.service.SendPTZCommand(r.Context(), id, req.Pan, req.Tilt, req.Zoom)
	if err != nil {
		h.audit.Log(r.Context(), r, "ptz_command", "camera", id, "failed", map[string]interface{}{"error": err.Error(), "pan": req.Pan, "tilt": req.Tilt, "zoom": req.Zoom})
		if errors.Is(err, ErrCameraNotFound) {
			h.respondWithError(w, http.StatusNotFound, "camera not found")
			return
		}
		h.log.Error("failed to send PTZ command", zap.String("camera_id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to execute PTZ command: "+err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "ptz_command", "camera", id, "success", map[string]interface{}{"pan": req.Pan, "tilt": req.Tilt, "zoom": req.Zoom})
	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "PTZ command executed successfully",
	})
}

func (h *Handler) GetRecordingsForTimeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	startStr := r.URL.Query().Get("start_time")
	endStr := r.URL.Query().Get("end_time")

	if startStr == "" || endStr == "" {
		h.respondWithError(w, http.StatusBadRequest, "missing start_time or end_time query parameter")
		return
	}

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid start_time format (RFC3339 required)")
		return
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid end_time format (RFC3339 required)")
		return
	}

	recordings, err := h.service.GetRecordingsForTimeline(r.Context(), id, start, end)
	if err != nil {
		h.log.Error("failed to get recordings for timeline", zap.String("camera_id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.respondWithJSON(w, http.StatusOK, recordings)
}

func (h *Handler) TriggerMotion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	err := h.service.TriggerMotion(r.Context(), id)
	if err != nil {
		h.audit.Log(r.Context(), r, "trigger_motion", "camera", id, "failed", map[string]interface{}{"error": err.Error()})
		h.respondWithError(w, http.StatusInternalServerError, "failed to trigger motion: "+err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "trigger_motion", "camera", id, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Motion triggered and event published successfully",
	})
}

func (h *Handler) ToggleCameraStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	cam, err := h.service.ToggleCameraStatus(r.Context(), id)
	if err != nil {
		h.audit.Log(r.Context(), r, "toggle_camera_status", "camera", id, "failed", map[string]interface{}{"error": err.Error()})
		h.respondWithError(w, http.StatusInternalServerError, "failed to toggle status: "+err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "toggle_camera_status", "camera", id, "success", map[string]interface{}{"new_status": cam.Statut})
	h.respondWithJSON(w, http.StatusOK, cam)
}

func (h *Handler) CreateBookmark(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "id")
	if cameraID == "" || !validation.IsUUID(cameraID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	var req CreateBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		h.respondWithError(w, http.StatusBadRequest, "bookmark name is required")
		return
	}

	b, err := h.service.CreateBookmark(r.Context(), cameraID, req)
	if err != nil {
		h.audit.Log(r.Context(), r, "create_bookmark", "camera", cameraID, "failed", map[string]interface{}{"name": req.Name, "error": err.Error()})
		h.respondWithError(w, http.StatusInternalServerError, "failed to create bookmark: "+err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "create_bookmark", "camera", cameraID, "success", map[string]interface{}{"bookmark_id": b.ID, "name": b.Name})
	h.respondWithJSON(w, http.StatusCreated, b)
}

func (h *Handler) UpdateBookmark(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid bookmark ID format")
		return
	}

	var req UpdateBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		h.respondWithError(w, http.StatusBadRequest, "bookmark name is required")
		return
	}

	err := h.service.UpdateBookmark(r.Context(), id, req)
	if err != nil {
		h.audit.Log(r.Context(), r, "update_bookmark", "bookmark", id, "failed", map[string]interface{}{"name": req.Name, "error": err.Error()})
		h.respondWithError(w, http.StatusInternalServerError, "failed to update bookmark: "+err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "update_bookmark", "bookmark", id, "success", map[string]interface{}{"name": req.Name})
	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "bookmark updated successfully"})
}

func (h *Handler) DeleteBookmark(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid bookmark ID format")
		return
	}

	err := h.service.DeleteBookmark(r.Context(), id)
	if err != nil {
		h.audit.Log(r.Context(), r, "delete_bookmark", "bookmark", id, "failed", map[string]interface{}{"error": err.Error()})
		h.respondWithError(w, http.StatusInternalServerError, "failed to delete bookmark: "+err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "delete_bookmark", "bookmark", id, "success", nil)
	h.respondWithJSON(w, http.StatusOK, map[string]string{"message": "bookmark deleted successfully"})
}

func (h *Handler) ListBookmarks(w http.ResponseWriter, r *http.Request) {
	cameraID := chi.URLParam(r, "id")
	if cameraID == "" || !validation.IsUUID(cameraID) {
		h.respondWithError(w, http.StatusBadRequest, "invalid camera ID format")
		return
	}

	bookmarks, err := h.service.ListBookmarks(r.Context(), cameraID)
	if err != nil {
		h.log.Error("failed to list bookmarks", zap.String("camera_id", cameraID), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.respondWithJSON(w, http.StatusOK, bookmarks)
}

func (h *Handler) GetBookmarkByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid bookmark ID format")
		return
	}

	b, err := h.service.GetBookmarkByID(r.Context(), id)
	if err != nil {
		h.respondWithError(w, http.StatusNotFound, "bookmark not found")
		return
	}

	h.respondWithJSON(w, http.StatusOK, b)
}
