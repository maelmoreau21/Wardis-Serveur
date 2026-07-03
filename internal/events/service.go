package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type Service interface {
	Start(ctx context.Context) error
	ListEvents(ctx context.Context) ([]CorrelatedEvent, error)
	SubscribeClient() chan CorrelatedEvent
	UnsubscribeClient(ch chan CorrelatedEvent)
	Close()
}

type service struct {
	repo      Repository
	natsURL   string
	nc        *nats.Conn
	subs      []*nats.Subscription
	clients   map[chan CorrelatedEvent]bool
	clientsMu sync.RWMutex
	log       *zap.Logger
}

func NewService(repo Repository, natsURL string, log *zap.Logger) Service {
	return &service{
		repo:    repo,
		natsURL: natsURL,
		clients: make(map[chan CorrelatedEvent]bool),
		log:     log,
	}
}

func (s *service) Start(ctx context.Context) error {
	s.log.Info("Connecting to NATS for Events Service...", zap.String("url", s.natsURL))
	nc, err := nats.Connect(s.natsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS in events service: %w", err)
	}
	s.nc = nc

	// Subscribe to access.*, alarm.*, video.* subjects
	subjects := []string{"access.*", "alarm.*", "video.*"}
	for _, subject := range subjects {
		sub, err := s.nc.Subscribe(subject, s.handleNatsMessage)
		if err != nil {
			s.Close()
			return fmt.Errorf("failed to subscribe to NATS subject %s: %w", subject, err)
		}
		s.subs = append(s.subs, sub)
		s.log.Info("Successfully subscribed to NATS subject", zap.String("subject", subject))
	}

	return nil
}

func (s *service) ListEvents(ctx context.Context) ([]CorrelatedEvent, error) {
	return s.repo.ListEvents(ctx)
}

func (s *service) SubscribeClient() chan CorrelatedEvent {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	ch := make(chan CorrelatedEvent, 20)
	s.clients[ch] = true
	s.log.Debug("Client subscribed to events stream", zap.Int("total_clients", len(s.clients)))
	return ch
}

func (s *service) UnsubscribeClient(ch chan CorrelatedEvent) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	if _, exists := s.clients[ch]; exists {
		delete(s.clients, ch)
		close(ch)
		s.log.Debug("Client unsubscribed from events stream", zap.Int("total_clients", len(s.clients)))
	}
}

func (s *service) Close() {
	for _, sub := range s.subs {
		_ = sub.Unsubscribe()
	}
	s.subs = nil

	if s.nc != nil {
		s.nc.Close()
		s.nc = nil
	}

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for ch := range s.clients {
		close(ch)
	}
	s.clients = make(map[chan CorrelatedEvent]bool)
}

func (s *service) handleNatsMessage(msg *nats.Msg) {
	s.log.Debug("Received NATS message", zap.String("subject", msg.Subject), zap.Int("size", len(msg.Data)))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var correlated *CorrelatedEvent
	var err error

	if msg.Subject == "alarm.triggered" {
		correlated, err = s.correlateAlarmEvent(ctx, msg.Data)
	} else if strings.HasPrefix(msg.Subject, "access.") {
		correlated, err = s.correlateAccessEvent(ctx, msg.Subject, msg.Data)
	} else if strings.HasPrefix(msg.Subject, "video.") {
		correlated, err = s.correlateVideoEvent(ctx, msg.Subject, msg.Data)
	} else {
		s.log.Warn("Ignoring unknown NATS subject", zap.String("subject", msg.Subject))
		return
	}

	if err != nil {
		s.log.Error("Failed to correlate event", zap.String("subject", msg.Subject), zap.Error(err))
		return
	}

	if correlated == nil {
		return
	}

	// Insert into DB
	saved, err := s.repo.InsertEvent(ctx, correlated)
	if err != nil {
		s.log.Error("Failed to save correlated event to DB", zap.Error(err))
		return
	}

	s.log.Info("Successfully logged and correlated event",
		zap.String("event_id", saved.ID),
		zap.String("type", saved.EventType),
		zap.Time("timestamp", saved.Timestamp),
	)

	// Broadcast to connected clients
	s.broadcast(*saved)
}

