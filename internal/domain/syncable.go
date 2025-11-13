package domain

import "time"

// Syncable provides common fields for entitites that participate in synchronization.
// This gets embedded in in any domain type that gets synched to keep things (hopefully) simple.
type Syncable struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Touch updates the UpdatedAt timestamp to the current timeime.
// Call this whenever the underlying entity changes.
func (s *Syncable) Touch() {
	s.UpdatedAt = time.Now()
}

// InitTimestamps sets both CreatedAt and UpdatedAt to now.
// Call this when creeating a new entity
func (s *Syncable) InitTimestamps() {
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
}
