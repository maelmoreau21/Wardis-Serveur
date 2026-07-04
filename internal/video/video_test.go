package video_test

import (
	"context"
	"encoding/xml"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"wardis-server/internal/video"
)

type mockRepository struct {
	cameras map[string]video.Camera
}

func (m *mockRepository) List(ctx context.Context) ([]video.Camera, error) {
	var list []video.Camera
	for _, c := range m.cameras {
		list = append(list, c)
	}
	return list, nil
}

func (m *mockRepository) GetByID(ctx context.Context, id string) (*video.Camera, error) {
	cam, ok := m.cameras[id]
	if !ok {
		return nil, video.ErrCameraNotFound
	}
	return &cam, nil
}

func (m *mockRepository) Create(ctx context.Context, c *video.Camera) (*video.Camera, error) {
	c.ID = "cam-uuid-" + c.Nom
	c.CreatedAt = time.Now()
	m.cameras[c.ID] = *c
	return c, nil
}

func (m *mockRepository) Update(ctx context.Context, c *video.Camera) (*video.Camera, error) {
	if _, ok := m.cameras[c.ID]; !ok {
		return nil, video.ErrCameraNotFound
	}
	m.cameras[c.ID] = *c
	return c, nil
}

func (m *mockRepository) Delete(ctx context.Context, id string) error {
	if _, ok := m.cameras[id]; !ok {
		return video.ErrCameraNotFound
	}
	delete(m.cameras, id)
	return nil
}

type mockMediaMtxClient struct {
	paths       map[string]string // key: path name, value: RTSP URL
	deleteCalls []string
	addCalls    []string
	activePaths []video.MediaMtxPathItem
}

func (m *mockMediaMtxClient) AddPath(ctx context.Context, name string, rtspURL string) error {
	m.paths[name] = rtspURL
	m.addCalls = append(m.addCalls, name)
	return nil
}

func (m *mockMediaMtxClient) DeletePath(ctx context.Context, name string) error {
	delete(m.paths, name)
	m.deleteCalls = append(m.deleteCalls, name)
	return nil
}

func (m *mockMediaMtxClient) ListActivePaths(ctx context.Context) ([]video.MediaMtxPathItem, error) {
	return m.activePaths, nil
}

type mockEventPublisher struct {
	discoveredCalls []interface{}
}

func (m *mockEventPublisher) PublishCameraDiscovered(ctx context.Context, payload interface{}) error {
	m.discoveredCalls = append(m.discoveredCalls, payload)
	return nil
}

func (m *mockEventPublisher) Close() {}

