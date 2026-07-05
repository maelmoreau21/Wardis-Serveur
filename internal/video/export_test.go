package video_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"wardis-server/internal/auth"
	"wardis-server/internal/config"
	"wardis-server/internal/video"
)

type exportMockRepository struct {
	camera      *video.Camera
	recordings  []video.VideoRecording
	getByIDError error
}

func (e *exportMockRepository) List(ctx context.Context) ([]video.Camera, error) {
	return nil, nil
}

func (e *exportMockRepository) GetByID(ctx context.Context, id string) (*video.Camera, error) {
	if e.getByIDError != nil {
		return nil, e.getByIDError
	}
	if e.camera == nil || e.camera.ID != id {
		return nil, video.ErrCameraNotFound
	}
	return e.camera, nil
}

func (e *exportMockRepository) Create(ctx context.Context, c *video.Camera) (*video.Camera, error) {
	return nil, nil
}

func (e *exportMockRepository) Update(ctx context.Context, c *video.Camera) (*video.Camera, error) {
	return nil, nil
}

func (e *exportMockRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (e *exportMockRepository) SaveRecording(ctx context.Context, rec *video.VideoRecording) error {
	return nil
}

func (e *exportMockRepository) GetOverlappingRecordings(ctx context.Context, cameraID string, start, end time.Time) ([]video.VideoRecording, error) {
	return e.recordings, nil
}

func (e *exportMockRepository) DeleteRecording(ctx context.Context, id string) error {
	return nil
}

func (e *exportMockRepository) GetLocalRecordingsOlderThan(ctx context.Context, threshold time.Time) ([]video.VideoRecording, error) {
	return nil, nil
}

func (e *exportMockRepository) GetAllRecordingsOlderThan(ctx context.Context, threshold time.Time) ([]video.VideoRecording, error) {
	return nil, nil
}

func (e *exportMockRepository) UpdateRecordingStorageType(ctx context.Context, id string, storageType string, filepath string) error {
	return nil
}

func (e *exportMockRepository) CreateBookmark(ctx context.Context, b *video.Bookmark) (*video.Bookmark, error) {
	return b, nil
}
func (e *exportMockRepository) UpdateBookmark(ctx context.Context, id string, name, notes string) error {
	return nil
}
func (e *exportMockRepository) DeleteBookmark(ctx context.Context, id string) error {
	return nil
}
func (e *exportMockRepository) ListBookmarks(ctx context.Context, cameraID string) ([]video.Bookmark, error) {
	return nil, nil
}
func (e *exportMockRepository) GetBookmarkByID(ctx context.Context, id string) (*video.Bookmark, error) {
	return nil, nil
}

type exportMockAuditLogger struct {
	logs []string
}

func (a *exportMockAuditLogger) Log(ctx context.Context, r *http.Request, action string, resource string, resourceID string, status string, details map[string]interface{}) {
	a.logs = append(a.logs, fmt.Sprintf("%s:%s:%s", action, resource, status))
}

type mockAuthService struct {
	validateClaims *auth.Claims
	validateError  error
}

func (m *mockAuthService) Login(ctx context.Context, email, password, ip, userAgent string) (*auth.LoginResponse, error) {
	return nil, nil
}
func (m *mockAuthService) ValidateMfa(ctx context.Context, mfaToken, code, ip, userAgent string) (*auth.LoginResponse, error) {
	return nil, nil
}
func (m *mockAuthService) ValidateToken(tokenStr string) (*auth.Claims, error) {
	return m.validateClaims, m.validateError
}
func (m *mockAuthService) HashPassword(password string) (string, error) {
	return "", nil
}
func (m *mockAuthService) GetUserByID(ctx context.Context, id string) (*auth.User, error) {
	return nil, nil
}
func (m *mockAuthService) CheckEntityPermission(ctx context.Context, userID string, permissionName string, entityType string, entityID string) (bool, error) {
	return true, nil
}
func (m *mockAuthService) UpdatePassword(ctx context.Context, id string, currentPassword, newPassword string) error {
	return nil
}
func (m *mockAuthService) ListUsers(ctx context.Context) ([]auth.User, error) {
	return nil, nil
}
func (m *mockAuthService) CreateUser(ctx context.Context, email, password, roleName string) (*auth.User, error) {
	return nil, nil
}
func (m *mockAuthService) DeleteUser(ctx context.Context, id string) error {
	return nil
}
func (m *mockAuthService) ListRoles(ctx context.Context) ([]auth.Role, error) {
	return nil, nil
}
func (m *mockAuthService) ListPermissions(ctx context.Context) ([]auth.Permission, error) {
	return nil, nil
}
func (m *mockAuthService) GetEntityPermissionsForUser(ctx context.Context, userID string) ([]auth.EntityPermission, error) {
	return nil, nil
}
func (m *mockAuthService) SaveEntityPermissions(ctx context.Context, userID string, entityPermissions []auth.EntityPermission) error {
	return nil
}
func (m *mockAuthService) SetupMfa(ctx context.Context, userID string) (*auth.MfaSetupResponse, error) {
	return nil, nil
}
func (m *mockAuthService) EnableMfa(ctx context.Context, userID, code string) error {
	return nil
}
func (m *mockAuthService) DisableMfa(ctx context.Context, userID string) error {
	return nil
}
func (m *mockAuthService) CreateRole(ctx context.Context, name, description string, permissions []string) (*auth.Role, error) {
	return nil, nil
}
func (m *mockAuthService) UpdateRole(ctx context.Context, roleID int, description string, permissions []string) error {
	return nil
}
func (m *mockAuthService) DeleteRole(ctx context.Context, roleID int) error {
	return nil
}
func (m *mockAuthService) ListActiveSessions(ctx context.Context) ([]auth.Session, error) {
	return nil, nil
}
func (m *mockAuthService) ListUserActiveSessions(ctx context.Context, userID string) ([]auth.Session, error) {
	return nil, nil
}
func (m *mockAuthService) RevokeSession(ctx context.Context, sessionID string) error {
	return nil
}

func TestExportVideoService(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &config.Config{
		RecordingsLocalDir: tempDir,
	}

	camID := "e9ee0ef9-73fb-46db-908f-2877cb7a5f24"
	mockCam := &video.Camera{
		ID:           camID,
		Nom:          "Test Camera",
		URLRTSP:      "rtsp://test-url",
		Statut:       "active",
		PTZSupported: true,
	}

	// Create dummy segments
	segment1Bytes := []byte("dummy video segment 1")
	segment2Bytes := []byte("dummy video segment 2")

	// Write segments to local dir
	camDir := filepath.Join(tempDir, camID)
	err := os.MkdirAll(camDir, 0755)
	if err != nil {
		t.Fatalf("failed to create temp camera dir: %v", err)
	}

	file1 := "seg1.mp4"
	file2 := "seg2.mp4"
	err = os.WriteFile(filepath.Join(camDir, file1), segment1Bytes, 0644)
	if err != nil {
		t.Fatalf("failed to write dummy segment 1: %v", err)
	}
	err = os.WriteFile(filepath.Join(camDir, file2), segment2Bytes, 0644)
	if err != nil {
		t.Fatalf("failed to write dummy segment 2: %v", err)
	}

	mockRecordings := []video.VideoRecording{
		{
			ID:          "rec-1",
			CameraID:    camID,
			StartTime:   time.Now().Add(-10 * time.Minute),
			EndTime:     time.Now().Add(-5 * time.Minute),
			Filepath:    filepath.Join(camID, file1),
			StorageType: "local",
			FileSize:    int64(len(segment1Bytes)),
		},
		{
			ID:          "rec-2",
			CameraID:    camID,
			StartTime:   time.Now().Add(-5 * time.Minute),
			EndTime:     time.Now(),
			Filepath:    filepath.Join(camID, file2),
			StorageType: "local",
			FileSize:    int64(len(segment2Bytes)),
		},
	}

	t.Run("Success Export Local Segments", func(t *testing.T) {
		repo := &exportMockRepository{
			camera:     mockCam,
			recordings: mockRecordings,
		}
		svc := video.NewService(repo, nil, nil, "jwtSecret", logger, cfg)

		operatorID := "operator-123"
		zipBytes, filename, err := svc.ExportVideo(ctx, camID, time.Now().Add(-15 * time.Minute), time.Now(), operatorID)
		if err != nil {
			t.Fatalf("unexpected error exporting video: %v", err)
		}

		if filename == "" {
			t.Errorf("expected filename to be populated, got empty")
		}

		// Read and verify the zip archive
		zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			t.Fatalf("failed to read generated zip archive: %v", err)
		}

		var videoFound, manifestFound bool
		var videoContent []byte
		var manifestContent []byte

		for _, f := range zipReader.File {
			if f.Name == "video.mp4" {
				videoFound = true
				rc, err := f.Open()
				if err != nil {
					t.Fatalf("failed to open video.mp4 from zip: %v", err)
				}
				var buf bytes.Buffer
				_, _ = buf.ReadFrom(rc)
				videoContent = buf.Bytes()
				rc.Close()
			}
			if f.Name == "manifest.json" {
				manifestFound = true
				rc, err := f.Open()
				if err != nil {
					t.Fatalf("failed to open manifest.json from zip: %v", err)
				}
				var buf bytes.Buffer
				_, _ = buf.ReadFrom(rc)
				manifestContent = buf.Bytes()
				rc.Close()
			}
		}

		if !videoFound {
			t.Error("video.mp4 not found in zip archive")
		}
		if !manifestFound {
			t.Error("manifest.json not found in zip archive")
		}

		// Check concatenated video content
		expectedVideoBytes := append(segment1Bytes, segment2Bytes...)
		if !bytes.Equal(videoContent, expectedVideoBytes) {
			t.Errorf("concatenated video bytes mismatch, expected %s, got %s", expectedVideoBytes, videoContent)
		}

		// Verify manifest content
		var manifestMap map[string]interface{}
		err = json.Unmarshal(manifestContent, &manifestMap)
		if err != nil {
			t.Fatalf("failed to unmarshal manifest JSON: %v", err)
		}

		if manifestMap["operator_id"] != operatorID {
			t.Errorf("expected operator ID %s, got %v", operatorID, manifestMap["operator_id"])
		}

		cameraMeta, ok := manifestMap["camera"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected camera metadata struct, got %T", manifestMap["camera"])
		}
		if cameraMeta["id"] != camID {
			t.Errorf("expected camera ID %s in manifest, got %v", camID, cameraMeta["id"])
		}
		if cameraMeta["nom"] != "Test Camera" {
			t.Errorf("expected camera name Test Camera, got %v", cameraMeta["nom"])
		}
		if _, ok := cameraMeta["password_encrypted"]; ok {
			t.Error("security breach: password_encrypted must not be included in manifest metadata")
		}

		// Check SHA-256 cryptographic control hash matches
		expectedHash := sha256.Sum256(expectedVideoBytes)
		expectedHashHex := fmt.Sprintf("%x", expectedHash)
		if manifestMap["sha256"] != expectedHashHex {
			t.Errorf("expected SHA-256 %s, got %v", expectedHashHex, manifestMap["sha256"])
		}
	})

	t.Run("Camera Not Found", func(t *testing.T) {
		repo := &exportMockRepository{
			camera: nil,
		}
		svc := video.NewService(repo, nil, nil, "jwtSecret", logger, cfg)

		_, _, err := svc.ExportVideo(ctx, "non-existent-camera", time.Now().Add(-15 * time.Minute), time.Now(), "op-1")
		if !errors.Is(err, video.ErrCameraNotFound) {
			t.Errorf("expected ErrCameraNotFound, got %v", err)
		}
	})

	t.Run("No Recordings Found", func(t *testing.T) {
		repo := &exportMockRepository{
			camera:     mockCam,
			recordings: nil,
		}
		svc := video.NewService(repo, nil, nil, "jwtSecret", logger, cfg)

		_, _, err := svc.ExportVideo(ctx, camID, time.Now().Add(-15 * time.Minute), time.Now(), "op-1")
		if !errors.Is(err, video.ErrNoRecordingsFound) {
			t.Errorf("expected ErrNoRecordingsFound, got %v", err)
		}
	})
}

