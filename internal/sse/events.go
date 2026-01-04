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
	// EventUserDeleted represents a user being deleted.
	// Sent to the deleted user so they can clear auth and show appropriate message.
	EventUserDeleted EventType = "user.deleted"

	// Collection events (admin-only)
	EventCollectionCreated     EventType = "collection.created"
	EventCollectionUpdated     EventType = "collection.updated"
	EventCollectionDeleted     EventType = "collection.deleted"
	EventCollectionBookAdded   EventType = "collection.book_added"
	EventCollectionBookRemoved EventType = "collection.book_removed"

	// Lens events (broadcast to all)
	EventLensCreated     EventType = "lens.created"
	EventLensUpdated     EventType = "lens.updated"
	EventLensDeleted     EventType = "lens.deleted"
	EventLensBookAdded   EventType = "lens.book_added"
	EventLensBookRemoved EventType = "lens.book_removed"

	// Tag events (broadcast to all)
	EventTagCreated     EventType = "tag.created"
	EventBookTagAdded   EventType = "book.tag_added"
	EventBookTagRemoved EventType = "book.tag_removed"

	// Inbox events (admin-only)
	EventInboxBookAdded    EventType = "inbox.book_added"
	EventInboxBookReleased EventType = "inbox.book_released"

	// Listening events (user-specific)
	EventProgressUpdated       EventType = "listening.progress_updated"
	EventListeningEventCreated EventType = "listening.event_created"
	EventReadingSessionUpdated EventType = "reading_session.updated"

	// Active session events (broadcast to all for "Currently Listening" feature)
	EventSessionStarted EventType = "session.started"
	EventSessionEnded   EventType = "session.ended"

	// Activity feed events (broadcast to all)
	EventActivityCreated EventType = "activity.created"

	// Profile events (broadcast to all)
	EventProfileUpdated EventType = "profile.updated"

	// User stats events (broadcast to all for leaderboard caching)
	EventUserStatsUpdated EventType = "user_stats.updated"
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

