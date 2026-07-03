package accesscontrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"wardis-server/internal/accesscontrol"
)

type mockService struct {
	accesscontrol.Service
}

type mockAudit struct{}

func (a *mockAudit) Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{}) {
}

func TestHandler_Validation(t *testing.T) {
	log := zap.NewNop()
	svc := &mockService{}
	auditLog := &mockAudit{}
	handler := accesscontrol.NewHandler(svc, log, auditLog)

	t.Run("OpenDoor - Invalid UUID format", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/doors/invalid-uuid/open", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "invalid-uuid")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.OpenDoor(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d", rec.Code)
		}

		var resp map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["error"] != "invalid door ID format" {
			t.Errorf("Expected error message 'invalid door ID format', got %q", resp["error"])
		}
	})

	t.Run("AssignBadge - Invalid UserID UUID format", func(t *testing.T) {
		body, _ := json.Marshal(accesscontrol.AssignBadgeRequest{
			BadgeNumber: "CARD123",
			UserID:      "invalid-uuid",
		})
		req := httptest.NewRequest("POST", "/badges/assign", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()
		handler.AssignBadge(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d", rec.Code)
		}

		var resp map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["error"] != "invalid user_id UUID format" {
			t.Errorf("Expected error message 'invalid user_id UUID format', got %q", resp["error"])
		}
	})

	t.Run("AssignBadge - Invalid BadgeNumber format", func(t *testing.T) {
		body, _ := json.Marshal(accesscontrol.AssignBadgeRequest{
			BadgeNumber: "CA", // too short, min is 3
			UserID:      "ca000000-0000-0000-0000-000000000001",
		})
		req := httptest.NewRequest("POST", "/badges/assign", bytes.NewBuffer(body))
		rec := httptest.NewRecorder()
		handler.AssignBadge(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request, got %d", rec.Code)
		}

		var resp map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["error"] != "invalid badge_number format or length" {
			t.Errorf("Expected error message 'invalid badge_number format or length', got %q", resp["error"])
		}
	})
}
