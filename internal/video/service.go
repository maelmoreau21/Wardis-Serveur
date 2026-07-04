package video

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"wardis-server/internal/config"
)

type Service interface {
	ListCameras(ctx context.Context) ([]Camera, error)
	GetCameraByID(ctx context.Context, id string) (*Camera, error)
	CreateCamera(ctx context.Context, req CreateCameraRequest) (*Camera, error)
	UpdateCamera(ctx context.Context, id string, req UpdateCameraRequest) (*Camera, error)
	DeleteCamera(ctx context.Context, id string) error
	GenerateStreamToken(ctx context.Context, cameraID string) (string, error)
	ValidateStreamToken(tokenStr string, cameraID string) error
	ListActiveStreams(ctx context.Context) ([]ActiveStream, error)
	SyncWithMediaMTX(ctx context.Context) error
	DiscoverCameras(ctx context.Context, username, password string, timeout time.Duration) ([]DiscoveredCamera, error)

	// Lifecycle & synchronization methods
	Start(ctx context.Context) error
	Close()
	SyncRecording(ctx context.Context, cameraID string, startTime, endTime time.Time, fileData []byte, filename string) (*VideoRecording, error)
}

type service struct {
	repo        Repository
	mtxClient   MediaMtxClient
	publisher   EventPublisher
	jwtSecret   []byte
	log         *zap.Logger
	cfg         *config.Config
	minioClient *minio.Client
	natsConn    *nats.Conn
	natsSubs    []*nats.Subscription
	stopChan    chan struct{}
}

func NewService(repo Repository, mtxClient MediaMtxClient, publisher EventPublisher, jwtSecret string, log *zap.Logger, cfg *config.Config) Service {
	var minioClient *minio.Client
	var err error
	if cfg != nil && cfg.MinioEndpoint != "" {
		minioClient, err = minio.New(cfg.MinioEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
			Secure: cfg.MinioUseSSL,
		})
		if err != nil {
			log.Error("Failed to initialize MinIO client in video service", zap.Error(err))
		} else {
			log.Info("Successfully initialized MinIO client in video service", zap.String("endpoint", cfg.MinioEndpoint))
		}
	}

	return &service{
		repo:        repo,
		mtxClient:   mtxClient,
		publisher:   publisher,
		jwtSecret:   []byte(jwtSecret),
		log:         log,
		cfg:         cfg,
		minioClient: minioClient,
		stopChan:    make(chan struct{}),
	}
}

func (s *service) ListCameras(ctx context.Context) ([]Camera, error) {
	return s.repo.List(ctx)
}

func (s *service) GetCameraByID(ctx context.Context, id string) (*Camera, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) CreateCamera(ctx context.Context, req CreateCameraRequest) (*Camera, error) {
	if req.Statut == "" {
		req.Statut = "active"
	}

	cam := &Camera{
		Nom:          req.Nom,
		URLRTSP:      req.URLRTSP,
		SiteID:       req.SiteID,
		Statut:       req.Statut,
		IP:           req.IP,
		Port:         req.Port,
		Username:     req.Username,
		PTZSupported: req.PTZSupported,
		ProfileToken: req.ProfileToken,
	}

	if req.Password != nil && *req.Password != "" {
		encPassword, err := EncryptPassword(*req.Password, s.jwtSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt password: %w", err)
		}
		cam.PasswordEncrypted = &encPassword
	}

	created, err := s.repo.Create(ctx, cam)
	if err != nil {
		return nil, err
	}

	if created.Statut == "active" {
		s.log.Info("Adding camera to MediaMTX after creation", zap.String("id", created.ID), zap.String("nom", created.Nom))
		if err := s.mtxClient.AddPath(ctx, created.ID, created.URLRTSP); err != nil {
			s.log.Error("Failed to register path in MediaMTX during camera creation", zap.String("id", created.ID), zap.Error(err))
			// We don't rollback DB creation since camera was successfully created.
			// Startup sync or manual retry will fix MediaMTX registration.
		}
	}

	return created, nil
}

