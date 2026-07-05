package layout

import (
	"time"
)

type SavedView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	GridLayout string    `json:"grid_layout"`
	Slots      string    `json:"slots"` // JSON representation of grid slots
	UserID     *string   `json:"user_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SavedViewRequest struct {
	Name       string `json:"name"`
	GridLayout string `json:"grid_layout"`
	Slots      string `json:"slots"` // JSON array string e.g. [{"slot_index": 0, "camera_id": "..."}]
}
