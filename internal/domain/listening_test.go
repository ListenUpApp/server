package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewListeningEvent_ComputesDuration(t *testing.T) {
	startedAt := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	endedAt := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)

	event := NewListeningEvent(
		"evt-123",
		"user-456",
		"book-789",
		0,       // startPositionMs
		1800000, // endPositionMs (30 minutes)
		startedAt,
		endedAt,
		1.0, // playbackSpeed
		"device-1",
		"Pixel 8",
	)

	require.NotNil(t, event)
	assert.Equal(t, "evt-123", event.ID)
	assert.Equal(t, "user-456", event.UserID)
	assert.Equal(t, "book-789", event.BookID)
	assert.Equal(t, int64(0), event.StartPositionMs)
	assert.Equal(t, int64(1800000), event.EndPositionMs)
	assert.Equal(t, int64(1800000), event.DurationMs) // Computed: End - Start
	assert.Equal(t, float32(1.0), event.PlaybackSpeed)
	assert.Equal(t, "device-1", event.DeviceID)
	assert.Equal(t, "Pixel 8", event.DeviceName)
	assert.False(t, event.CreatedAt.IsZero())
}

func TestListeningEvent_WallDurationMs(t *testing.T) {
	tests := []struct {
		name          string
		durationMs    int64
		playbackSpeed float32
		wantWallMs    int64
	}{
		{
			name:          "1x speed - wall equals content",
			durationMs:    1800000, // 30 min content
			playbackSpeed: 1.0,
			wantWallMs:    1800000, // 30 min wall
		},
		{
			name:          "2x speed - half wall time",
			durationMs:    1800000, // 30 min content
			playbackSpeed: 2.0,
			wantWallMs:    900000, // 15 min wall
		},
		{
			name:          "0.5x speed - double wall time",
			durationMs:    1800000, // 30 min content
			playbackSpeed: 0.5,
			wantWallMs:    3600000, // 60 min wall
		},
		{
			name:          "1.5x speed",
			durationMs:    1800000,
			playbackSpeed: 1.5,
			wantWallMs:    1200000, // 20 min wall
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &ListeningEvent{
				DurationMs:    tt.durationMs,
				PlaybackSpeed: tt.playbackSpeed,
			}
			assert.Equal(t, tt.wantWallMs, event.WallDurationMs())
		})
	}
}

func TestNewPlaybackProgress_FromFirstEvent(t *testing.T) {
	event := &ListeningEvent{
		UserID:          "user-123",
		BookID:          "book-456",
		StartPositionMs: 0,
		EndPositionMs:   1800000, // 30 min
		StartedAt:       time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		EndedAt:         time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC),
		DurationMs:      1800000,
		PlaybackSpeed:   1.0,
	}

	bookDurationMs := int64(3600000) // 1 hour book

	progress := NewPlaybackProgress(event, bookDurationMs)

	require.NotNil(t, progress)
	assert.Equal(t, "user-123", progress.UserID)
	assert.Equal(t, "book-456", progress.BookID)
	assert.Equal(t, int64(1800000), progress.CurrentPositionMs)
	assert.Equal(t, 0.5, progress.Progress) // 30min / 60min = 50%
	assert.False(t, progress.IsFinished)
	assert.Nil(t, progress.FinishedAt)
	assert.Equal(t, event.StartedAt, progress.StartedAt)
	assert.Equal(t, event.EndedAt, progress.LastPlayedAt)
	assert.Equal(t, int64(1800000), progress.TotalListenTimeMs)
}

func TestProgressID(t *testing.T) {
	id := ProgressID("user-123", "book-456")
	assert.Equal(t, "user-123:book-456", id)
}

func TestPlaybackProgress_UpdateFromEvent(t *testing.T) {
	// Initial progress at 30 min
	progress := &PlaybackProgress{
		UserID:            "user-123",
		BookID:            "book-456",
		CurrentPositionMs: 1800000,
		Progress:          0.5,
		StartedAt:         time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		LastPlayedAt:      time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC),
		TotalListenTimeMs: 1800000,
	}

	// New event: listened from 30min to 45min
	event := &ListeningEvent{
		StartPositionMs: 1800000,
		EndPositionMs:   2700000, // 45 min
		StartedAt:       time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC),
		EndedAt:         time.Date(2025, 1, 1, 14, 15, 0, 0, time.UTC),
		DurationMs:      900000, // 15 min of content
	}

	bookDurationMs := int64(3600000) // 1 hour book

	progress.UpdateFromEvent(event, bookDurationMs)

	assert.Equal(t, int64(2700000), progress.CurrentPositionMs)
	assert.Equal(t, 0.75, progress.Progress) // 45min / 60min
	assert.Equal(t, event.EndedAt, progress.LastPlayedAt)
	assert.Equal(t, int64(2700000), progress.TotalListenTimeMs) // 30 + 15 min
	assert.False(t, progress.IsFinished)
	// StartedAt should NOT change
	assert.Equal(t, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), progress.StartedAt)
}

func TestPlaybackProgress_UpdateFromEvent_Rewind(t *testing.T) {
	// Progress at 45 min
	progress := &PlaybackProgress{
		UserID:            "user-123",
		BookID:            "book-456",
		CurrentPositionMs: 2700000,
		Progress:          0.75,
		TotalListenTimeMs: 2700000,
	}

	// User rewound and listened from 20min to 25min
	event := &ListeningEvent{
		StartPositionMs: 1200000, // 20 min
		EndPositionMs:   1500000, // 25 min
		DurationMs:      300000,  // 5 min of content
		EndedAt:         time.Now(),
	}

	bookDurationMs := int64(3600000)

	progress.UpdateFromEvent(event, bookDurationMs)

	// Position should NOT go backwards - keep the furthest point
	assert.Equal(t, int64(2700000), progress.CurrentPositionMs)
	assert.Equal(t, 0.75, progress.Progress)
	// But total listen time DOES accumulate (they listened to content)
	assert.Equal(t, int64(3000000), progress.TotalListenTimeMs) // 45 + 5 min
}

func TestPlaybackProgress_DetectsCompletion(t *testing.T) {
	bookDurationMs := int64(3600000) // 1 hour

	tests := []struct {
		name         string
		positionMs   int64
		wantFinished bool
	}{
		{
			name:         "50% - not finished",
			positionMs:   1800000,
			wantFinished: false,
		},
		{
			name:         "98% - not finished",
			positionMs:   3528000,
			wantFinished: false,
		},
		{
			name:         "99% - finished",
			positionMs:   3564000,
			wantFinished: true,
		},
		{
			name:         "100% - finished",
			positionMs:   3600000,
			wantFinished: true,
		},
		{
			name:         "beyond 100% - finished",
			positionMs:   3700000,
			wantFinished: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &ListeningEvent{
				UserID:        "user-123",
				BookID:        "book-456",
				EndPositionMs: tt.positionMs,
				DurationMs:    tt.positionMs,
				StartedAt:     time.Now(),
				EndedAt:       time.Now(),
			}

			progress := NewPlaybackProgress(event, bookDurationMs)

			assert.Equal(t, tt.wantFinished, progress.IsFinished)
			if tt.wantFinished {
				assert.NotNil(t, progress.FinishedAt)
			} else {
				assert.Nil(t, progress.FinishedAt)
			}
		})
	}
}
