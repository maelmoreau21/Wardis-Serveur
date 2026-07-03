package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.ListEvents(r.Context())
	if err != nil {
		h.log.Error("Failed to list events", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "failed to list events"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(events); err != nil {
		h.log.Error("Failed to encode events response", zap.Error(err))
	}
}

func (h *Handler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.log.Warn("Streaming unsupported by response writer")
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	eventCh := h.service.SubscribeClient()
	defer h.service.UnsubscribeClient(eventCh)

	h.log.Debug("Client established Server-Sent Events stream")

	// Send an initial comment to keep-alive and verify connection
	_, _ = fmt.Fprint(w, ": ok\n\n")
	flusher.Flush()

	// Keep alive ticker to prevent connection timeouts
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			h.log.Debug("Client closed Server-Sent Events stream connection")
			return

		case tickerChan := <-ticker.C:
			// Ping client to keep connection alive
			_ = tickerChan
			_, err := fmt.Fprint(w, ": ping\n\n")
			if err != nil {
				h.log.Debug("Failed to send ping to SSE client, closing connection", zap.Error(err))
				return
			}
			flusher.Flush()

		case event, ok := <-eventCh:
			if !ok {
				h.log.Debug("Event channel closed, closing SSE client connection")
				return
			}

			eventBytes, err := json.Marshal(event)
			if err != nil {
				h.log.Error("Failed to marshal event for streaming", zap.Error(err))
				continue
			}

			_, err = fmt.Fprintf(w, "data: %s\n\n", string(eventBytes))
			if err != nil {
				h.log.Debug("Failed to send event to SSE client, closing connection", zap.Error(err))
				return
			}
			flusher.Flush()
		}
	}
}
