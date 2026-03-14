package sse

import (
	"context"
	"time"
)

// EventLogEntry represents a persisted SSE event for replay.
type EventLogEntry struct {
	ID        int64
	EventType string
	Payload   string
	UserID    string
	CreatedAt time.Time
}

// EventLogger persists SSE events for replay on client reconnect.
type EventLogger interface {
	// LogEvent writes an event to the persistent log.
	// Returns the auto-incremented event ID.
	LogEvent(ctx context.Context, eventType, payload, userID string) (int64, error)

	// ReplayEvents returns events newer than 'since' visible to userID.
	// Broadcast events (user_id IS NULL) are always included.
	ReplayEvents(ctx context.Context, since time.Time, userID string) ([]EventLogEntry, error)

	// ReplayEventsSinceID returns events with id > lastEventID visible to userID.
	ReplayEventsSinceID(ctx context.Context, lastEventID int64, userID string) ([]EventLogEntry, error)

	// CleanupEventLog deletes events older than maxAge.
	CleanupEventLog(ctx context.Context, maxAge time.Duration) (int64, error)
}
