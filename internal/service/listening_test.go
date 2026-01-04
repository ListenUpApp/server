package service

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestListening(t *testing.T) (*ListeningService, *store.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "listening-service-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	testStore, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	readingSessionSvc := NewReadingSessionService(testStore, store.NewNoopEmitter(), logger)
	svc := NewListeningService(testStore, store.NewNoopEmitter(), readingSessionSvc, logger)

	cleanup := func() {
		testStore.Close()
		os.RemoveAll(tmpDir)
	}

	return svc, testStore, cleanup
}

func createTestBookForListening(t *testing.T, s *store.Store, bookID string, durationMs int64) {
	t.Helper()
	ctx := context.Background()

	book := &domain.Book{
		Syncable: domain.Syncable{
			ID: bookID,
		},
		Title:         "Test Book",
		TotalDuration: durationMs,
	}
	book.InitTimestamps()
	require.NoError(t, s.CreateBook(ctx, book))
}

func TestRecordEvent_CreatesEventAndProgress(t *testing.T) {
	svc, testStore, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test book first
	createTestBookForListening(t, testStore, "book-123", 3600000) // 1 hour

	// Record event
	req := RecordEventRequest{
		BookID:          "book-123",
		StartPositionMs: 0,
		EndPositionMs:   1800000, // 30 min
		StartedAt:       time.Now().Add(-30 * time.Minute),
		EndedAt:         time.Now(),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
		DeviceName:      "Test Device",
	}

	resp, err := svc.RecordEvent(ctx, "user-456", req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify event was created
	assert.NotEmpty(t, resp.Event.ID)
	assert.Equal(t, "user-456", resp.Event.UserID)
	assert.Equal(t, "book-123", resp.Event.BookID)
	assert.Equal(t, int64(1800000), resp.Event.DurationMs)

	// Verify progress was created
	assert.Equal(t, "user-456", resp.Progress.UserID)
	assert.Equal(t, "book-123", resp.Progress.BookID)
	assert.Equal(t, int64(1800000), resp.Progress.CurrentPositionMs)
	assert.Equal(t, 0.5, resp.Progress.Progress)
	assert.False(t, resp.Progress.IsFinished)
}

func TestRecordEvent_UpdatesExistingProgress(t *testing.T) {
	svc, testStore, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test book
	createTestBookForListening(t, testStore, "book-123", 3600000) // 1 hour

	// First event - listen to first 30 min
	req1 := RecordEventRequest{
		BookID:          "book-123",
		StartPositionMs: 0,
		EndPositionMs:   1800000, // 30 min
		StartedAt:       time.Now().Add(-60 * time.Minute),
		EndedAt:         time.Now().Add(-30 * time.Minute),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
	}

	resp1, err := svc.RecordEvent(ctx, "user-456", req1)
	require.NoError(t, err)
	assert.Equal(t, 0.5, resp1.Progress.Progress)

	// Second event - listen from 30 min to 45 min
	req2 := RecordEventRequest{
		BookID:          "book-123",
		StartPositionMs: 1800000,
		EndPositionMs:   2700000, // 45 min
		StartedAt:       time.Now().Add(-15 * time.Minute),
		EndedAt:         time.Now(),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
	}

	resp2, err := svc.RecordEvent(ctx, "user-456", req2)
	require.NoError(t, err)
	assert.Equal(t, 0.75, resp2.Progress.Progress)
	assert.Equal(t, int64(2700000), resp2.Progress.TotalListenTimeMs) // 30 + 15 min
}

func TestRecordEvent_MarksFinished(t *testing.T) {
	svc, testStore, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test book
	createTestBookForListening(t, testStore, "book-123", 3600000) // 1 hour

	// Listen to 99% of the book
	req := RecordEventRequest{
		BookID:          "book-123",
		StartPositionMs: 0,
		EndPositionMs:   3564000, // 99%
		StartedAt:       time.Now().Add(-60 * time.Minute),
		EndedAt:         time.Now(),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
	}

	resp, err := svc.RecordEvent(ctx, "user-456", req)
	require.NoError(t, err)
	assert.True(t, resp.Progress.IsFinished)
	assert.NotNil(t, resp.Progress.FinishedAt)
}

func TestRecordEvent_ValidationFails(t *testing.T) {
	svc, _, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Missing required fields
	req := RecordEventRequest{
		BookID: "", // Missing
	}

	_, err := svc.RecordEvent(ctx, "user-456", req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "book_id")
}

func TestGetContinueListening(t *testing.T) {
	svc, testStore, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Create test books
	createTestBookForListening(t, testStore, "book-1", 3600000)
	createTestBookForListening(t, testStore, "book-2", 3600000)
	createTestBookForListening(t, testStore, "book-3", 3600000)

	// Record events for multiple books
	for i, bookID := range []string{"book-1", "book-2", "book-3"} {
		req := RecordEventRequest{
			BookID:          bookID,
			StartPositionMs: 0,
			EndPositionMs:   int64((i + 1) * 600000), // 10, 20, 30 min
			StartedAt:       time.Now().Add(time.Duration(-i) * time.Hour),
			EndedAt:         time.Now().Add(time.Duration(-i) * time.Hour).Add(10 * time.Minute),
			PlaybackSpeed:   1.0,
			DeviceID:        "device-1",
		}
		_, err := svc.RecordEvent(ctx, "user-456", req)
		require.NoError(t, err)
	}

	// Get continue listening
	results, err := svc.GetContinueListening(ctx, "user-456", 10)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Most recent should be first
	assert.Equal(t, "book-1", results[0].BookID)
}

func TestUserSettings_CRUD(t *testing.T) {
	svc, _, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Get creates defaults
	settings, err := svc.GetUserSettings(ctx, "user-123")
	require.NoError(t, err)
	assert.Equal(t, float32(1.0), settings.DefaultPlaybackSpeed)
	assert.Equal(t, 30, settings.DefaultSkipForwardSec)

	// Update
	newSpeed := float32(1.5)
	newSkip := 45
	updated, err := svc.UpdateUserSettings(ctx, "user-123", UpdateUserSettingsRequest{
		DefaultPlaybackSpeed:  &newSpeed,
		DefaultSkipForwardSec: &newSkip,
	})
	require.NoError(t, err)
	assert.Equal(t, float32(1.5), updated.DefaultPlaybackSpeed)
	assert.Equal(t, 45, updated.DefaultSkipForwardSec)

	// Verify persistence
	settings, err = svc.GetUserSettings(ctx, "user-123")
	require.NoError(t, err)
	assert.Equal(t, float32(1.5), settings.DefaultPlaybackSpeed)
}

func TestBookPreferences_CRUD(t *testing.T) {
	svc, _, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Get returns defaults
	prefs, err := svc.GetBookPreferences(ctx, "user-123", "book-456")
	require.NoError(t, err)
	assert.Nil(t, prefs.PlaybackSpeed)
	assert.False(t, prefs.HideFromContinueListening)

	// Update
	speed := float32(2.0)
	hide := true
	updated, err := svc.UpdateBookPreferences(ctx, "user-123", "book-456", UpdateBookPreferencesRequest{
		PlaybackSpeed:             &speed,
		HideFromContinueListening: &hide,
	})
	require.NoError(t, err)
	assert.Equal(t, float32(2.0), *updated.PlaybackSpeed)
	assert.True(t, updated.HideFromContinueListening)

	// Verify persistence
	prefs, err = svc.GetBookPreferences(ctx, "user-123", "book-456")
	require.NoError(t, err)
	assert.Equal(t, float32(2.0), *prefs.PlaybackSpeed)
	assert.True(t, prefs.HideFromContinueListening)
}

func TestRecordEvent_Idempotency(t *testing.T) {
	svc, testStore, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test book
	createTestBookForListening(t, testStore, "book-123", 3600000) // 1 hour

	// Record event with client-provided ID
	clientEventID := "client-evt-12345"
	req := RecordEventRequest{
		EventID:         clientEventID,
		BookID:          "book-123",
		StartPositionMs: 0,
		EndPositionMs:   1800000, // 30 min
		StartedAt:       time.Now().Add(-30 * time.Minute),
		EndedAt:         time.Now(),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
		DeviceName:      "Test Device",
	}

	// First submission
	resp1, err := svc.RecordEvent(ctx, "user-456", req)
	require.NoError(t, err)
	require.NotNil(t, resp1)
	assert.Equal(t, clientEventID, resp1.Event.ID)

	// Second submission with same ID (should be idempotent)
	resp2, err := svc.RecordEvent(ctx, "user-456", req)
	require.NoError(t, err)
	require.NotNil(t, resp2)
	assert.Equal(t, clientEventID, resp2.Event.ID)

	// Verify same event was returned (not duplicated)
	assert.Equal(t, resp1.Event.ID, resp2.Event.ID)
	assert.Equal(t, resp1.Event.BookID, resp2.Event.BookID)
	assert.Equal(t, resp1.Event.StartPositionMs, resp2.Event.StartPositionMs)

	// Verify only one event exists in the store
	events, err := testStore.GetEventsForUser(ctx, "user-456")
	require.NoError(t, err)
	assert.Len(t, events, 1, "Expected exactly 1 event, not duplicates")
	assert.Equal(t, clientEventID, events[0].ID)
}

func TestGetUserStats(t *testing.T) {
	svc, testStore, cleanup := setupTestListening(t)
	defer cleanup()

	ctx := context.Background()

	// Create test books
	createTestBookForListening(t, testStore, "book-1", 3600000)
	createTestBookForListening(t, testStore, "book-2", 3600000)

	// Record events
	req1 := RecordEventRequest{
		BookID:          "book-1",
		StartPositionMs: 0,
		EndPositionMs:   1800000, // 30 min
		StartedAt:       time.Now().Add(-60 * time.Minute),
		EndedAt:         time.Now().Add(-30 * time.Minute),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
	}
	_, err := svc.RecordEvent(ctx, "user-456", req1)
	require.NoError(t, err)

	// Finish book-2
	req2 := RecordEventRequest{
		BookID:          "book-2",
		StartPositionMs: 0,
		EndPositionMs:   3564000, // 99%
		StartedAt:       time.Now().Add(-60 * time.Minute),
		EndedAt:         time.Now(),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
	}
	_, err = svc.RecordEvent(ctx, "user-456", req2)
	require.NoError(t, err)

	stats, err := svc.GetUserStats(ctx, "user-456")
	require.NoError(t, err)
	assert.Equal(t, int64(1800000+3564000), stats.TotalListenTimeMs)
	assert.Equal(t, 2, stats.BooksStarted)
	assert.Equal(t, 1, stats.BooksFinished)
}
