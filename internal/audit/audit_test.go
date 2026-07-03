package audit_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"wardis-server/internal/audit"
)

func TestAuditLoggerStdout(t *testing.T) {
	// Create observer core to intercept Zap logging calls
	core, recorded := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	auditLog := audit.New(nil, logger)

	// Send an audit log call
	req := httptest.NewRequest("POST", "/login", nil)
	req.Header.Set("X-Forwarded-For", "192.0.2.1")

	auditLog.Log(context.Background(), req, "login", "auth", "test-user-id", "success", map[string]interface{}{"email": "user@example.com"})

	// Verify Zap caught the log message
	if recorded.Len() != 1 {
		t.Fatalf("Expected 1 log entry, got %d", recorded.Len())
	}

	entry := recorded.All()[0]
	if entry.Message != "Audit Log Entry" {
		t.Errorf("Expected log message 'Audit Log Entry', got %q", entry.Message)
	}

	// Verify contextual fields
	fields := entry.ContextMap()
	if fields["audit"] != true {
		t.Errorf("Expected 'audit' field to be true, got %v", fields["audit"])
	}
	if fields["action"] != "login" {
		t.Errorf("Expected 'action' field to be 'login', got %v", fields["action"])
	}
	if fields["resource"] != "auth" {
		t.Errorf("Expected 'resource' field to be 'auth', got %v", fields["resource"])
	}
	if fields["resource_id"] != "test-user-id" {
		t.Errorf("Expected 'resource_id' field to be 'test-user-id', got %v", fields["resource_id"])
	}
	if fields["status"] != "success" {
		t.Errorf("Expected 'status' field to be 'success', got %v", fields["status"])
	}
	if fields["ip_address"] != "192.0.2.1" {
		t.Errorf("Expected 'ip_address' field to be '192.0.2.1', got %v", fields["ip_address"])
	}
}

func TestMockAuditLogger(t *testing.T) {
	mock := &audit.MockAuditLogger{}
	var logger audit.AuditLogger = mock

	logger.Log(context.Background(), nil, "open_door", "door", "door-uuid-1", "success", nil)

	if len(mock.Logs) != 1 {
		t.Fatalf("Expected 1 log in mock, got %d", len(mock.Logs))
	}

	entry := mock.Logs[0]
	if entry.Action != "open_door" || entry.Resource != "door" || entry.ResourceID != "door-uuid-1" || entry.Status != "success" {
		t.Errorf("Mock audit entry mismatch: %+v", entry)
	}
}
