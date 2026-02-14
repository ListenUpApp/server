package domain

import "time"

// Event source constants
const (
	EventSourcePlayback = "playback" // Normal listening activity
	EventSourceImport   = "import"   // Imported from external system
	EventSourceManual   = "manual"   // Manual user action
)

// ListeningEvent is the atomic, immutable record of listening activity.
// Events are append-only - everything else derives from them.
type ListeningEvent struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	BookID string `json:"book_id"`

	StartPositionMs int64     `json:"start_position_ms"`
	EndPositionMs   int64     `json:"end_position_ms"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`

	PlaybackSpeed float32 `json:"playback_speed"`
	DeviceID      string  `json:"device_id"`
	DeviceName    string  `json:"device_name,omitempty"`
	Source        string  `json:"source"` // playback, import, or manual

	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// PlaybackState is a materialized view over ListeningEvents.
// Contains ONLY event-derived data. Fully rebuildable from events.
//
// Note: Progress is computed on-demand via ComputeProgress(bookDurationMs).
// TotalListenTimeMs should be computed by summing event durations when accuracy matters,
// but is cached here for performance in non-critical paths.
type PlaybackState struct {
	UserID            string     `json:"user_id"`
	BookID            string     `json:"book_id"`
	CurrentPositionMs int64      `json:"current_position_ms"`
	IsFinished        bool       `json:"is_finished"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	StartedAt         time.Time  `json:"started_at"`
	LastPlayedAt      time.Time  `json:"last_played_at"`
	TotalListenTimeMs int64      `json:"total_listen_time_ms"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// StateID generates composite key: "userID:bookID".
func StateID(userID, bookID string) string {
	return userID + ":" + bookID
}

// NewPlaybackState creates state from the first listening event.
func NewPlaybackState(event *ListeningEvent, bookDurationMs int64) *PlaybackState {
	state := &PlaybackState{
		UserID:            event.UserID,
		BookID:            event.BookID,
		CurrentPositionMs: event.EndPositionMs,
		StartedAt:         event.StartedAt,
		LastPlayedAt:      event.EndedAt,
		TotalListenTimeMs: event.DurationMs,
		UpdatedAt:         time.Now(),
	}

	// Check completion (99% threshold)
	state.checkCompletion(bookDurationMs)

	return state
}

// UpdateFromEvent updates state with a new listening event.
// Position only advances forward (rewinds don't move position back).
// Total listen time always accumulates.
func (s *PlaybackState) UpdateFromEvent(event *ListeningEvent, bookDurationMs int64) {
	// Always accumulate total listen time
	s.TotalListenTimeMs += event.DurationMs

	// Only advance position forward (rewinds don't reset progress)
	if event.EndPositionMs > s.CurrentPositionMs {
		s.CurrentPositionMs = event.EndPositionMs
	}

	// Update last played time only if this event is newer (H5: ordering fix)
	if event.EndedAt.After(s.LastPlayedAt) {
		s.LastPlayedAt = event.EndedAt
	}

	// Check completion
	s.checkCompletion(bookDurationMs)

	s.UpdatedAt = time.Now()
}

// checkCompletion marks the book as finished if position >= 99% of duration.
func (s *PlaybackState) checkCompletion(bookDurationMs int64) {
	if bookDurationMs <= 0 {
		return
	}

	threshold := float64(bookDurationMs) * 0.99
	if float64(s.CurrentPositionMs) >= threshold {
		s.IsFinished = true
		now := time.Now()
		s.FinishedAt = &now
	}
}

// ComputeProgress returns the computed progress as a fraction (0.0 to 1.0).
// This is the authoritative way to calculate progress from position and duration.
// Returns 0.0 if bookDurationMs is 0, and caps at 1.0 for positions beyond duration.
func (s *PlaybackState) ComputeProgress(bookDurationMs int64) float64 {
	if bookDurationMs <= 0 {
		return 0.0
	}
	progress := float64(s.CurrentPositionMs) / float64(bookDurationMs)
	if progress > 1.0 {
		return 1.0
	}
	return progress
}

// WallDurationMs returns the actual elapsed time (wall clock).
// This differs from DurationMs when playback speed != 1.0.
// Example: 30 min of content at 2x speed = 15 min wall time.
func (e *ListeningEvent) WallDurationMs() int64 {
	if e.PlaybackSpeed == 0 {
		return e.DurationMs
	}
	return int64(float64(e.DurationMs) / float64(e.PlaybackSpeed))
}

// NewListeningEvent creates a new event with computed fields.
func NewListeningEvent(
	id, userID, bookID string,
	startPositionMs, endPositionMs int64,
	startedAt, endedAt time.Time,
	playbackSpeed float32,
	deviceID, deviceName string,
) *ListeningEvent {
	return &ListeningEvent{
		ID:              id,
		UserID:          userID,
		BookID:          bookID,
		StartPositionMs: startPositionMs,
		EndPositionMs:   endPositionMs,
		StartedAt:       startedAt,
		EndedAt:         endedAt,
		PlaybackSpeed:   playbackSpeed,
		DeviceID:        deviceID,
		DeviceName:      deviceName,
		Source:          EventSourcePlayback, // Default to playback
		DurationMs:      endPositionMs - startPositionMs,
		CreatedAt:       time.Now(),
	}
}

// ContinueListeningItem is a display-ready item for the Continue Listening section.
// Combines progress with essential book details to eliminate client-side joins.
type ContinueListeningItem struct {
	// Progress fields
	BookID            string    `json:"book_id"`
	CurrentPositionMs int64     `json:"current_position_ms"`
	Progress          float64   `json:"progress"` // 0.0 - 1.0
	LastPlayedAt      time.Time `json:"last_played_at"`

	// Book fields (denormalized for display)
	Title           string  `json:"title"`
	AuthorName      string  `json:"author_name"`
	CoverPath       *string `json:"cover_path,omitempty"`
	CoverBlurHash   *string `json:"cover_blur_hash,omitempty"`
	TotalDurationMs int64   `json:"total_duration_ms"`
}
