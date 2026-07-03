package audit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"wardis-server/internal/auth"
)

type AuditLogger interface {
	Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{})
}

type auditLogger struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func New(db *pgxpool.Pool, log *zap.Logger) AuditLogger {
	return &auditLogger{
		db:  db,
		log: log,
	}
}

func (a *auditLogger) Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{}) {
	var userID *string
	var userEmail string

	// Extract claims from context if present
	if claims, ok := auth.UserClaimsFromContext(ctx); ok {
		uid := claims.UserID
		userID = &uid
		userEmail = claims.Email
	}

	// Extract client IP address
	var ip string
	if r != nil {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				ip = strings.TrimSpace(parts[0])
			}
		}
		if ip == "" {
			var err error
			ip, _, err = net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
		}
	}

	// Marshal details to JSON
	var detailsJSON []byte
	if details != nil {
		var err error
		detailsJSON, err = json.Marshal(details)
		if err != nil {
			a.log.Error("failed to marshal audit details to JSON", zap.Error(err))
		}
	}
	if len(detailsJSON) == 0 {
		detailsJSON = []byte("{}")
	}

	// Log structured output to stdout immediately
	a.log.Info("Audit Log Entry",
		zap.Bool("audit", true),
		zap.String("action", action),
		zap.String("resource", resource),
		zap.String("resource_id", resourceID),
		zap.String("status", status),
		zap.String("user_email", userEmail),
		zap.String("ip_address", ip),
	)

	// Save to DB asynchronously using a detached context (since the request context might be cancelled)
	if a.db != nil {
		go func() {
			dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			query := `
				INSERT INTO audit_logs (user_id, user_email, action, resource, resource_id, status, details, ip_address)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`
			_, err := a.db.Exec(dbCtx, query, userID, userEmail, action, resource, resourceID, status, detailsJSON, ip)
			if err != nil {
				a.log.Error("failed to write audit log to database", zap.Error(err))
			}
		}()
	}
}

// MockAuditLogger is a mock of the AuditLogger interface for unit testing
type MockAuditLogger struct {
	Logs []MockEntry
}

type MockEntry struct {
	Action     string
	Resource   string
	ResourceID string
	Status     string
	UserEmail  string
}

func (m *MockAuditLogger) Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{}) {
	var userEmail string
	if claims, ok := auth.UserClaimsFromContext(ctx); ok {
		userEmail = claims.Email
	}
	m.Logs = append(m.Logs, MockEntry{
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Status:     status,
		UserEmail:  userEmail,
	})
}