// UserDeletedEventData is the data payload for user deleted events.
// Sent to the specific user being deleted so they can clear auth.
type UserDeletedEventData struct {
	UserID string `json:"user_id"`
	Reason string `json:"reason,omitempty"`
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

// LensEventData is the data payload for lens CRUD events.
type LensEventData struct {
	ID               string    `json:"id"`
	OwnerID          string    `json:"owner_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	BookCount        int       `json:"book_count"`
	OwnerDisplayName string    `json:"owner_display_name"`
	OwnerAvatarColor string    `json:"owner_avatar_color"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
}

// LensDeletedEventData is the data payload for lens delete events.
type LensDeletedEventData struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"owner_id"`
	DeletedAt time.Time `json:"deleted_at"`
}

// LensBookEventData is the data payload for lens book add/remove events.
type LensBookEventData struct {
	LensID    string    `json:"lens_id"`
	OwnerID   string    `json:"owner_id"`
	BookID    string    `json:"book_id"`
	BookCount int       `json:"book_count"`
	Timestamp time.Time `json:"timestamp"`
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

// NewUserDeletedEvent creates a user.deleted event for a specific user.
// This event is targeted to the deleted user so they can clear auth.
func NewUserDeletedEvent(userID, reason string) Event {
	return Event{
		Type:      EventUserDeleted,
		UserID:    userID, // Target this event to the specific user
		Data:      UserDeletedEventData{UserID: userID, Reason: reason},
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

// NewLensCreatedEvent creates a lens.created event.
func NewLensCreatedEvent(lens *domain.Lens, ownerDisplayName, ownerAvatarColor string) Event {
	return Event{
		Type: EventLensCreated,
		Data: LensEventData{
			ID:               lens.ID,
			OwnerID:          lens.OwnerID,
			Name:             lens.Name,
			Description:      lens.Description,
			BookCount:        len(lens.BookIDs),
			OwnerDisplayName: ownerDisplayName,
			OwnerAvatarColor: ownerAvatarColor,
			CreatedAt:        lens.CreatedAt,
		},
		Timestamp: time.Now(),
	}
}

// NewLensUpdatedEvent creates a lens.updated event.
func NewLensUpdatedEvent(lens *domain.Lens, ownerDisplayName, ownerAvatarColor string) Event {
	return Event{
		Type: EventLensUpdated,
		Data: LensEventData{
			ID:               lens.ID,
			OwnerID:          lens.OwnerID,
			Name:             lens.Name,
			Description:      lens.Description,
			BookCount:        len(lens.BookIDs),
			OwnerDisplayName: ownerDisplayName,
			OwnerAvatarColor: ownerAvatarColor,
			UpdatedAt:        lens.UpdatedAt,
		},
		Timestamp: time.Now(),
	}
}

// NewLensDeletedEvent creates a lens.deleted event.
func NewLensDeletedEvent(lensID, ownerID string) Event {
	return Event{
		Type: EventLensDeleted,
		Data: LensDeletedEventData{
			ID:        lensID,
			OwnerID:   ownerID,
			DeletedAt: time.Now(),
		},
		Timestamp: time.Now(),
	}
}

// NewLensBookAddedEvent creates a lens.book_added event.
func NewLensBookAddedEvent(lens *domain.Lens, bookID string) Event {
	return Event{
		Type: EventLensBookAdded,
		Data: LensBookEventData{
			LensID:    lens.ID,
			OwnerID:   lens.OwnerID,
			BookID:    bookID,
			BookCount: len(lens.BookIDs),
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}
}

// NewLensBookRemovedEvent creates a lens.book_removed event.
func NewLensBookRemovedEvent(lens *domain.Lens, bookID string) Event {
	return Event{
		Type: EventLensBookRemoved,
		Data: LensBookEventData{
			LensID:    lens.ID,
			OwnerID:   lens.OwnerID,
			BookID:    bookID,
			BookCount: len(lens.BookIDs),
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}
}

// TagEventData is the data payload for tag CRUD events.
type TagEventData struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	BookCount int       `json:"book_count"`
	CreatedAt time.Time `json:"created_at"`
}

// BookTagEventData is the data payload for book tag add/remove events.
type BookTagEventData struct {
	BookID string       `json:"book_id"`
	Tag    TagEventData `json:"tag"`
}

// NewTagCreatedEvent creates a tag.created event.
func NewTagCreatedEvent(tag *domain.Tag) Event {
	return Event{
		Type: EventTagCreated,
		Data: TagEventData{
			ID:        tag.ID,
			Slug:      tag.Slug,
			BookCount: tag.BookCount,
			CreatedAt: tag.CreatedAt,
		},
		Timestamp: time.Now(),
	}
}

// NewBookTagAddedEvent creates a book.tag_added event.
func NewBookTagAddedEvent(bookID string, tag *domain.Tag) Event {
	return Event{
		Type: EventBookTagAdded,
		Data: BookTagEventData{
			BookID: bookID,
			Tag: TagEventData{
				ID:        tag.ID,
				Slug:      tag.Slug,
				BookCount: tag.BookCount,
				CreatedAt: tag.CreatedAt,
			},
		},
		Timestamp: time.Now(),
	}
}

// NewBookTagRemovedEvent creates a book.tag_removed event.
func NewBookTagRemovedEvent(bookID string, tag *domain.Tag) Event {
	return Event{
		Type: EventBookTagRemoved,
		Data: BookTagEventData{
			BookID: bookID,
			Tag: TagEventData{
				ID:        tag.ID,
				Slug:      tag.Slug,
				BookCount: tag.BookCount,
				CreatedAt: tag.CreatedAt,
			},
		},
		Timestamp: time.Now(),
	}
}

// InboxBookAddedEventData is the data payload for inbox book added events.
type InboxBookAddedEventData struct {
	Book *dto.Book `json:"book"`
}

// InboxBookReleasedEventData is the data payload for inbox book released events.
type InboxBookReleasedEventData struct {
	BookID string `json:"book_id"`
}

// NewInboxBookAddedEvent creates an inbox.book_added event.
func NewInboxBookAddedEvent(book *dto.Book) Event {
	return Event{
		Type:      EventInboxBookAdded,
		Data:      InboxBookAddedEventData{Book: book},
		Timestamp: time.Now(),
	}
}

// NewInboxBookReleasedEvent creates an inbox.book_released event.
func NewInboxBookReleasedEvent(bookID string) Event {
	return Event{
		Type:      EventInboxBookReleased,
		Data:      InboxBookReleasedEventData{BookID: bookID},
		Timestamp: time.Now(),
	}
}

// ProgressUpdatedEventData is the data payload for listening.progress_updated events.
type ProgressUpdatedEventData struct {
	BookID            string    `json:"book_id"`
	CurrentPositionMs int64     `json:"current_position_ms"`
	Progress          float64   `json:"progress"`
	TotalListenTimeMs int64     `json:"total_listen_time_ms"`
	IsFinished        bool      `json:"is_finished"`
	LastPlayedAt      time.Time `json:"last_played_at"`
}

// NewProgressUpdatedEvent creates a listening.progress_updated event for a specific user.
func NewProgressUpdatedEvent(userID string, progress *domain.PlaybackProgress) Event {
	return Event{
		Type: EventProgressUpdated,
		Data: ProgressUpdatedEventData{
			BookID:            progress.BookID,
			CurrentPositionMs: progress.CurrentPositionMs,
			Progress:          progress.Progress,
			TotalListenTimeMs: progress.TotalListenTimeMs,
			IsFinished:        progress.IsFinished,
			LastPlayedAt:      progress.LastPlayedAt,
		},
		UserID:    userID, // Only send to this user
		Timestamp: time.Now(),
	}
}

// ListeningEventCreatedEventData is the data payload for listening.event_created events.
// Sent to other devices when a listening event is recorded, enabling offline-first stats.
type ListeningEventCreatedEventData struct {
	ID              string    `json:"id"`
	BookID          string    `json:"book_id"`
	StartPositionMs int64     `json:"start_position_ms"`
	EndPositionMs   int64     `json:"end_position_ms"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	PlaybackSpeed   float32   `json:"playback_speed"`
	DeviceID        string    `json:"device_id"`
	CreatedAt       time.Time `json:"created_at"`
}

