package video

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Camera struct {
	ID                string    `json:"id"`
	Nom               string    `json:"nom"`
	URLRTSP           string    `json:"url_rtsp"`
	MainStreamURL     string    `json:"main_stream_url"`
	SubStreamURL      string    `json:"sub_stream_url"`
	SiteID            *string   `json:"site_id,omitempty"`
	Statut            string    `json:"statut"` // "active" or "inactive"
	CreatedAt         time.Time `json:"created_at"`
	IP                *string   `json:"ip,omitempty"`
	Port              *int      `json:"port,omitempty"`
	Username          *string   `json:"username,omitempty"`
	PasswordEncrypted *string   `json:"password_encrypted,omitempty"`
	PTZSupported      bool      `json:"ptz_supported"`
	ProfileToken      *string   `json:"profile_token,omitempty"`
	RecordingMode     string    `json:"recording_mode"` // "continuous", "motion", "both", "none"
	RetentionDays     int       `json:"retention_days"`
}

type CreateCameraRequest struct {
	Nom           string  `json:"nom"`
	URLRTSP       string  `json:"url_rtsp"`
	MainStreamURL string  `json:"main_stream_url"`
	SubStreamURL  string  `json:"sub_stream_url"`
	SiteID        *string `json:"site_id,omitempty"`
	Statut        string  `json:"statut"`
	IP            *string `json:"ip,omitempty"`
	Port          *int    `json:"port,omitempty"`
	Username      *string `json:"username,omitempty"`
	Password      *string `json:"password,omitempty"`
	PTZSupported  bool    `json:"ptz_supported"`
	ProfileToken  *string `json:"profile_token,omitempty"`
	RecordingMode string  `json:"recording_mode"`
	RetentionDays int     `json:"retention_days"`
}

type UpdateCameraRequest struct {
	Nom           string  `json:"nom"`
	URLRTSP       string  `json:"url_rtsp"`
	MainStreamURL string  `json:"main_stream_url"`
	SubStreamURL  string  `json:"sub_stream_url"`
	SiteID        *string `json:"site_id,omitempty"`
	Statut        string  `json:"statut"`
	IP            *string `json:"ip,omitempty"`
	Port          *int    `json:"port,omitempty"`
	Username      *string `json:"username,omitempty"`
	Password      *string `json:"password,omitempty"`
	PTZSupported  bool    `json:"ptz_supported"`
	ProfileToken  *string `json:"profile_token,omitempty"`
	RecordingMode string  `json:"recording_mode"`
	RetentionDays int     `json:"retention_days"`
}

type DiscoverCamerasRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Timeout  int    `json:"timeout_seconds"`
}

type DiscoveredCamera struct {
	IP            string `json:"ip"`
	Port          int    `json:"port"`
	EndpointURL   string `json:"endpoint_url"`
	DeviceToken   string `json:"device_token"`
	Model         string `json:"model"`
	StreamURL     string `json:"stream_url"`
	MainStreamURL string `json:"main_stream_url"`
	SubStreamURL  string `json:"sub_stream_url"`
	PTZSupported  bool   `json:"ptz_supported"`
	ProfileToken  string `json:"profile_token"`
}

type DiscoveredCameraEvent struct {
	IP            string    `json:"ip"`
	Model         string    `json:"model"`
	StreamURL     string    `json:"stream_url"`
	MainStreamURL string    `json:"main_stream_url"`
	SubStreamURL  string    `json:"sub_stream_url"`
	Timestamp     time.Time `json:"timestamp"`
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

type VideoRecording struct {
	ID          string    `json:"id"`
	CameraID    string    `json:"camera_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Filepath    string    `json:"filepath"`
	StorageType string    `json:"storage_type"` // "local" or "cloud"
	FileSize    int64     `json:"file_size"`
	CreatedAt   time.Time `json:"created_at"`
}

type SyncRecordingPayload struct {
	CameraID     string    `json:"camera_id"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	VideoDataB64 string    `json:"video_data_b64"`
}

type Bookmark struct {
	ID        string    `json:"id"`
	CameraID  string    `json:"camera_id"`
	Name      string    `json:"name"`
	Notes     string    `json:"notes"`
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateBookmarkRequest struct {
	Name      string    `json:"name"`
	Notes     string    `json:"notes"`
	Timestamp time.Time `json:"timestamp"`
}

type UpdateBookmarkRequest struct {
	Name  string `json:"name"`
	Notes string `json:"notes"`
}

type VideoEventPayload struct {
	CameraID  string    `json:"camera_id"`
	EventType string    `json:"event_type"` // e.g. "video.motion"
	Timestamp time.Time `json:"timestamp"`
	Details   *string   `json:"details,omitempty"`
}
