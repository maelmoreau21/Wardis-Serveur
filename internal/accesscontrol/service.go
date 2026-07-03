package accesscontrol

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
)

type Service interface {
	ListDoors(ctx context.Context) ([]Door, error)
	AssignBadge(ctx context.Context, number string, userID string) (*Badge, error)
	OpenDoor(ctx context.Context, id string) error
	CloseDoor(ctx context.Context, id string) error
	SwipeBadge(ctx context.Context, doorID string, number string) (*SwipeBadgeResponse, error)
	ListAccessLogs(ctx context.Context) ([]AccessLog, error)
}

type service struct {
	repo      Repository
	publisher EventPublisher
	log       *zap.Logger
}

func NewService(repo Repository, publisher EventPublisher, log *zap.Logger) Service {
	return &service{
		repo:      repo,
		publisher: publisher,
		log:       log,
	}
}

func (s *service) ListDoors(ctx context.Context) ([]Door, error) {
	return s.repo.ListDoors(ctx)
}

func (s *service) AssignBadge(ctx context.Context, number string, userID string) (*Badge, error) {
	if number == "" {
		return nil, errors.New("badge number is required")
	}
	if userID == "" {
		return nil, errors.New("user ID is required")
	}
	return s.repo.AssignBadge(ctx, number, userID)
}

func (s *service) OpenDoor(ctx context.Context, id string) error {
	return s.repo.UpdateDoorStatus(ctx, id, "open")
}

func (s *service) CloseDoor(ctx context.Context, id string) error {
	return s.repo.UpdateDoorStatus(ctx, id, "closed")
}

func (s *service) SwipeBadge(ctx context.Context, doorID string, number string) (*SwipeBadgeResponse, error) {
	var siteID *string
	var doorName string

	// 1. Resolve Door
	door, err := s.repo.GetDoorByID(ctx, doorID)
	if err != nil {
		if errors.Is(err, ErrDoorNotFound) {
			// Even if door is not found, we cannot map site ID or door ID.
			// However, swipe cannot proceed because we don't have door. Return error immediately.
			return &SwipeBadgeResponse{
				AccessGranted: false,
				Message:       "door not found",
			}, nil
		}
		return nil, fmt.Errorf("failed to get door during badge swipe: %w", err)
	}

	siteID = &door.SiteID
	doorName = door.Name

	// 2. Resolve Badge
	badge, err := s.repo.GetBadgeByNumber(ctx, number)
	if err != nil {
		if errors.Is(err, ErrBadgeNotFound) {
			reason := "badge not found"
			logRecord, logErr := s.repo.CreateAccessLog(ctx, &AccessLog{
				BadgeNumber:  number,
				DoorID:       &doorID,
				SiteID:       siteID,
				AccessType:   "denied",
				DeniedReason: &reason,
			})
			if logErr == nil {
				s.publishNatsEvent(ctx, false, logRecord)
			}
			return &SwipeBadgeResponse{
				AccessGranted: false,
				Message:       "access denied: badge not registered",
			}, nil
		}
		return nil, fmt.Errorf("failed to check badge during badge swipe: %w", err)
	}

	// 3. Check Badge Status
	if badge.Status != "active" {
		reason := fmt.Sprintf("badge is %s", badge.Status)
		logRecord, logErr := s.repo.CreateAccessLog(ctx, &AccessLog{
			BadgeID:      &badge.ID,
			BadgeNumber:  badge.Number,
			DoorID:       &doorID,
			SiteID:       siteID,
			UserID:       badge.UserID,
			AccessType:   "denied",
			DeniedReason: &reason,
		})
		if logErr == nil {
			s.publishNatsEvent(ctx, false, logRecord)
		}
		return &SwipeBadgeResponse{
			AccessGranted: false,
			Message:       "access denied: badge is inactive",
		}, nil
	}

	// 4. Check Access Permissions
	allowed, err := s.repo.CheckAccessPermission(ctx, badge.ID, doorID)
	if err != nil {
		return nil, fmt.Errorf("failed to check permissions during badge swipe: %w", err)
	}

	if !allowed {
		reason := "insufficient access permissions for door " + doorName
		logRecord, logErr := s.repo.CreateAccessLog(ctx, &AccessLog{
			BadgeID:      &badge.ID,
			BadgeNumber:  badge.Number,
			DoorID:       &doorID,
			SiteID:       siteID,
			UserID:       badge.UserID,
			AccessType:   "denied",
			DeniedReason: &reason,
		})
		if logErr == nil {
			s.publishNatsEvent(ctx, false, logRecord)
		}
		return &SwipeBadgeResponse{
			AccessGranted: false,
			Message:       "access denied: access not allowed",
		}, nil
	}

	// 5. Access Granted
	logRecord, logErr := s.repo.CreateAccessLog(ctx, &AccessLog{
		BadgeID:    &badge.ID,
		BadgeNumber: badge.Number,
		DoorID:     &doorID,
		SiteID:     siteID,
		UserID:     badge.UserID,
		AccessType: "granted",
	})
	if logErr != nil {
		s.log.Error("failed to create access log on success", zap.Error(logErr))
	} else {
		s.publishNatsEvent(ctx, true, logRecord)
	}

	return &SwipeBadgeResponse{
		AccessGranted: true,
		Message:       "access granted",
	}, nil
}

func (s *service) ListAccessLogs(ctx context.Context) ([]AccessLog, error) {
	return s.repo.ListAccessLogs(ctx)
}

func (s *service) publishNatsEvent(ctx context.Context, granted bool, log *AccessLog) {
	payload := map[string]interface{}{
		"log_id":        log.ID,
		"badge_number":  log.BadgeNumber,
		"door_id":       log.DoorID,
		"site_id":       log.SiteID,
		"user_id":       log.UserID,
		"access_type":   log.AccessType,
		"denied_reason": log.DeniedReason,
		"timestamp":     log.CreatedAt,
	}

	var err error
	if granted {
		err = s.publisher.PublishAccessGranted(ctx, payload)
	} else {
		err = s.publisher.PublishAccessDenied(ctx, payload)
	}

	if err != nil {
		s.log.Error("failed to publish NATS access event",
			zap.Bool("granted", granted),
			zap.String("badge_number", log.BadgeNumber),
			zap.Error(err),
		)
	} else {
		s.log.Info("successfully published NATS access event",
			zap.Bool("granted", granted),
			zap.String("badge_number", log.BadgeNumber),
		)
	}
}