// NewListeningEventCreatedEvent creates a listening.event_created event for a specific user.
// Used to sync listening events to other devices for offline stats computation.
func NewListeningEventCreatedEvent(userID string, event *domain.ListeningEvent) Event {
	return Event{
		Type: EventListeningEventCreated,
		Data: ListeningEventCreatedEventData{
			ID:              event.ID,
			BookID:          event.BookID,
			StartPositionMs: event.StartPositionMs,
			EndPositionMs:   event.EndPositionMs,
			StartedAt:       event.StartedAt,
			EndedAt:         event.EndedAt,
			PlaybackSpeed:   event.PlaybackSpeed,
			DeviceID:        event.DeviceID,
			CreatedAt:       event.CreatedAt,
		},
		UserID:    userID, // Only send to this user's other devices
		Timestamp: time.Now(),
	}
}

// ReadingSessionUpdatedEventData is the data payload for reading_session.updated events.
type ReadingSessionUpdatedEventData struct {
	SessionID    string     `json:"session_id"`
	BookID       string     `json:"book_id"`
	IsCompleted  bool       `json:"is_completed"`
	ListenTimeMs int64      `json:"listen_time_ms"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// NewReadingSessionUpdatedEvent creates a reading_session.updated event.
// Broadcast to all users so book readers lists can update.
func NewReadingSessionUpdatedEvent(session *domain.BookReadingSession) Event {
	return Event{
		Type: EventReadingSessionUpdated,
		Data: ReadingSessionUpdatedEventData{
			SessionID:    session.ID,
			BookID:       session.BookID,
			IsCompleted:  session.IsCompleted,
			ListenTimeMs: session.ListenTimeMs,
			FinishedAt:   session.FinishedAt,
		},
		Timestamp: time.Now(),
	}
}

// ActivityEventData is the data payload for activity.created events.
// Contains activity fields directly for immediate rendering in the feed.
// Fields are at top level (not wrapped) for consistency with client expectations.
type ActivityEventData struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Type            string    `json:"type"`
	CreatedAt       time.Time `json:"created_at"`
	UserDisplayName string    `json:"user_display_name"`
	UserAvatarColor string    `json:"user_avatar_color"`
	UserAvatarType  string    `json:"user_avatar_type"`
	UserAvatarValue string    `json:"user_avatar_value,omitempty"`
	BookID          string    `json:"book_id,omitempty"`
	BookTitle       string    `json:"book_title,omitempty"`
	BookAuthorName  string    `json:"book_author_name,omitempty"`
	BookCoverPath   string    `json:"book_cover_path,omitempty"`
	IsReread        bool      `json:"is_reread,omitempty"`
	DurationMs      int64     `json:"duration_ms,omitempty"` // For listening_session activities
	MilestoneValue  int       `json:"milestone_value,omitempty"`
	MilestoneUnit   string    `json:"milestone_unit,omitempty"`
	LensID          string    `json:"lens_id,omitempty"`
	LensName        string    `json:"lens_name,omitempty"`
}

// NewActivityEvent creates an activity.created event.
// Broadcast to all users for real-time activity feed updates.
func NewActivityEvent(activity *domain.Activity) Event {
	return Event{
		Type: EventActivityCreated,
		Data: ActivityEventData{
			ID:              activity.ID,
			UserID:          activity.UserID,
			Type:            string(activity.Type),
			CreatedAt:       activity.CreatedAt,
			UserDisplayName: activity.UserDisplayName,
			UserAvatarColor: activity.UserAvatarColor,
			UserAvatarType:  activity.UserAvatarType,
			UserAvatarValue: activity.UserAvatarValue,
			BookID:          activity.BookID,
			BookTitle:       activity.BookTitle,
			BookAuthorName:  activity.BookAuthorName,
			BookCoverPath:   activity.BookCoverPath,
			IsReread:        activity.IsReread,
			DurationMs:      activity.DurationMs,
			MilestoneValue:  activity.MilestoneValue,
			MilestoneUnit:   activity.MilestoneUnit,
			LensID:          activity.LensID,
			LensName:        activity.LensName,
		},
		Timestamp: time.Now(),
	}
}

// ProfileUpdatedEventData is the payload for profile update events.
type ProfileUpdatedEventData struct {
	UserID      string `json:"user_id"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	AvatarType  string `json:"avatar_type"`
	AvatarValue string `json:"avatar_value,omitempty"`
	AvatarColor string `json:"avatar_color"`
	Tagline     string `json:"tagline,omitempty"`
}