func TestExportVideoHandler(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	cfg := &config.Config{
		RecordingsLocalDir: tempDir,
	}

	camID := "e9ee0ef9-73fb-46db-908f-2877cb7a5f24"
	mockCam := &video.Camera{
		ID:           camID,
		Nom:          "Test Camera",
		URLRTSP:      "rtsp://test-url",
		Statut:       "active",
		PTZSupported: true,
	}

	segment1Bytes := []byte("test segment")
	camDir := filepath.Join(tempDir, camID)
	_ = os.MkdirAll(camDir, 0755)
	file1 := "seg1.mp4"
	_ = os.WriteFile(filepath.Join(camDir, file1), segment1Bytes, 0644)

	mockRecordings := []video.VideoRecording{
		{
			ID:          "rec-1",
			CameraID:    camID,
			StartTime:   time.Now().Add(-10 * time.Minute),
			EndTime:     time.Now().Add(-5 * time.Minute),
			Filepath:    filepath.Join(camID, file1),
			StorageType: "local",
			FileSize:    int64(len(segment1Bytes)),
		},
	}

	repo := &exportMockRepository{
		camera:     mockCam,
		recordings: mockRecordings,
	}
	svc := video.NewService(repo, nil, nil, "jwtSecret", logger, cfg)
	auditLogger := &exportMockAuditLogger{}
	handler := video.NewHandler(svc, logger, auditLogger)

	t.Run("Success HTTP Handler Flow", func(t *testing.T) {
		startTimeStr := time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339)
		endTimeStr := time.Now().UTC().Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/video/export?camera_id=%s&start_time=%s&end_time=%s", camID, startTimeStr, endTimeStr), nil)
		req.Header.Set("Authorization", "Bearer valid-dummy-token")
		w := httptest.NewRecorder()

		mockAuthSvc := &mockAuthService{
			validateClaims: &auth.Claims{
				UserID: "operator-uuid-999",
				Email:  "operator@test.com",
				Role:   "operator",
			},
		}

		router := chi.NewRouter()
		router.Use(auth.AuthMiddleware(mockAuthSvc))
		router.Get("/api/v1/video/export", handler.ExportVideo)

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "application/zip" {
			t.Errorf("expected Content-Type application/zip, got %s", contentType)
		}

		contentDisposition := w.Header().Get("Content-Disposition")
		if contentDisposition == "" || !bytes.Contains([]byte(contentDisposition), []byte("attachment; filename=")) {
			t.Errorf("expected Content-Disposition header with filename, got %s", contentDisposition)
		}

		// Verify audit logs
		if len(auditLogger.logs) == 0 {
			t.Error("expected audit log to be written on success")
		} else {
			lastLog := auditLogger.logs[len(auditLogger.logs)-1]
			if lastLog != "export_video:camera:success" {
				t.Errorf("expected audit action export_video, got: %s", lastLog)
			}
		}
	})

	t.Run("HTTP Handler Missing Parameters", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/video/export?camera_id=%s", camID), nil)
		req.Header.Set("Authorization", "Bearer valid-dummy-token")
		w := httptest.NewRecorder()

		mockAuthSvc := &mockAuthService{
			validateClaims: &auth.Claims{
				UserID: "operator-uuid-999",
				Role:   "operator",
			},
		}

		router := chi.NewRouter()
		router.Use(auth.AuthMiddleware(mockAuthSvc))
		router.Get("/api/v1/video/export", handler.ExportVideo)

		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected HTTP 400, got %d", w.Code)
		}
	})
}