func (s *service) UpdateCamera(ctx context.Context, id string, req UpdateCameraRequest) (*Camera, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	oldStatut := existing.Statut
	oldURL := existing.URLRTSP

	existing.Nom = req.Nom
	existing.URLRTSP = req.URLRTSP
	existing.SiteID = req.SiteID
	existing.Statut = req.Statut
	existing.IP = req.IP
	existing.Port = req.Port
	existing.Username = req.Username
	existing.PTZSupported = req.PTZSupported
	existing.ProfileToken = req.ProfileToken

	if req.Password != nil && *req.Password != "" {
		encPassword, err := EncryptPassword(*req.Password, s.jwtSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt password: %w", err)
		}
		existing.PasswordEncrypted = &encPassword
	}

	updated, err := s.repo.Update(ctx, existing)
	if err != nil {
		return nil, err
	}

	// Update MediaMTX settings based on status change or RTSP URL change
	if oldStatut == "active" && updated.Statut == "inactive" {
		s.log.Info("Removing camera from MediaMTX due to inactivation", zap.String("id", id))
		if err := s.mtxClient.DeletePath(ctx, id); err != nil {
			s.log.Error("Failed to delete path in MediaMTX during camera inactivation", zap.String("id", id), zap.Error(err))
		}
	} else if updated.Statut == "active" {
		if oldStatut == "inactive" || oldURL != updated.URLRTSP {
			s.log.Info("Updating camera path in MediaMTX", zap.String("id", id))
			// Delete and recreate to ensure state is clean
			_ = s.mtxClient.DeletePath(ctx, id)
			if err := s.mtxClient.AddPath(ctx, id, updated.URLRTSP); err != nil {
				s.log.Error("Failed to register path in MediaMTX during camera update", zap.String("id", id), zap.Error(err))
			}
		}
	}

	return updated, nil
}

func (s *service) DeleteCamera(ctx context.Context, id string) error {
	// Verify it exists
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete from MediaMTX first
	s.log.Info("Removing camera from MediaMTX before database deletion", zap.String("id", id))
	if err := s.mtxClient.DeletePath(ctx, id); err != nil {
		s.log.Error("Failed to delete path in MediaMTX during camera deletion", zap.String("id", id), zap.Error(err))
	}

	return s.repo.Delete(ctx, id)
}

func (s *service) GenerateStreamToken(ctx context.Context, cameraID string) (string, error) {
	_, err := s.repo.GetByID(ctx, cameraID)
	if err != nil {
		return "", err
	}

	claims := &StreamClaims{
		CameraID: cameraID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)), // 5 minutes lifetime
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *service) ValidateStreamToken(tokenStr string, cameraID string) error {
	token, err := jwt.ParseWithClaims(tokenStr, &StreamClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return err
	}

	claims, ok := token.Claims.(*StreamClaims)
	if !ok || !token.Valid {
		return fmt.Errorf("invalid token claims or token is expired")
	}

	if claims.CameraID != cameraID {
		return fmt.Errorf("token camera ID '%s' does not match requested camera ID '%s'", claims.CameraID, cameraID)
	}

	return nil
}

func (s *service) ListActiveStreams(ctx context.Context) ([]ActiveStream, error) {
	mtxPaths, err := s.mtxClient.ListActivePaths(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch active paths from MediaMTX: %w", err)
	}

	cameras, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list cameras: %w", err)
	}

	camMap := make(map[string]Camera)
	for _, cam := range cameras {
		camMap[cam.ID] = cam
	}

	var activeStreams []ActiveStream
	for _, item := range mtxPaths {
		cam, ok := camMap[item.Name]
		if !ok {
			// Path name in MediaMTX doesn't match any camera UUID, ignore or list it with default name
			continue
		}

		activeStreams = append(activeStreams, ActiveStream{
			CameraID:  cam.ID,
			CameraNom: cam.Nom,
			Tracks:    item.Tracks,
			Readers:   len(item.Readers),
			Statut:    cam.Statut,
		})
	}

	return activeStreams, nil
}

func (s *service) SyncWithMediaMTX(ctx context.Context) error {
	s.log.Info("Starting synchronization of cameras with MediaMTX...")
	cameras, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch cameras during sync: %w", err)
	}

	for _, cam := range cameras {
		if cam.Statut == "active" {
			s.log.Info("Sync: adding camera path to MediaMTX", zap.String("id", cam.ID), zap.String("nom", cam.Nom))
			// Delete to clear, then recreate
			_ = s.mtxClient.DeletePath(ctx, cam.ID)
			if err := s.mtxClient.AddPath(ctx, cam.ID, cam.URLRTSP); err != nil {
				s.log.Error("Sync: failed to add camera path", zap.String("id", cam.ID), zap.Error(err))
			}
		} else {
			s.log.Info("Sync: removing camera path from MediaMTX", zap.String("id", cam.ID), zap.String("nom", cam.Nom))
			if err := s.mtxClient.DeletePath(ctx, cam.ID); err != nil {
				s.log.Error("Sync: failed to delete camera path", zap.String("id", cam.ID), zap.Error(err))
			}
		}
	}

	s.log.Info("MediaMTX synchronization completed")
	return nil
}

