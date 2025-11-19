package domain

import "time"

// Instance represents the singleton server instance configuration.
type Instance struct {
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	LocalURL   string    `json:"local_url,omitempty"`
	RemoteURL  string    `json:"remote_url,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	ID         string    `json:"id"`
	RootUserID string    `json:"root_user_id,omitempty"`
}

// IsSetupRequired returns true if the server needs initial setup.
// Setup is required when no root user has been configured.
func (i *Instance) IsSetupRequired() bool {
	return i.RootUserID == ""
}

// SetRootUser marks the instance as configured with a root user.
func (i *Instance) SetRootUser(userID string) {
	i.RootUserID = userID
	i.UpdatedAt = time.Now()
}