func TestVideoService(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	jwtSecret := "test-secret"
	pub := &mockEventPublisher{}

	t.Run("Create Camera Active", func(t *testing.T) {
		repo := &mockRepository{cameras: make(map[string]video.Camera)}
		mtx := &mockMediaMtxClient{paths: make(map[string]string)}
		svc := video.NewService(repo, mtx, pub, jwtSecret, logger)

		req := video.CreateCameraRequest{
			Nom:     "Front Gate",
			URLRTSP: "rtsp://example.com/live",
			Statut:  "active",
		}

		cam, err := svc.CreateCamera(ctx, req)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if cam.Nom != "Front Gate" {
			t.Errorf("expected Front Gate, got %s", cam.Nom)
		}

		// Verify added to MediaMTX
		if mtx.paths[cam.ID] != "rtsp://example.com/live" {
			t.Errorf("expected MediaMTX to have path configured, but not found or incorrect")
		}
	})

	t.Run("Create Camera Inactive", func(t *testing.T) {
		repo := &mockRepository{cameras: make(map[string]video.Camera)}
		mtx := &mockMediaMtxClient{paths: make(map[string]string)}
		svc := video.NewService(repo, mtx, pub, jwtSecret, logger)

		req := video.CreateCameraRequest{
			Nom:     "Back Office",
			URLRTSP: "rtsp://example.com/back",
			Statut:  "inactive",
		}

		cam, err := svc.CreateCamera(ctx, req)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Verify NOT added to MediaMTX
		if _, ok := mtx.paths[cam.ID]; ok {
			t.Errorf("expected inactive camera to NOT be added to MediaMTX")
		}
	})

	t.Run("Update Camera URL and Statut", func(t *testing.T) {
		repo := &mockRepository{cameras: make(map[string]video.Camera)}
		mtx := &mockMediaMtxClient{paths: make(map[string]string)}
		svc := video.NewService(repo, mtx, pub, jwtSecret, logger)

		// Create camera first
		cam, _ := svc.CreateCamera(ctx, video.CreateCameraRequest{
			Nom:     "Cam1",
			URLRTSP: "rtsp://old-url",
			Statut:  "active",
		})

		// Update URL
		_, err := svc.UpdateCamera(ctx, cam.ID, video.UpdateCameraRequest{
			Nom:     "Cam1",
			URLRTSP: "rtsp://new-url",
			Statut:  "active",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if mtx.paths[cam.ID] != "rtsp://new-url" {
			t.Errorf("expected MediaMTX URL to update, got %s", mtx.paths[cam.ID])
		}

		// Deactivate camera
		_, err = svc.UpdateCamera(ctx, cam.ID, video.UpdateCameraRequest{
			Nom:     "Cam1",
			URLRTSP: "rtsp://new-url",
			Statut:  "inactive",
		})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if _, ok := mtx.paths[cam.ID]; ok {
			t.Errorf("expected MediaMTX path to be deleted when inactivated")
		}
	})

	t.Run("Delete Camera", func(t *testing.T) {
		repo := &mockRepository{cameras: make(map[string]video.Camera)}
		mtx := &mockMediaMtxClient{paths: make(map[string]string)}
		svc := video.NewService(repo, mtx, pub, jwtSecret, logger)

		cam, _ := svc.CreateCamera(ctx, video.CreateCameraRequest{
			Nom:     "DeleteMe",
			URLRTSP: "rtsp://url",
			Statut:  "active",
		})

		err := svc.DeleteCamera(ctx, cam.ID)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if _, err := repo.GetByID(ctx, cam.ID); !errors.Is(err, video.ErrCameraNotFound) {
			t.Errorf("expected camera to be deleted from DB, got err: %v", err)
		}

		if _, ok := mtx.paths[cam.ID]; ok {
			t.Errorf("expected MediaMTX path to be deleted when camera is deleted")
		}
	})

	t.Run("Generate and Validate Stream Tokens", func(t *testing.T) {
		repo := &mockRepository{cameras: make(map[string]video.Camera)}
		mtx := &mockMediaMtxClient{paths: make(map[string]string)}
		svc := video.NewService(repo, mtx, pub, jwtSecret, logger)

		cam, _ := svc.CreateCamera(ctx, video.CreateCameraRequest{
			Nom:     "SecureCam",
			URLRTSP: "rtsp://secure",
			Statut:  "active",
		})

		token, err := svc.GenerateStreamToken(ctx, cam.ID)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		// Validation success
		err = svc.ValidateStreamToken(token, cam.ID)
		if err != nil {
			t.Errorf("expected token validation to succeed, got: %v", err)
		}

		// Validation failure (wrong camera ID)
		err = svc.ValidateStreamToken(token, "wrong-cam-id")
		if err == nil {
			t.Errorf("expected token validation to fail for wrong camera ID")
		}
	})

	t.Run("List Active Streams matching DB", func(t *testing.T) {
		repo := &mockRepository{cameras: make(map[string]video.Camera)}
		mtx := &mockMediaMtxClient{paths: make(map[string]string)}
		svc := video.NewService(repo, mtx, pub, jwtSecret, logger)

		cam1, _ := svc.CreateCamera(ctx, video.CreateCameraRequest{Nom: "Cam1", URLRTSP: "rtsp://cam1", Statut: "active"})
		cam2, _ := svc.CreateCamera(ctx, video.CreateCameraRequest{Nom: "Cam2", URLRTSP: "rtsp://cam2", Statut: "active"})

		// MediaMTX reports only cam1 is active, plus some unrelated path "unknown"
		mtx.activePaths = []video.MediaMtxPathItem{
			{
				Name:    cam1.ID,
				Tracks:  []string{"h264"},
				Readers: []interface{}{"reader1"},
			},
			{
				Name:    "unknown-path",
				Tracks:  []string{"opus"},
				Readers: []interface{}{},
			},
		}

		active, err := svc.ListActiveStreams(ctx)
		if err != nil {
			t.Fatalf("failed to list active streams: %v", err)
		}

		if len(active) != 1 {
			t.Fatalf("expected 1 active stream, got %d", len(active))
		}

		if active[0].CameraID != cam1.ID {
			t.Errorf("expected active stream to correspond to %s, got %s", cam1.ID, active[0].CameraID)
		}

		if active[0].Readers != 1 {
			t.Errorf("expected 1 reader, got %d", active[0].Readers)
		}
		
		_ = cam2 // keep compiler happy
	})
}

func TestONVIFCrypto(t *testing.T) {
	key := []byte("super-secret-key-replace-in-production")
	password := "camera-admin-pwd-123!"

	enc, err := video.EncryptPassword(password, key)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	dec, err := video.DecryptPassword(enc, key)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if dec != password {
		t.Errorf("decrypted password does not match original, got %s, expected %s", dec, password)
	}
}

func TestSOAPResponseParsing(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<Envelope xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:tns="http://www.onvif.org/ver10/device/wsdl">
  <Body>
    <GetDeviceInformationResponse>
      <Manufacturer>Test-Manufacturer</Manufacturer>
      <Model>Test-Model-123</Model>
      <FirmwareVersion>2.5.0</FirmwareVersion>
      <SerialNumber>SN987654321</SerialNumber>
      <HardwareId>HW-ABC-DEF</HardwareId>
    </GetDeviceInformationResponse>
  </Body>
</Envelope>`

	var soapResp video.SOAPResponseEnvelope
	err := xml.Unmarshal([]byte(xmlData), &soapResp)
	if err != nil {
		t.Fatalf("failed to unmarshal SOAP: %v", err)
	}

	info := soapResp.Body.GetDeviceInformationResponse
	if info.Manufacturer != "Test-Manufacturer" {
		t.Errorf("expected Test-Manufacturer, got %s", info.Manufacturer)
	}
	if info.Model != "Test-Model-123" {
		t.Errorf("expected Test-Model-123, got %s", info.Model)
	}
}
