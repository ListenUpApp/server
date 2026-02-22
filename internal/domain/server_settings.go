package domain

import "time"

// ServerSettings contains server-wide configuration.
type ServerSettings struct {
	Name         string    `json:"name"`
	InboxEnabled bool      `json:"inbox_enabled"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NewServerSettings creates settings with sensible defaults.
func NewServerSettings() *ServerSettings {
	return &ServerSettings{
		Name:         "",
		InboxEnabled: false,
		UpdatedAt:    time.Now(),
	}
}

// GetDisplayName returns the server name or a default if empty.
func (s *ServerSettings) GetDisplayName() string {
	if s.Name == "" {
		return "ListenUp Server"
	}
	return s.Name
}
