package layout

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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.UserClaimsFromContext(r.Context())
	userID := ""
	if ok {
		userID = claims.UserID
	}

	views, err := h.service.List(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to list saved views", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to list saved views")
		return
	}

	h.respondWithJSON(w, http.StatusOK, views)
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid view ID format")
		return
	}

	view, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrViewNotFound) {
			h.respondWithError(w, http.StatusNotFound, "saved view not found")
			return
		}
		h.log.Error("failed to get saved view by id", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, "failed to get saved view")
		return
	}

	h.respondWithJSON(w, http.StatusOK, view)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.UserClaimsFromContext(r.Context())
	userID := ""
	if ok {
		userID = claims.UserID
	}

	var req SavedViewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		h.respondWithError(w, http.StatusBadRequest, "name is required")
		return
	}

	view, err := h.service.Create(r.Context(), userID, req)
	if err != nil {
		h.audit.Log(r.Context(), r, "create_view", "saved_view", "", "failed", map[string]interface{}{"name": req.Name, "error": err.Error()})
		h.log.Error("failed to create saved view", zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "create_view", "saved_view", view.ID, "success", map[string]interface{}{"name": view.Name})
	h.respondWithJSON(w, http.StatusCreated, view)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid view ID format")
		return
	}

	var req SavedViewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		h.respondWithError(w, http.StatusBadRequest, "name is required")
		return
	}

	view, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		h.audit.Log(r.Context(), r, "update_view", "saved_view", id, "failed", map[string]interface{}{"name": req.Name, "error": err.Error()})
		h.log.Error("failed to update saved view", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "update_view", "saved_view", id, "success", map[string]interface{}{"name": view.Name})
	h.respondWithJSON(w, http.StatusOK, view)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || !validation.IsUUID(id) {
		h.respondWithError(w, http.StatusBadRequest, "invalid view ID format")
		return
	}

	err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.audit.Log(r.Context(), r, "delete_view", "saved_view", id, "failed", map[string]interface{}{"error": err.Error()})
		h.log.Error("failed to delete saved view", zap.String("id", id), zap.Error(err))
		h.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit.Log(r.Context(), r, "delete_view", "saved_view", id, "success", nil)
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
