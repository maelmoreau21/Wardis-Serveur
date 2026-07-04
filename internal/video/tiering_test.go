package video

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"wardis-server/internal/config"
)

type mockRecordingRepository struct {
	Repository
	saveCalls               []*VideoRecording
	deleteRecordingCalls    []string
	overlappingRecordings   []VideoRecording
	getOverlappingCalls     [][3]interface{} // cameraID, start, end
	localRecordings         []VideoRecording
	updateStorageCalls      []struct{ id, storageType, filepath string }
	getByIDCamera           *Camera
	getByIDError            error
}

func (m *mockRecordingRepository) GetByID(ctx context.Context, id string) (*Camera, error) {
	if m.getByIDError != nil {
		return nil, m.getByIDError
	}
	if m.getByIDCamera != nil {
		return m.getByIDCamera, nil
	}
	return &Camera{ID: id, Nom: "Test Camera"}, nil
}

func (m *mockRecordingRepository) SaveRecording(ctx context.Context, rec *VideoRecording) error {
	rec.ID = "new-rec-id"
	rec.CreatedAt = time.Now()
	m.saveCalls = append(m.saveCalls, rec)
	return nil
}

func (m *mockRecordingRepository) GetOverlappingRecordings(ctx context.Context, cameraID string, start, end time.Time) ([]VideoRecording, error) {
	m.getOverlappingCalls = append(m.getOverlappingCalls, [3]interface{}{cameraID, start, end})
	return m.overlappingRecordings, nil
}

func (m *mockRecordingRepository) DeleteRecording(ctx context.Context, id string) error {
	m.deleteRecordingCalls = append(m.deleteRecordingCalls, id)
	return nil
}

func (m *mockRecordingRepository) GetLocalRecordingsOlderThan(ctx context.Context, threshold time.Time) ([]VideoRecording, error) {
	return m.localRecordings, nil
}

func (m *mockRecordingRepository) UpdateRecordingStorageType(ctx context.Context, id string, storageType string, filepath string) error {
	m.updateStorageCalls = append(m.updateStorageCalls, struct{ id, storageType, filepath string }{id, storageType, filepath})
	return nil
}

func TestSyncRecordingLocalFile(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()
	cfg := &config.Config{
		RecordingsLocalDir: tempDir,
	}

	repo := &mockRecordingRepository{}
	svc := NewService(repo, nil, nil, "secret", logger, cfg).(*service)

	cameraID := "c1e847be-a53c-4dc5-b91c-0e6ce36eca31"
	startTime := time.Now()
	endTime := startTime.Add(1 * time.Minute)
	fileData := []byte("fake video content")
	filename := "test_video.mp4"

	rec, err := svc.SyncRecording(context.Background(), cameraID, startTime, endTime, fileData, filename)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if rec == nil {
		t.Fatal("expected recording object to be returned, got nil")
	}

	// Verify local file exists
	expectedPath := filepath.Join(tempDir, cameraID, filename)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected video file to be written to %s, but it does not exist", expectedPath)
	}

	// Read content
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if !bytes.Equal(data, fileData) {
		t.Errorf("expected file content %s, got %s", string(fileData), string(data))
	}

	// Verify repository SaveRecording was called
	if len(repo.saveCalls) != 1 {
		t.Fatalf("expected SaveRecording to be called once, got %d times", len(repo.saveCalls))
	}
	savedRec := repo.saveCalls[0]
	if savedRec.CameraID != cameraID {
		t.Errorf("expected CameraID %s, got %s", cameraID, savedRec.CameraID)
	}
	if savedRec.Filepath != filepath.Join(cameraID, filename) {
		t.Errorf("expected Filepath %s, got %s", filepath.Join(cameraID, filename), savedRec.Filepath)
	}
}

func TestSyncRecordingOverlapReconciliation(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()
	cfg := &config.Config{
		RecordingsLocalDir: tempDir,
	}

	baseTime := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	// Predefine overlapping recordings
	overlapping := []VideoRecording{
		{ID: "r1", CameraID: "cam-1", StartTime: baseTime, EndTime: baseTime.Add(5 * time.Minute), Filepath: "cam-1/r1.mp4", StorageType: "local"},
		{ID: "r2", CameraID: "cam-1", StartTime: baseTime.Add(15 * time.Minute), EndTime: baseTime.Add(20 * time.Minute), Filepath: "cam-1/r2.mp4", StorageType: "local"},
	}

	repo := &mockRecordingRepository{
		overlappingRecordings: overlapping,
	}
	svc := NewService(repo, nil, nil, "secret", logger, cfg).(*service)

	// Create dummy files that are scheduled to be deleted due to overlap
	_ = os.MkdirAll(filepath.Join(tempDir, "cam-1"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "cam-1", "r1.mp4"), []byte("r1"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "cam-1", "r2.mp4"), []byte("r2"), 0644)

	// Try to sync an incoming recording that completely covers both
	incomingStart := baseTime.Add(-1 * time.Minute)
	incomingEnd := baseTime.Add(25 * time.Minute)
	fileData := []byte("long video")
	filename := "sync.mp4"

	rec, err := svc.SyncRecording(context.Background(), "cam-1", incomingStart, incomingEnd, fileData, filename)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if rec == nil {
		t.Fatal("expected recording object, got nil")
	}

	// Verify that r1 and r2 were deleted from disk and repository
	if len(repo.deleteRecordingCalls) != 2 {
		t.Errorf("expected 2 recordings to be deleted, got: %d", len(repo.deleteRecordingCalls))
	}

	// Verify physical file deletions
	if _, err := os.Stat(filepath.Join(tempDir, "cam-1", "r1.mp4")); !os.IsNotExist(err) {
		t.Error("expected physical file r1.mp4 to be deleted, but it still exists")
	}
	if _, err := os.Stat(filepath.Join(tempDir, "cam-1", "r2.mp4")); !os.IsNotExist(err) {
		t.Error("expected physical file r2.mp4 to be deleted, but it still exists")
	}

	// Verify new file exists
	newPath := filepath.Join(tempDir, "cam-1", filename)
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("expected synced video file to exist, but not found")
	}
}

