package abs

import (
	"time"

	"github.com/google/uuid"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// Converter transforms ABS data into ListenUp entities.
// The conversion is lossy by nature - ABS stores different data than ListenUp.
type Converter struct{}

// NewConverter creates a new converter.
func NewConverter() *Converter {
	return &Converter{}
}

// SessionToEvent converts an ABS listening session to a ListenUp listening event.
// The mapping is straightforward since both represent a listening span.
//
// ABS Session has:
//   - StartTime/CurrentTime: positions in seconds
//   - Duration/TimeListening: duration in seconds
//   - StartedAt/UpdatedAt: timestamps in Unix ms
//
// ListenUp ListeningEvent has:
//   - StartPositionMs/EndPositionMs: positions in milliseconds
//   - DurationMs: computed from positions
//   - StartedAt/EndedAt: time.Time
func (c *Converter) SessionToEvent(
	session *Session,
	listenUpUserID string,
	listenUpBookID string,
) *domain.ListeningEvent {
	// Generate a new ID for the event (we don't preserve ABS IDs)
	eventID := uuid.NewString()

	// Convert positions from seconds to milliseconds
	startPositionMs := session.StartPositionMs()
	endPositionMs := session.EndPositionMs()

	// Convert timestamps
	startedAt := session.StartedAtTime()
	endedAt := session.EndedAtTime()

	// Calculate duration - use the actual listen duration, not position delta
	// This handles cases where user rewound during session
	durationMs := session.DurationMs()

	return &domain.ListeningEvent{
		ID:              eventID,
		UserID:          listenUpUserID,
		BookID:          listenUpBookID,
		StartPositionMs: startPositionMs,
		EndPositionMs:   endPositionMs,
		StartedAt:       startedAt,
		EndedAt:         endedAt,
		PlaybackSpeed:   1.0, // ABS doesn't store playback speed per session
		DeviceID:        "abs-import",
		DeviceName:      "Audiobookshelf Import",
		DurationMs:      durationMs,
		CreatedAt:       time.Now(),
	}
}

// ProgressToEvents creates synthetic listening events from ABS MediaProgress.
// This is used when session history is unavailable but we want to preserve
// the user's current position and completion state.
//
// We create a single synthetic event that represents the accumulated progress.
// This is imperfect but preserves the essential state.
func (c *Converter) ProgressToEvents(
	progress *MediaProgress,
	listenUpUserID string,
	listenUpBookID string,
) []*domain.ListeningEvent {
	if progress.CurrentTime <= 0 {
		return nil // No meaningful progress to import
	}

	eventID := uuid.NewString()

	// Convert current position from seconds to milliseconds
	currentPositionMs := int64(progress.CurrentTime * 1000)

	// Use progress timestamps
	startedAt := time.UnixMilli(progress.StartedAt)
	lastUpdate := progress.LastUpdateTime()

	// Create a synthetic event from start to current position
	// This isn't historically accurate but captures the state
	return []*domain.ListeningEvent{{
		ID:              eventID,
		UserID:          listenUpUserID,
		BookID:          listenUpBookID,
		StartPositionMs: 0,
		EndPositionMs:   currentPositionMs,
		StartedAt:       startedAt,
		EndedAt:         lastUpdate,
		PlaybackSpeed:   1.0,
		DeviceID:        "abs-import-progress",
		DeviceName:      "Audiobookshelf Progress Import",
		DurationMs:      currentPositionMs, // Approximate
		CreatedAt:       time.Now(),
	}}
}

// ConvertSessions converts all sessions for a user+book pair.
// Returns events sorted by StartedAt for proper replay.
func (c *Converter) ConvertSessions(
	sessions []Session,
	listenUpUserID string,
	listenUpBookID string,
) []*domain.ListeningEvent {
	events := make([]*domain.ListeningEvent, 0, len(sessions))

	for i := range sessions {
		session := &sessions[i]

		// Skip podcast episodes
		if !session.IsBook() {
			continue
		}

		event := c.SessionToEvent(session, listenUpUserID, listenUpBookID)
		events = append(events, event)
	}

	// Sort by start time to ensure correct replay order
	sortEventsByStartTime(events)

	return events
}

// sortEventsByStartTime sorts events by StartedAt ascending.
// This ensures proper chronological replay when rebuilding progress.
func sortEventsByStartTime(events []*domain.ListeningEvent) {
	// Simple insertion sort - sessions are typically already sorted
	for i := 1; i < len(events); i++ {
		j := i
		for j > 0 && events[j].StartedAt.Before(events[j-1].StartedAt) {
			events[j], events[j-1] = events[j-1], events[j]
			j--
		}
	}
}

// SessionStats calculates summary statistics for converted sessions.
type SessionStats struct {
	TotalSessions     int
	TotalDurationMs   int64
	EarliestSession   time.Time
	LatestSession     time.Time
	UniqueBooks       int
	UniqueUsers       int
}

// CalculateSessionStats computes statistics for a batch of sessions.
func CalculateSessionStats(sessions []Session) SessionStats {
	if len(sessions) == 0 {
		return SessionStats{}
	}

	stats := SessionStats{
		TotalSessions:   len(sessions),
		EarliestSession: time.Now(),
	}

	books := make(map[string]bool)
	users := make(map[string]bool)

	for i := range sessions {
		s := &sessions[i]
		if !s.IsBook() {
			continue
		}

		stats.TotalDurationMs += s.DurationMs()
		books[s.LibraryItemID] = true
		users[s.UserID] = true

		startedAt := s.StartedAtTime()
		if startedAt.Before(stats.EarliestSession) {
			stats.EarliestSession = startedAt
		}
		if startedAt.After(stats.LatestSession) {
			stats.LatestSession = startedAt
		}
	}

	stats.UniqueBooks = len(books)
	stats.UniqueUsers = len(users)

	return stats
}