func (s *service) DiscoverCameras(ctx context.Context, username, password string, timeout time.Duration) ([]DiscoveredCamera, error) {
	s.log.Info("Starting ONVIF WS-Discovery on local network", zap.Duration("timeout", timeout))
	devices, err := DiscoverONVIFDevices(ctx, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to discover ONVIF devices: %w", err)
	}

	s.log.Info("WS-Discovery completed", zap.Int("device_count", len(devices)))

	var discovered []DiscoveredCamera

	for _, dev := range devices {
		s.log.Info("Discovered camera NVT endpoint", zap.String("ip", dev.IP), zap.Int("port", dev.Port), zap.String("xaddr", dev.XAddr))

		model := "ONVIF Camera"
		streamURL := ""
		ptzSupported := false
		profileToken := ""

		// Step 1: Get Device Information via SOAP
		devInfoReq := GetDeviceInformation{
			Xmlns: "http://www.onvif.org/ver10/device/wsdl",
		}
		devInfoResp, err := SendSOAPRequest(ctx, dev.XAddr, devInfoReq, username, password)
		if err != nil {
			s.log.Warn("Failed to fetch device information via SOAP", zap.String("ip", dev.IP), zap.Error(err))
		} else {
			model = devInfoResp.Body.GetDeviceInformationResponse.Model
			if model == "" {
				model = "ONVIF Camera"
			}
		}

		// Step 2: Get Capabilities via SOAP
		capReq := GetCapabilities{
			Xmlns:    "http://www.onvif.org/ver10/device/wsdl",
			Category: "All",
		}
		capResp, err := SendSOAPRequest(ctx, dev.XAddr, capReq, username, password)
		if err != nil {
			s.log.Warn("Failed to fetch capabilities via SOAP", zap.String("ip", dev.IP), zap.Error(err))
		} else {
			mediaURL := capResp.Body.GetCapabilitiesResponse.Capabilities.Media.XAddr
			ptzURL := capResp.Body.GetCapabilitiesResponse.Capabilities.PTZ.XAddr

			if ptzURL != "" {
				ptzSupported = true
			}

			if mediaURL != "" {
				// Step 3: Get Profiles via Media Service
				profReq := GetProfiles{
					Xmlns: "http://www.onvif.org/ver10/media/wsdl",
				}
				profResp, err := SendSOAPRequest(ctx, mediaURL, profReq, username, password)
				if err != nil {
					s.log.Warn("Failed to fetch media profiles via SOAP", zap.String("ip", dev.IP), zap.Error(err))
				} else if len(profResp.Body.GetProfilesResponse.Profiles) > 0 {
					profileToken = profResp.Body.GetProfilesResponse.Profiles[0].Token

					// Step 4: Get Stream URI for the first profile
					streamUriReq := GetStreamUri{
						XmlnsTrt: "http://www.onvif.org/ver10/media/wsdl",
						XmlnsTt:  "http://www.onvif.org/ver10/schema",
						ProfileToken: profileToken,
					}
					streamUriReq.StreamSetup.Stream = "RTP-Unicast"
					streamUriReq.StreamSetup.Transport.Protocol = "RTSP"

					uriResp, err := SendSOAPRequest(ctx, mediaURL, streamUriReq, username, password)
					if err != nil {
						s.log.Warn("Failed to fetch stream URI via SOAP", zap.String("ip", dev.IP), zap.Error(err))
					} else {
						streamURL = uriResp.Body.GetStreamUriResponse.MediaUri.Uri
					}
				}
			}
		}

		camResult := DiscoveredCamera{
			IP:           dev.IP,
			Port:         dev.Port,
			EndpointURL:  dev.XAddr,
			DeviceToken:  dev.EndpointReference,
			Model:        model,
			StreamURL:    streamURL,
			PTZSupported: ptzSupported,
			ProfileToken: profileToken,
		}

		discovered = append(discovered, camResult)

		// Step 5: Publish event to NATS
		eventPayload := DiscoveredCameraEvent{
			IP:        dev.IP,
			Model:     model,
			StreamURL: streamURL,
			Timestamp: time.Now(),
		}
		if s.publisher != nil {
			if err := s.publisher.PublishCameraDiscovered(ctx, eventPayload); err != nil {
				s.log.Error("Failed to publish cameras.discovered event to NATS", zap.Error(err))
			} else {
				s.log.Info("Successfully published cameras.discovered event to NATS", zap.String("ip", dev.IP))
			}
		}
	}

	return discovered, nil
}