type mockAuditLogger struct {
	calls []string
}

func (m *mockAuditLogger) Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{}) {
	m.calls = append(m.calls, action)
}

type mockVideoService struct {
	Service
	syncRecordingFunc func(ctx context.Context, cameraID string, startTime, endTime time.Time, fileData []byte, filename string) (*VideoRecording, error)
}

func (m *mockVideoService) SyncRecording(ctx context.Context, cameraID string, startTime, endTime time.Time, fileData []byte, filename string) (*VideoRecording, error) {
	if m.syncRecordingFunc != nil {
		return m.syncRecordingFunc(ctx, cameraID, startTime, endTime, fileData, filename)
	}
	return nil, nil
}

func TestHTTPSyncRecordingHandler(t *testing.T) {
	logger := zap.NewNop()
	audit := &mockAuditLogger{}

	mockSvc := &mockVideoService{}
	handler := NewHandler(mockSvc, logger, audit)

	// Set up router
	router := chi.NewRouter()
	router.Post("/cameras/{id}/sync", handler.SyncRecording)

	// 1. Prepare Multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	startTime := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	endTime := time.Date(2026, 7, 4, 12, 5, 0, 0, time.UTC).Format(time.RFC3339)

	_ = writer.WriteField("start_time", startTime)
	_ = writer.WriteField("end_time", endTime)

	part, err := writer.CreateFormFile("video", "video.mp4")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("fake video content"))
	_ = writer.Close()

	// 2. Set up mock service behavior
	called := false
	mockSvc.syncRecordingFunc = func(ctx context.Context, cameraID string, start, end time.Time, data []byte, name string) (*VideoRecording, error) {
		called = true
		if cameraID != "c1e847be-a53c-4dc5-b91c-0e6ce36eca31" {
			t.Errorf("expected Camera ID c1e847be-a53c-4dc5-b91c-0e6ce36eca31, got %s", cameraID)
		}
		if name != "video.mp4" {
			t.Errorf("expected filename video.mp4, got %s", name)
		}
		return &VideoRecording{
			ID:          "rec-123",
			CameraID:    cameraID,
			StartTime:   start,
			EndTime:     end,
			Filepath:    "cam/video.mp4",
			StorageType: "local",
			FileSize:    int64(len(data)),
		}, nil
	}

	// 3. Dispatch HTTP request
	req := httptest.NewRequest("POST", "/cameras/c1e847be-a53c-4dc5-b91c-0e6ce36eca31/sync", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected response code 201 Created, got: %d, body: %s", rr.Code, rr.Body.String())
	}

	if !called {
		t.Error("expected Service.SyncRecording to be called, but it was not")
	}

	// Verify response body
	var resp VideoRecording
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.ID != "rec-123" {
		t.Errorf("expected recording ID rec-123, got %s", resp.ID)
	}
}

func TestNatsSyncPayloadHandler(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()
	cfg := &config.Config{
		RecordingsLocalDir: tempDir,
	}

	repo := &mockRecordingRepository{}
	svc := NewService(repo, nil, nil, "secret", logger, cfg).(*service)

	startTime := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	endTime := startTime.Add(1 * time.Minute)
	videoData := []byte("nats raw data")
	videoDataB64 := base64.StdEncoding.EncodeToString(videoData)

	payload := SyncRecordingPayload{
		CameraID:     "c1e847be-a53c-4dc5-b91c-0e6ce36eca31",
		StartTime:    startTime,
		EndTime:      endTime,
		VideoDataB64: videoDataB64,
	}

	payloadBytes, _ := json.Marshal(payload)

	// Mock NATS Msg
	msg := &nats.Msg{
		Subject: "video.sync",
		Data:    payloadBytes,
	}

	// Invoke subscriber handler
	svc.handleNatsSyncMessage(msg)

	// Verify file was saved on disk
	expectedFilename := fmt.Sprintf("%d_%d.mp4", startTime.Unix(), endTime.Unix())
	expectedPath := filepath.Join(tempDir, "c1e847be-a53c-4dc5-b91c-0e6ce36eca31", expectedFilename)

	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("expected file to be created by NATS subscriber handler at %s, but not found", expectedPath)
	}

	// Read content
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !bytes.Equal(data, videoData) {
		t.Errorf("expected file data %s, got %s", string(videoData), string(data))
	}
}
