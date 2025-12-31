package domain

import "time"

// ServerSettings contains server-wide configuration.
// Stored as a single key in Badger.
type ServerSettings struct {
	InboxEnabled bool      `json:"inbox_enabled"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NewServerSettings creates settings with sensible defaults.
func NewServerSettings() *ServerSettings {
	return &ServerSettings{
		InboxEnabled: false,
		UpdatedAt:    time.Now(),
	}
}