// NewProfileUpdatedEvent creates a profile.updated event.
// Broadcast to all users so avatars can update across the app.
func NewProfileUpdatedEvent(data ProfileUpdatedEventData) Event {
	return Event{
		Type:      EventProfileUpdated,
		Data:      data,
		Timestamp: time.Now(),
	}
}

// SessionStartedEventData is the payload for session.started events.
// Broadcast to all users for "What Others Are Listening To" feature.
type SessionStartedEventData struct {
	SessionID string    `json:"session_id"`
	UserID    string    `json:"user_id"`
	BookID    string    `json:"book_id"`
	StartedAt time.Time `json:"started_at"`
}

// SessionEndedEventData is the payload for session.ended events.
type SessionEndedEventData struct {
	SessionID string `json:"session_id"`
}

// NewSessionStartedEvent creates a session.started event.
// Broadcast to all users so they can see who's currently listening.
func NewSessionStartedEvent(sessionID, userID, bookID string, startedAt time.Time) Event {
	return Event{
		Type: EventSessionStarted,
		Data: SessionStartedEventData{
			SessionID: sessionID,
			UserID:    userID,
			BookID:    bookID,
			StartedAt: startedAt,
		},
		Timestamp: time.Now(),
	}
}

// NewSessionEndedEvent creates a session.ended event.
// Broadcast to all users to remove the session from "Currently Listening".
func NewSessionEndedEvent(sessionID string) Event {
	return Event{
		Type: EventSessionEnded,
		Data: SessionEndedEventData{
			SessionID: sessionID,
		},
		Timestamp: time.Now(),
	}
}

// UserStatsUpdatedEventData is the payload for user_stats.updated events.
// Contains updated all-time stats for a user, enabling clients to cache
// without re-fetching the entire leaderboard.
type UserStatsUpdatedEventData struct {
	UserID        string `json:"user_id"`
	DisplayName   string `json:"display_name"`
	AvatarType    string `json:"avatar_type"`
	AvatarValue   string `json:"avatar_value,omitempty"`
	AvatarColor   string `json:"avatar_color"`
	TotalTimeMs   int64  `json:"total_time_ms"`
	TotalBooks    int    `json:"total_books"`
	CurrentStreak int    `json:"current_streak"`
}

// NewUserStatsUpdatedEvent creates a user_stats.updated event.
// Broadcast to all users so they can update their leaderboard cache.
func NewUserStatsUpdatedEvent(data UserStatsUpdatedEventData) Event {
	return Event{
		Type:      EventUserStatsUpdated,
		Data:      data,
		Timestamp: time.Now(),
	}
}
