// Package sse implements Server-Sent Events for real-time library updates and event broadcasting.
package sse

import (
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// In ListenUp we primarily use SSE for server-to-client communication.
// since most interactions follow a request/response pattern.
// Full bidirectional communication (like listening sessions) may be.
// implemented with WebSockets in the future if needed.

// EventType represents the type of SSE Event.
type EventType string

const (
	// EventBookCreated represents a book creation event.
	EventBookCreated EventType = "book.created"
	// EventBookUpdated represents a book update event.
	EventBookUpdated EventType = "book.updated"
	// EventBookDeleted represents a book deletion event.
	EventBookDeleted EventType = "book.deleted"

	// EventScanStarted represents a library scan start event.
	EventScanStarted EventType = "library.scan_started"
	// EventScanComplete represents a library scan completion event.
	EventScanComplete EventType = "library.scan_completed"

	// TODO: See if we actually need progress updates.  Right now the scanner
	// completes the scan in milliseconds on my computer so progress updates is.
	// overkill.  However we should re-evaluate once we've tested more in other settings.

	// EventHeartbeat represents a connection keepalive event.
	EventHeartbeat EventType = "heartbeat"

	// EventContributorCreated represents a contributor creation event.
	EventContributorCreated EventType = "contributor.created"
	// EventContributorUpdated represents a contributor update event.
	EventContributorUpdated EventType = "contributor.updated"
	// EventContributorDeleted represents a contributor deletion event.
	EventContributorDeleted EventType = "contributor.deleted"

	// EventSeriesCreated represents a series creation event.
	EventSeriesCreated EventType = "series.created"
	// EventSeriesUpdated represents a series update event.
	EventSeriesUpdated EventType = "series.updated"
	// EventSeriesDeleted represents a series deletion event.
	EventSeriesDeleted EventType = "series.deleted"
)

// Event represents an SSE event to be sent to clients.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
	Type      EventType `json:"type"`

	// Future fields for filtering (TODO: Add when implementing multi-user)
	// SECURITY TODO: Currently broadcasting to all clients regardless of user/collection
	// Issue: Any connected client receives all library events (John's reading habits are at risk of disclosure.)
	// Risk: Medium - self-hosted single-user deployments are fine, multi-user is not
	// Fix: When implementing auth at some point, filter events by UserID and Collection
}

// BookEventData is the data payload for book events.
type BookEventData struct {
	Book *domain.Book `json:"book"`
}

// BookDeletedEventData is the data payload for book delete events.
type BookDeletedEventData struct {
	DeletedAt time.Time `json:"deleted_at"`
	BookID    string    `json:"book_id"`
}

// ScanStartedEventData is the data payload for scan start events.
type ScanStartedEventData struct {
	StartedAt time.Time `json:"started_at"`
	LibraryID string    `json:"library_id"`
}

// ScanCompleteEventData is the data payload for scan complete events.
type ScanCompleteEventData struct {
	CompletedAt  time.Time `json:"completed_at"`
	LibraryID    string    `json:"library_id"`
	BooksAdded   int       `json:"books_added"`
	BooksUpdated int       `json:"books_updated"`
	BooksRemoved int       `json:"books_removed"`
}

// HeartbeatEventData is the data payload for heartbeat events.
type HeartbeatEventData struct {
	ServerTime time.Time `json:"server_time"`
}

// ContributorEventData is the data payload for contributor events.
type ContributorEventData struct {
	Contributor *domain.Contributor `json:"contributor"`
}

// SeriesEventData is the data payload for series events.
type SeriesEventData struct {
	Series *domain.Series `json:"series"`
}

// NewBookCreatedEvent creates a book.created event.
func NewBookCreatedEvent(book *domain.Book) Event {
	return Event{
		Type:      EventBookCreated,
		Data:      BookEventData{Book: book},
		Timestamp: time.Now(),
	}
}

// NewBookUpdatedEvent creates a book.updated event.
func NewBookUpdatedEvent(book *domain.Book) Event {
	return Event{
		Type:      EventBookUpdated,
		Data:      BookEventData{Book: book},
		Timestamp: time.Now(),
	}
}

// NewBookDeletedEvent creates a book.deleted event.
func NewBookDeletedEvent(bookID string, deletedAt time.Time) Event {
	return Event{
		Type: EventBookDeleted,
		Data: BookDeletedEventData{
			BookID:    bookID,
			DeletedAt: deletedAt,
		},
		Timestamp: time.Now(),
	}
}

// NewScanStartedEvent creates a library.scan_started event.
func NewScanStartedEvent(libraryID string) Event {
	return Event{
		Type: EventScanStarted,
		Data: ScanStartedEventData{
			LibraryID: libraryID,
			StartedAt: time.Now(),
		},
		Timestamp: time.Now(),
	}
}

// NewScanCompleteEvent creates a library.scan_completed event.
func NewScanCompleteEvent(libraryID string, added, updated, removed int) Event {
	return Event{
		Type: EventScanComplete,
		Data: ScanCompleteEventData{
			LibraryID:    libraryID,
			CompletedAt:  time.Now(),
			BooksAdded:   added,
			BooksUpdated: updated,
			BooksRemoved: removed,
		},
		Timestamp: time.Now(),
	}
}

// NewHeartbeatEvent creates a heartbeat event.
func NewHeartbeatEvent() Event {
	return Event{
		Type: EventHeartbeat,
		Data: HeartbeatEventData{
			ServerTime: time.Now(),
		},
		Timestamp: time.Now(),
	}
}

// NewContributorCreatedEvent creates a contributor.created event.
func NewContributorCreatedEvent(contributor *domain.Contributor) Event {
	return Event{
		Type:      EventContributorCreated,
		Data:      ContributorEventData{Contributor: contributor},
		Timestamp: time.Now(),
	}
}

// NewContributorUpdatedEvent creates a contributor.updated event.
func NewContributorUpdatedEvent(contributor *domain.Contributor) Event {
	return Event{
		Type:      EventContributorUpdated,
		Data:      ContributorEventData{Contributor: contributor},
		Timestamp: time.Now(),
	}
}

// NewSeriesCreatedEvent creates a series.created event.
func NewSeriesCreatedEvent(series *domain.Series) Event {
	return Event{
		Type:      EventSeriesCreated,
		Data:      SeriesEventData{Series: series},
		Timestamp: time.Now(),
	}
}

// NewSeriesUpdatedEvent creates a series.updated event.
func NewSeriesUpdatedEvent(series *domain.Series) Event {
	return Event{
		Type:      EventSeriesUpdated,
		Data:      SeriesEventData{Series: series},
		Timestamp: time.Now(),
	}
}
