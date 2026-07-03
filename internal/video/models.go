package video

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Camera struct {
	ID        string    `json:"id"`
	Nom       string    `json:"nom"`
	URLRTSP   string    `json:"url_rtsp"`
	SiteID    *string   `json:"site_id,omitempty"`
	Statut    string    `json:"statut"` // "active" or "inactive"
	CreatedAt time.Time `json:"created_at"`
}

type CreateCameraRequest struct {
	Nom     string  `json:"nom"`
	URLRTSP string  `json:"url_rtsp"`
	SiteID  *string `json:"site_id,omitempty"`
	Statut  string  `json:"statut"`
}

type UpdateCameraRequest struct {
	Nom     string  `json:"nom"`
	URLRTSP string  `json:"url_rtsp"`
	SiteID  *string `json:"site_id,omitempty"`
	Statut  string  `json:"statut"`
}

type StreamClaims struct {
	CameraID string `json:"camera_id"`
	jwt.RegisteredClaims
}

type MediaMtxAuthRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
	IP       string `json:"ip"`
	Action   string `json:"action"` // "publish", "read", etc.
	Path     string `json:"path"`   // e.g. the camera UUID
	Protocol string `json:"protocol"`
	ID       string `json:"id"`
	Query    string `json:"query"` // contains "jwt=..." or "token=..."
}

type MediaMtxPathItem struct {
	Name    string                 `json:"name"`
	Source  map[string]interface{} `json:"source"`
	Tracks  []string               `json:"tracks"`
	Readers []interface{}          `json:"readers"`
}

type MediaMtxActivePathsResponse struct {
	PageCount int                `json:"pageCount"`
	Items     []MediaMtxPathItem `json:"items"`
}

type ActiveStream struct {
	CameraID  string    `json:"camera_id"`
	CameraNom string    `json:"camera_nom"`
	Tracks    []string  `json:"tracks"`
	Readers   int       `json:"readers_count"`
	Statut    string    `json:"statut"`
}
