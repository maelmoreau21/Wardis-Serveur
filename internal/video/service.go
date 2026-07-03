package video

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
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
}

type service struct {
	repo       Repository
	mtxClient  MediaMtxClient
	jwtSecret  []byte
	log        *zap.Logger
}

func NewService(repo Repository, mtxClient MediaMtxClient, jwtSecret string, log *zap.Logger) Service {
	return &service{
		repo:       repo,
		mtxClient:  mtxClient,
		jwtSecret:  []byte(jwtSecret),
		log:        log,
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
		Nom:     req.Nom,
		URLRTSP: req.URLRTSP,
		SiteID:  req.SiteID,
		Statut:  req.Statut,
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
