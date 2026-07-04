package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local/Tauri requests
	},
}

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

func (h *Handler) StreamEventsWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("Failed to upgrade HTTP connection to WebSocket", zap.Error(err))
		return
	}
	defer conn.Close()

	h.log.Debug("Client established WebSocket events stream")

	eventCh := h.service.SubscribeClient()
	defer h.service.UnsubscribeClient(eventCh)

	closeCh := make(chan struct{})
	go func() {
		defer close(closeCh)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				h.log.Debug("WebSocket read error (connection closed by client)", zap.Error(err))
				return
			}
		}
	}()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-closeCh:
			h.log.Debug("Closing WebSocket connection due to client disconnect")
			return

		case <-r.Context().Done():
			h.log.Debug("Closing WebSocket connection due to request context cancel")
			return

		case <-ticker.C:
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				h.log.Debug("Failed to send ping to WS client, closing connection", zap.Error(err))
				return
			}

		case event, ok := <-eventCh:
			if !ok {
				h.log.Debug("Event channel closed, closing WebSocket connection")
				return
			}

			err := conn.WriteJSON(event)
			if err != nil {
				h.log.Debug("Failed to write event to WS client, closing connection", zap.Error(err))
				return
			}
		}
	}
}

