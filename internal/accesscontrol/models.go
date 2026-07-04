package accesscontrol

import (
	"time"
)

type Site struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Door struct {
	ID          string    `json:"id"`
	SiteID      string    `json:"site_id"`
	ZoneID      *string   `json:"zone_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // "open" or "closed"
	CreatedAt   time.Time `json:"created_at"`
}

type Badge struct {
	ID           string    `json:"id"`
	Number       string    `json:"number"`
	UserID       *string   `json:"user_id,omitempty"`
	CardholderID *string   `json:"cardholder_id,omitempty"`
	Status       string    `json:"status"` // "active" or "revoked"
	CreatedAt    time.Time `json:"created_at"`
}

type AccessPermission struct {
	ID        string    `json:"id"`
	BadgeID   string    `json:"badge_id"`
	DoorID    string    `json:"door_id"`
	Allowed   bool      `json:"allowed"`
	CreatedAt time.Time `json:"created_at"`
}

type AccessLog struct {
	ID              string    `json:"id"`
	BadgeID         *string   `json:"badge_id,omitempty"`
	BadgeNumber     string    `json:"badge_number"`
	DoorID          *string   `json:"door_id,omitempty"`
	SiteID          *string   `json:"site_id,omitempty"`
	UserID          *string   `json:"user_id,omitempty"`
	CardholderID    *string   `json:"cardholder_id,omitempty"`
	CardholderName  *string   `json:"cardholder_name,omitempty"`
	CardholderPhoto *string   `json:"cardholder_photo,omitempty"`
	AccessType      string    `json:"access_type"` // "granted" or "denied"
	DeniedReason    *string   `json:"denied_reason,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type Cardholder struct {
	ID          string    `json:"id"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	Company     string    `json:"company"`
	Email       string    `json:"email"`
	Photo       string    `json:"photo"`
	AccessGroup string    `json:"access_group"`
	Schedule    string    `json:"schedule"`
	BadgeNumber string    `json:"badge_number,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CardholderRequest struct {
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Company     string `json:"company"`
	Email       string `json:"email"`
	Photo       string `json:"photo"`
	AccessGroup string `json:"access_group"`
	Schedule    string `json:"schedule"`
	BadgeNumber string `json:"badge_number"`
}

type AssignBadgeRequest struct {
	BadgeNumber string `json:"badge_number"`
	UserID      string `json:"user_id"`
}

type SwipeBadgeRequest struct {
	BadgeNumber string `json:"badge_number"`
}

type SwipeBadgeResponse struct {
	AccessGranted bool   `json:"access_granted"`
	Message       string `json:"message"`
}
