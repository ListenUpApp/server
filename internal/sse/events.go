// Package sse implements Server-Sent Events for real-time library updates and event broadcasting.
package sse

import (
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
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

	// EventTranscodeProgress represents a transcode job progress update.
	EventTranscodeProgress EventType = "transcode.progress"
	// EventTranscodeComplete represents a transcode job completion.
	EventTranscodeComplete EventType = "transcode.complete"
	// EventTranscodeFailed represents a transcode job failure.
	EventTranscodeFailed EventType = "transcode.failed"

	// EventUserPending represents a new user registration awaiting approval.
	// Only sent to admin users.
	EventUserPending EventType = "user.pending"
	// EventUserApproved represents a pending user being approved.
	// Only sent to admin users.
	EventUserApproved EventType = "user.approved"

	// Collection events (admin-only)
	EventCollectionCreated     EventType = "collection.created"
	EventCollectionUpdated     EventType = "collection.updated"
	EventCollectionDeleted     EventType = "collection.deleted"
	EventCollectionBookAdded   EventType = "collection.book_added"
	EventCollectionBookRemoved EventType = "collection.book_removed"
)

// Event represents an SSE event to be sent to clients.
// The Data field contains the event payload as a JSON object for direct deserialization.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"` // Event-specific data as JSON object
	Type      EventType `json:"type"`

	// Filtering fields for multi-user support.
	// When set, events are only delivered to clients matching these criteria.
	// Empty string means "broadcast to all" (backwards compatible).
	UserID       string `json:"-"` // Filter to specific user (not sent to client)
	CollectionID string `json:"-"` // Filter to specific collection (not sent to client)
}

// BookEventData is the data payload for book events.
// Contains enriched book DTO with denormalized fields for immediate rendering.
// This ensures SSE events are self-contained and immediately renderable without
// additional database queries or waiting for related entity events.
type BookEventData struct {
	Book *dto.Book `json:"book"`
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

// TranscodeProgressEventData is the data payload for transcode progress events.
type TranscodeProgressEventData struct {
	JobID       string `json:"job_id"`
	BookID      string `json:"book_id"`
	AudioFileID string `json:"audio_file_id"`
	Progress    int    `json:"progress"`
}

// TranscodeCompleteEventData is the data payload for transcode complete events.
type TranscodeCompleteEventData struct {
	JobID       string `json:"job_id"`
	BookID      string `json:"book_id"`
	AudioFileID string `json:"audio_file_id"`
}

// TranscodeFailedEventData is the data payload for transcode failure events.
type TranscodeFailedEventData struct {
	JobID       string `json:"job_id"`
	BookID      string `json:"book_id"`
	AudioFileID string `json:"audio_file_id"`
	Error       string `json:"error"`
}

// UserPendingEventData is the data payload for user pending events.
type UserPendingEventData struct {
	User *domain.User `json:"user"`
}

// UserApprovedEventData is the data payload for user approved events.
type UserApprovedEventData struct {
	User *domain.User `json:"user"`
}

// CollectionEventData is the data payload for collection CRUD events.
type CollectionEventData struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	BookCount int       `json:"book_count"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// CollectionBookEventData is the data payload for collection book add/remove events.
type CollectionBookEventData struct {
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name"`
	BookID         string `json:"book_id"`
}

// NewBookCreatedEvent creates a book.created event.
// Expects an enriched dto.Book with denormalized display fields populated.
func NewBookCreatedEvent(book *dto.Book) Event {
	return Event{
		Type:      EventBookCreated,
		Data:      BookEventData{Book: book},
		Timestamp: time.Now(),
	}
}

// NewBookUpdatedEvent creates a book.updated event.
// Expects an enriched dto.Book with denormalized display fields populated.
func NewBookUpdatedEvent(book *dto.Book) Event {
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

// NewTranscodeProgressEvent creates a transcode.progress event.
func NewTranscodeProgressEvent(jobID, bookID, audioFileID string, progress int) Event {
	return Event{
		Type: EventTranscodeProgress,
		Data: TranscodeProgressEventData{
			JobID:       jobID,
			BookID:      bookID,
			AudioFileID: audioFileID,
			Progress:    progress,
		},
		Timestamp: time.Now(),
	}
}

// NewTranscodeCompleteEvent creates a transcode.complete event.
func NewTranscodeCompleteEvent(jobID, bookID, audioFileID string) Event {
	return Event{
		Type: EventTranscodeComplete,
		Data: TranscodeCompleteEventData{
			JobID:       jobID,
			BookID:      bookID,
			AudioFileID: audioFileID,
		},
		Timestamp: time.Now(),
	}
}

// NewTranscodeFailedEvent creates a transcode.failed event.
func NewTranscodeFailedEvent(jobID, bookID, audioFileID, errMsg string) Event {
	return Event{
		Type: EventTranscodeFailed,
		Data: TranscodeFailedEventData{
			JobID:       jobID,
			BookID:      bookID,
			AudioFileID: audioFileID,
			Error:       errMsg,
		},
		Timestamp: time.Now(),
	}
}

// NewUserPendingEvent creates a user.pending event for admin users.
func NewUserPendingEvent(user *domain.User) Event {
	return Event{
		Type:      EventUserPending,
		Data:      UserPendingEventData{User: user},
		Timestamp: time.Now(),
	}
}

// NewUserApprovedEvent creates a user.approved event for admin users.
func NewUserApprovedEvent(user *domain.User) Event {
	return Event{
		Type:      EventUserApproved,
		Data:      UserApprovedEventData{User: user},
		Timestamp: time.Now(),
	}
}

// NewCollectionCreatedEvent creates a collection.created event.
func NewCollectionCreatedEvent(id, name string, bookCount int) Event {
	return Event{
		Type: EventCollectionCreated,
		Data: CollectionEventData{
			ID:        id,
			Name:      name,
			BookCount: bookCount,
			CreatedAt: time.Now(),
		},
		Timestamp: time.Now(),
	}
}

// NewCollectionUpdatedEvent creates a collection.updated event.
func NewCollectionUpdatedEvent(id, name string, bookCount int) Event {
	return Event{
		Type: EventCollectionUpdated,
		Data: CollectionEventData{
			ID:        id,
			Name:      name,
			BookCount: bookCount,
			UpdatedAt: time.Now(),
		},
		Timestamp: time.Now(),
	}
}

// NewCollectionDeletedEvent creates a collection.deleted event.
func NewCollectionDeletedEvent(id, name string) Event {
	return Event{
		Type: EventCollectionDeleted,
		Data: CollectionEventData{
			ID:   id,
			Name: name,
		},
		Timestamp: time.Now(),
	}
}

// NewCollectionBookAddedEvent creates a collection.book_added event.
func NewCollectionBookAddedEvent(collectionID, collectionName, bookID string) Event {
	return Event{
		Type: EventCollectionBookAdded,
		Data: CollectionBookEventData{
			CollectionID:   collectionID,
			CollectionName: collectionName,
			BookID:         bookID,
		},
		Timestamp: time.Now(),
	}
}

// NewCollectionBookRemovedEvent creates a collection.book_removed event.
func NewCollectionBookRemovedEvent(collectionID, collectionName, bookID string) Event {
	return Event{
		Type: EventCollectionBookRemoved,
		Data: CollectionBookEventData{
			CollectionID:   collectionID,
			CollectionName: collectionName,
			BookID:         bookID,
		},
		Timestamp: time.Now(),
	}
}
