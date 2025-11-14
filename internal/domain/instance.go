package domain

import "time"

// Instance represents the singleton server instance configuration.
type Instance struct {
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ID          string    `json:"id"`
	HasRootUser bool      `json:"has_root_user"`
}
