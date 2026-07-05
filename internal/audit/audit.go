package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"wardis-server/internal/auth"
)

type AuditEntry struct {
	ID         string                 `json:"id"`
	UserID     *string                `json:"user_id"`
	UserEmail  string                 `json:"user_email"`
	Action     string                 `json:"action"`
	Resource   string                 `json:"resource"`
	ResourceID *string                `json:"resource_id"`
	Status     string                 `json:"status"`
	Details    map[string]interface{} `json:"details"`
	IPAddress  string                 `json:"ip_address"`
	CreatedAt  time.Time              `json:"created_at"`
}

type AuditLogger interface {
	Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{})
	List(ctx context.Context, email, action, resource, status string, startTime, endTime *time.Time, limit, offset int) ([]AuditEntry, int, error)
	ListHandler(w http.ResponseWriter, r *http.Request)
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

func (a *auditLogger) List(ctx context.Context, email, action, resource, status string, startTime, endTime *time.Time, limit, offset int) ([]AuditEntry, int, error) {
	var conditions []string
	var args []interface{}
	argCount := 1

	if email != "" {
		conditions = append(conditions, fmt.Sprintf("user_email ILIKE $%d", argCount))
		args = append(args, "%"+email+"%")
		argCount++
	}
	if action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argCount))
		args = append(args, action)
		argCount++
	}
	if resource != "" {
		conditions = append(conditions, fmt.Sprintf("resource = $%d", argCount))
		args = append(args, resource)
		argCount++
	}
	if status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argCount))
		args = append(args, status)
		argCount++
	}
	if startTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argCount))
		args = append(args, *startTime)
		argCount++
	}
	if endTime != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argCount))
		args = append(args, *endTime)
		argCount++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", whereClause)
	var total int
	err := a.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	// Get records
	query := fmt.Sprintf(`
		SELECT id, user_id, user_email, action, resource, resource_id, status, details, ip_address, created_at
		FROM audit_logs
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argCount, argCount+1)

	argsWithLimit := append(args, limit, offset)
	rows, err := a.db.Query(ctx, query, argsWithLimit...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		var userID sql.NullString
		var resourceID sql.NullString
		var detailsJSON []byte

		err := rows.Scan(
			&entry.ID,
			&userID,
			&entry.UserEmail,
			&entry.Action,
			&entry.Resource,
			&resourceID,
			&entry.Status,
			&detailsJSON,
			&entry.IPAddress,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit entry row: %w", err)
		}

		if userID.Valid {
			entry.UserID = &userID.String
		}
		if resourceID.Valid {
			entry.ResourceID = &resourceID.String
		}

		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &entry.Details)
		}
		if entry.Details == nil {
			entry.Details = make(map[string]interface{})
		}

		entries = append(entries, entry)
	}

	return entries, total, nil
}

func (a *auditLogger) ListHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	action := r.URL.Query().Get("action")
	resource := r.URL.Query().Get("resource")
	status := r.URL.Query().Get("status")
	startTimeStr := r.URL.Query().Get("start_time")
	endTimeStr := r.URL.Query().Get("end_time")

	var startTime *time.Time
	if startTimeStr != "" {
		tVal, err := time.Parse(time.RFC3339, startTimeStr)
		if err == nil {
			startTime = &tVal
		}
	}

	var endTime *time.Time
	if endTimeStr != "" {
		tVal, err := time.Parse(time.RFC3339, endTimeStr)
		if err == nil {
			endTime = &tVal
		}
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	entries, total, err := a.List(r.Context(), email, action, resource, status, startTime, endTime, limit, offset)
	if err != nil {
		a.log.Error("failed to list audit logs", zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to query audit logs"}`))
		return
	}

	resp := map[string]interface{}{
		"logs":  entries,
		"total": total,
	}

	responseJSON, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON)
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

func (m *MockAuditLogger) List(ctx context.Context, email, action, resource, status string, startTime, endTime *time.Time, limit, offset int) ([]AuditEntry, int, error) {
	return nil, 0, nil
}

func (m *MockAuditLogger) ListHandler(w http.ResponseWriter, r *http.Request) {
}
