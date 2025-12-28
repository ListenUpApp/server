package domain

import "time"

// Syncable provides common fields for entitites that participate in synchronization.
// This gets embedded in in any domain type that gets synched to keep things (hopefully) simple.
type Syncable struct {
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	ID        string     `json:"id"`
}

// Touch updates the UpdatedAt timestamp to the current time.
// Call this whenever the underlying entity changes.
func (s *Syncable) Touch() {
	s.UpdatedAt = time.Now()
}

// InitTimestamps sets both CreatedAt and UpdatedAt to now.
// Call this when creating a new entity.
func (s *Syncable) InitTimestamps() {
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
}

// IsDeleted returns true if this entity has been soft-deleted.
func (s *Syncable) IsDeleted() bool {
	return s.DeletedAt != nil
}

// MarkDeleted marks this entity as soft-deleted by setting DeletedAt to now.
// This also updates UpdatedAt so the deletion appears in delta sync queries.
func (s *Syncable) MarkDeleted() {
	now := time.Now()
	s.DeletedAt = &now
	s.UpdatedAt = now
}
