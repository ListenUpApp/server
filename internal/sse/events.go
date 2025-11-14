package sse

import (
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// In ListenUp we primarily use SSE for server-to-client communication
// since most interactions follow a request/response pattern.
// Full bidirectional communication (like listening sessions) may be
// implemented with WebSockets in the future if needed.

// EventType represents the type of SSE Event
type EventType string

const (
	// Book events
	EventBookCreated EventType = "book.created"
	EventBookUpdated EventType = "book.updated"
	EventBookDeleted EventType = "book.deleted"

	// Library Scan Events
	EventScanStarted  EventType = "library.scan_started"
	EventScanComplete EventType = "library.scan_completed"

	// TODO: See if we actually need progress updates.  Right now the scanner
	// completes the scan in milliseconds on my computer so progress updates is
	// overkill.  However we should re-evaluate once we've tested more in other settings.

	// Heartbeat for connection keepalive
	EventHeartbeat EventType = "heartbeat"
)

// Event represents an SSE event to be sent to clients.
type Event struct {
	Type      EventType `json:"type"`
	Data      any       `json:"data"`
	Timestamp time.Time `json:"timestamp"`

	// Future fields for filtering (TODO: Add when implementing multi-user)
	// SECURITY TODO: Currently broadcasting to all clients regardless of user/collection
	// Issue: Any connected client receives all library events (John's reading habits are at risk of disclosure.)
	// Risk: Medium - self-hosted single-user deployments are fine, multi-user is not
	// Fix: When implementing auth at some point, filter events by UserID and Collection
	// UserID     string      `json:"user_id,omitempty"`
	// Collection []string    `json:"collections,omitempty"`
}

// BookEventData is the data payload for book events
type BookEventData struct {
	Book *domain.Book `json:"book"`
}

// BookDeletedEventData is the data payload for book delete events
type BookDeletedEventData struct {
	BookID    string    `json:"book_id"`
	DeletedAt time.Time `json:"deleted_at"`
}

// ScanStartedEventData is the data payload for scan start events
type ScanStartedEventData struct {
	LibraryID string    `json:"library_id"`
	StartedAt time.Time `json:"started_at"`
}

// ScanCompleteEventData is the data payload for scan complete events
type ScanCompleteEventData struct {
	LibraryID    string    `json:"library_id"`
	CompletedAt  time.Time `json:"completed_at"`
	BooksAdded   int       `json:"books_added"`
	BooksUpdated int       `json:"books_updated"`
	BooksRemoved int       `json:"books_removed"`
}

// HeartbeatEventData is the data payload for heartbeat events
type HeartbeatEventData struct {
	ServerTime time.Time `json:"server_time"`
}

// NewBookCreatedEvent creates a book.created event
func NewBookCreatedEvent(book *domain.Book) Event {
	return Event{
		Type:      EventBookCreated,
		Data:      BookEventData{Book: book},
		Timestamp: time.Now(),
	}
}

// NewBookUpdatedEvent creates a book.updated event
func NewBookUpdatedEvent(book *domain.Book) Event {
	return Event{
		Type:      EventBookUpdated,
		Data:      BookEventData{Book: book},
		Timestamp: time.Now(),
	}
}

// NewBookDeletedEvent creates a book.deleted event
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

// NewScanStartedEvent creates a library.scan_started event
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

// NewScanCompleteEvent creates a library.scan_completed event
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

// NewHeartbeatEvent creates a heartbeat event
func NewHeartbeatEvent() Event {
	return Event{
		Type: EventHeartbeat,
		Data: HeartbeatEventData{
			ServerTime: time.Now(),
		},
		Timestamp: time.Now(),
	}
}
