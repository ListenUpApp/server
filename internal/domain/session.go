package domain

import "time"

// SessionStaleThreshold defines when an inactive session is considered stale (~6 months).
const SessionStaleThreshold = 6 * 30 * 24 * time.Hour

// BookReadingSession tracks a user's reading attempt of a book.
// Users can have multiple sessions for the same book (re-reads).
// Sessions track when they started, if they finished, and how much time was spent.
type BookReadingSession struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	BookID        string     `json:"book_id"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	IsCompleted   bool       `json:"is_completed"`   // True = finished 99%+
	FinalProgress float64    `json:"final_progress"` // 0.0-1.0 when ended
	ListenTimeMs  int64      `json:"listen_time_ms"` // Accumulated in session
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// NewBookReadingSession creates a new reading session.
func NewBookReadingSession(id, userID, bookID string) *BookReadingSession {
	now := time.Now()
	return &BookReadingSession{
		ID:            id,
		UserID:        userID,
		BookID:        bookID,
		StartedAt:     now,
		FinishedAt:    nil,
		IsCompleted:   false,
		FinalProgress: 0.0,
		ListenTimeMs:  0,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// IsActive returns true if the session is still active (not finished).
func (s *BookReadingSession) IsActive() bool {
	return s.FinishedAt == nil
}

// IsStale returns true if the session has been inactive for more than 6 months.
// An active session is stale if it hasn't been updated in 6 months.
// An inactive session is stale if it was finished more than 6 months ago.
func (s *BookReadingSession) IsStale(now time.Time) bool {
	if s.IsActive() {
		// Active session: stale if not updated in 6 months
		return now.Sub(s.UpdatedAt) > SessionStaleThreshold
	}
	// Inactive session: stale if finished more than 6 months ago
	return s.FinishedAt != nil && now.Sub(*s.FinishedAt) > SessionStaleThreshold
}

// MarkCompleted marks the session as completed (user finished the book at 99%+).
func (s *BookReadingSession) MarkCompleted(progress float64, listenTimeMs int64) {
	now := time.Now()
	s.FinishedAt = &now
	s.IsCompleted = true
	s.FinalProgress = progress
	s.ListenTimeMs = listenTimeMs
	s.UpdatedAt = now
}

// MarkAbandoned marks the session as abandoned (user stopped before 99%).
func (s *BookReadingSession) MarkAbandoned(progress float64, listenTimeMs int64) {
	now := time.Now()
	s.FinishedAt = &now
	s.IsCompleted = false
	s.FinalProgress = progress
	s.ListenTimeMs = listenTimeMs
	s.UpdatedAt = now
}

// UpdateProgress updates the accumulated listen time and UpdatedAt timestamp.
// Call this periodically as the user listens to keep the session fresh.
func (s *BookReadingSession) UpdateProgress(listenTimeMs int64) {
	s.ListenTimeMs = listenTimeMs
	s.UpdatedAt = time.Now()
}
