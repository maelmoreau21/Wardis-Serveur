package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCorrelateVideoEvent(t *testing.T) {
	logger := zap.NewNop()
	repo := &postgresRepository{db: nil} // We don't need a DB connection for pure correlation logic
	svc := &service{
		repo:    repo,
		natsURL: "nats://localhost:4222",
		log:     logger,
	}

	ctx := context.Background()

	// 1. Valid JSON payload with proper UUID
	validCameraID := "ca000000-0000-0000-0000-000000000001"
	payload := VideoEventPayload{
		CameraID:  validCameraID,
		EventType: "motion",
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(payload)

	event, err := svc.correlateVideoEvent(ctx, "video.motion", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.SourceID != validCameraID {
		t.Errorf("expected SourceID %s, got %s", validCameraID, event.SourceID)
	}
	if event.CameraID == nil || *event.CameraID != validCameraID {
		t.Errorf("expected CameraID %s, got %v", validCameraID, event.CameraID)
	}

	// 2. Malformed JSON payload (should fallback to zero UUID and store raw map)
	malformedData := []byte(`{"camera_id": 12345, "event_type": "motion"}`) // camera_id is not a string
	event, err = svc.correlateVideoEvent(ctx, "video.motion", malformedData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.SourceID != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("expected fallback SourceID zero-uuid, got %s", event.SourceID)
	}
	if event.CameraID != nil {
		t.Errorf("expected CameraID to be nil for malformed payload, got %v", event.CameraID)
	}

	// 3. Valid JSON but invalid CameraID format (e.g. "short-id")
	payloadShort := VideoEventPayload{
		CameraID:  "short-id",
		EventType: "motion",
		Timestamp: time.Now(),
	}
	dataShort, _ := json.Marshal(payloadShort)
	event, err = svc.correlateVideoEvent(ctx, "video.motion", dataShort)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.SourceID != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("expected fallback SourceID for short CameraID, got %s", event.SourceID)
	}
	if event.CameraID != nil {
		t.Errorf("expected CameraID to be nil for short CameraID, got %v", event.CameraID)
	}
}
