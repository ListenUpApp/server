package domain

import "time"

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

	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// PlaybackProgress is a materialized view over ListeningEvents.
// Contains ONLY event-derived data. Fully rebuildable from events.
type PlaybackProgress struct {
	UserID            string     `json:"user_id"`
	BookID            string     `json:"book_id"`
	CurrentPositionMs int64      `json:"current_position_ms"`
	Progress          float64    `json:"progress"` // 0.0 - 1.0
	IsFinished        bool       `json:"is_finished"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	StartedAt         time.Time  `json:"started_at"`
	LastPlayedAt      time.Time  `json:"last_played_at"`
	TotalListenTimeMs int64      `json:"total_listen_time_ms"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ProgressID generates composite key: "userID:bookID".
func ProgressID(userID, bookID string) string {
	return userID + ":" + bookID
}

// NewPlaybackProgress creates progress from the first listening event.
func NewPlaybackProgress(event *ListeningEvent, bookDurationMs int64) *PlaybackProgress {
	progress := &PlaybackProgress{
		UserID:            event.UserID,
		BookID:            event.BookID,
		CurrentPositionMs: event.EndPositionMs,
		StartedAt:         event.StartedAt,
		LastPlayedAt:      event.EndedAt,
		TotalListenTimeMs: event.DurationMs,
		UpdatedAt:         time.Now(),
	}

	// Calculate progress percentage
	if bookDurationMs > 0 {
		progress.Progress = float64(progress.CurrentPositionMs) / float64(bookDurationMs)
	}

	// Check completion (99% threshold)
	progress.checkCompletion(bookDurationMs)

	return progress
}

// UpdateFromEvent updates progress with a new listening event.
// Position only advances forward (rewinds don't move position back).
// Total listen time always accumulates.
func (p *PlaybackProgress) UpdateFromEvent(event *ListeningEvent, bookDurationMs int64) {
	// Always accumulate total listen time
	p.TotalListenTimeMs += event.DurationMs

	// Only advance position forward (rewinds don't reset progress)
	if event.EndPositionMs > p.CurrentPositionMs {
		p.CurrentPositionMs = event.EndPositionMs
	}

	// Update last played time
	p.LastPlayedAt = event.EndedAt

	// Recalculate progress percentage
	if bookDurationMs > 0 {
		p.Progress = float64(p.CurrentPositionMs) / float64(bookDurationMs)
	}

	// Check completion
	p.checkCompletion(bookDurationMs)

	p.UpdatedAt = time.Now()
}

// checkCompletion marks the book as finished if position >= 99% of duration.
func (p *PlaybackProgress) checkCompletion(bookDurationMs int64) {
	if bookDurationMs <= 0 {
		return
	}

	threshold := float64(bookDurationMs) * 0.99
	if float64(p.CurrentPositionMs) >= threshold {
		p.IsFinished = true
		now := time.Now()
		p.FinishedAt = &now
	}
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
	TotalDurationMs int64   `json:"total_duration_ms"`
}
