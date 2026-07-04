package events

import (
	"time"
)

type ClosestCamera struct {
	ID      string `json:"id"`
	Nom     string `json:"nom"`
	URLRTSP string `json:"url_rtsp"`
	Statut  string `json:"statut"`
}

type LastBadgeSwipe struct {
	LogID           string     `json:"log_id"`
	BadgeNumber     string     `json:"badge_number"`
	DoorID          *string    `json:"door_id,omitempty"`
	UserID          *string    `json:"user_id,omitempty"`
	CardholderID    *string    `json:"cardholder_id,omitempty"`
	CardholderName  *string    `json:"cardholder_name,omitempty"`
	CardholderPhoto *string    `json:"cardholder_photo,omitempty"`
	AccessType      string     `json:"access_type"` // "granted" or "denied"
	Timestamp       time.Time  `json:"timestamp"`
}

type CorrelationDetails struct {
	ClosestCamera  *ClosestCamera  `json:"closest_camera,omitempty"`
	LastBadgeSwipe *LastBadgeSwipe `json:"last_badge_swipe,omitempty"`
	Payload        interface{}     `json:"payload"`
}

type CorrelatedEvent struct {
	ID          string             `json:"id"`
	EventType   string             `json:"event_type"` // e.g. "alarm.triggered", "access.granted"
	SourceID    string             `json:"source_id"`
	ZoneID      *string            `json:"zone_id,omitempty"`
	CapteurID   *string            `json:"capteur_id,omitempty"`
	DoorID      *string            `json:"door_id,omitempty"`
	BadgeNumber *string            `json:"badge_number,omitempty"`
	CameraID    *string            `json:"camera_id,omitempty"`
	Timestamp   time.Time          `json:"timestamp"`
	Details     CorrelationDetails `json:"details"`
	CreatedAt   time.Time          `json:"created_at"`
}

// NATS payload structures:

type AlarmEventPayload struct {
	AlarmeID  string    `json:"alarme_id"`
	ZoneID    string    `json:"zone_id"`
	CapteurID string    `json:"capteur_id"`
	Statut    string    `json:"statut"`
	Timestamp time.Time `json:"timestamp"`
}

type AccessEventPayload struct {
	LogID           string    `json:"log_id"`
	BadgeNumber     string    `json:"badge_number"`
	DoorID          *string   `json:"door_id,omitempty"`
	SiteID          *string   `json:"site_id,omitempty"`
	UserID          *string   `json:"user_id,omitempty"`
	CardholderID    *string   `json:"cardholder_id,omitempty"`
	CardholderName  *string   `json:"cardholder_name,omitempty"`
	CardholderPhoto *string   `json:"cardholder_photo,omitempty"`
	AccessType      string    `json:"access_type"` // "granted" or "denied"
	DeniedReason    *string   `json:"denied_reason,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
}

type VideoEventPayload struct {
	CameraID  string    `json:"camera_id"`
	EventType string    `json:"event_type"`
	Timestamp time.Time `json:"timestamp"`
	Details   *string   `json:"details,omitempty"`
}