func (s *service) correlateAlarmEvent(ctx context.Context, data []byte) (*CorrelatedEvent, error) {
	var payload AlarmEventPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal alarm payload: %w", err)
	}

	// Correlation lookup:
	// 1. Get closest camera in the alarm's zone
	closestCam, err := s.repo.GetClosestCameraForZone(ctx, payload.ZoneID)
	if err != nil {
		s.log.Warn("Error looking up closest camera for zone", zap.String("zone_id", payload.ZoneID), zap.Error(err))
	}

	// 2. Get last badge swipe in the alarm's zone
	lastSwipe, err := s.repo.GetLastBadgeSwipeInZone(ctx, payload.ZoneID)
	if err != nil {
		s.log.Warn("Error looking up last badge swipe for zone", zap.String("zone_id", payload.ZoneID), zap.Error(err))
	}

	correlated := &CorrelatedEvent{
		EventType:   "alarm.triggered",
		SourceID:    payload.AlarmeID,
		ZoneID:      &payload.ZoneID,
		CapteurID:   &payload.CapteurID,
		Timestamp:   payload.Timestamp,
		Details: CorrelationDetails{
			ClosestCamera:  closestCam,
			LastBadgeSwipe: lastSwipe,
			Payload:        payload,
		},
	}

	if closestCam != nil {
		correlated.CameraID = &closestCam.ID
	}

	return correlated, nil
}

func (s *service) correlateAccessEvent(ctx context.Context, subject string, data []byte) (*CorrelatedEvent, error) {
	var payload AccessEventPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal access payload: %w", err)
	}

	var closestCam *ClosestCamera
	var zoneIDStr *string

	if payload.DoorID != nil {
		// 1. Get closest camera for door (monitoring door's zone)
		var err error
		closestCam, err = s.repo.GetClosestCameraForDoor(ctx, *payload.DoorID)
		if err != nil {
			s.log.Warn("Error looking up closest camera for door", zap.String("door_id", *payload.DoorID), zap.Error(err))
		}

		// 2. Get zone ID for door
		zoneID, err := s.repo.GetZoneIDForDoor(ctx, *payload.DoorID)
		if err != nil {
			s.log.Warn("Error looking up zone ID for door", zap.String("door_id", *payload.DoorID), zap.Error(err))
		} else if zoneID != "" {
			zoneIDStr = &zoneID
		}
	}

	correlated := &CorrelatedEvent{
		EventType:   subject,
		SourceID:    payload.LogID,
		DoorID:      payload.DoorID,
		ZoneID:      zoneIDStr,
		BadgeNumber: &payload.BadgeNumber,
		Timestamp:   payload.Timestamp,
		Details: CorrelationDetails{
			ClosestCamera: closestCam,
			Payload:       payload,
		},
	}

	if closestCam != nil {
		correlated.CameraID = &closestCam.ID
	}

	return correlated, nil
}

func (s *service) correlateVideoEvent(ctx context.Context, subject string, data []byte) (*CorrelatedEvent, error) {
	var payload VideoEventPayload
	// Fallback to unmarshalling as a map if the structured payload doesn't fit
	if err := json.Unmarshal(data, &payload); err != nil {
		// If unmarshal fails, we store the raw map
		var rawMap map[string]interface{}
		_ = json.Unmarshal(data, &rawMap)

		return &CorrelatedEvent{
			EventType: subject,
			SourceID:  "00000000-0000-0000-0000-000000000000",
			Timestamp: time.Now(),
			Details: CorrelationDetails{
				Payload: rawMap,
			},
		}, nil
	}

	sourceID := payload.CameraID
	if len(sourceID) != 36 {
		sourceID = "00000000-0000-0000-0000-000000000000"
	}

	var cameraIDPtr *string
	if len(payload.CameraID) == 36 {
		cameraIDPtr = &payload.CameraID
	}

	correlated := &CorrelatedEvent{
		EventType: subject,
		SourceID:  sourceID,
		CameraID:  cameraIDPtr,
		Timestamp: payload.Timestamp,
		Details: CorrelationDetails{
			Payload: payload,
		},
	}

	if payload.Timestamp.IsZero() {
		correlated.Timestamp = time.Now()
	}

	return correlated, nil
}

func (s *service) broadcast(event CorrelatedEvent) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for ch := range s.clients {
		select {
		case ch <- event:
		default:
			s.log.Warn("Client event queue full, dropping event")
		}
	}
}
