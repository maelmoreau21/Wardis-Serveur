package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type Handler struct {
	db      *pgxpool.Pool
	natsURL string
}

func NewHandler(db *pgxpool.Pool, natsURL string) *Handler {
	return &Handler{
		db:      db,
		natsURL: natsURL,
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"UP"}`))
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. Verify DB connection if initialized
	if h.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.db.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			resp, _ := json.Marshal(map[string]string{
				"status": "DOWN",
				"reason": "database health check failed: " + err.Error(),
			})
			w.Write(resp)
			return
		}
	}

	// 2. Verify NATS connection if initialized
	if h.natsURL != "" {
		nc, err := nats.Connect(h.natsURL, nats.Timeout(2*time.Second))
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			resp, _ := json.Marshal(map[string]string{
				"status": "DOWN",
				"reason": "nats health check failed: " + err.Error(),
			})
			w.Write(resp)
			return
		}
		nc.Close()
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"UP"}`))
}
