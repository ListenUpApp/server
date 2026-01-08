package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestListeningStore(t *testing.T) (*store.Store, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "listening-store-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	cleanup := func() {
		s.Close()
		os.RemoveAll(tmpDir)
	}
	return s, cleanup
}

func TestCreateListeningEvent(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()
	event := &domain.ListeningEvent{
		ID:              "evt-123",
		UserID:          "user-456",
		BookID:          "book-789",
		StartPositionMs: 0,
		EndPositionMs:   1800000,
		StartedAt:       time.Now(),
		EndedAt:         time.Now(),
		PlaybackSpeed:   1.0,
		DeviceID:        "device-1",
		DurationMs:      1800000,
		CreatedAt:       time.Now(),
	}

	err := s.CreateListeningEvent(ctx, event)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := s.GetListeningEvent(ctx, "evt-123")
	require.NoError(t, err)
	assert.Equal(t, event.ID, retrieved.ID)
	assert.Equal(t, event.UserID, retrieved.UserID)
	assert.Equal(t, event.BookID, retrieved.BookID)
}

func TestGetEventsForUser(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create events for two users
	events := []*domain.ListeningEvent{
		{ID: "evt-1", UserID: "user-A", BookID: "book-1", CreatedAt: time.Now()},
		{ID: "evt-2", UserID: "user-A", BookID: "book-2", CreatedAt: time.Now()},
		{ID: "evt-3", UserID: "user-B", BookID: "book-1", CreatedAt: time.Now()},
	}

	for _, e := range events {
		require.NoError(t, s.CreateListeningEvent(ctx, e))
	}

	// Get user-A's events
	userAEvents, err := s.GetEventsForUser(ctx, "user-A")
	require.NoError(t, err)
	assert.Len(t, userAEvents, 2)

	// Get user-B's events
	userBEvents, err := s.GetEventsForUser(ctx, "user-B")
	require.NoError(t, err)
	assert.Len(t, userBEvents, 1)
}

func TestGetEventsForBook(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	events := []*domain.ListeningEvent{
		{ID: "evt-1", UserID: "user-A", BookID: "book-1", CreatedAt: time.Now()},
		{ID: "evt-2", UserID: "user-B", BookID: "book-1", CreatedAt: time.Now()},
		{ID: "evt-3", UserID: "user-A", BookID: "book-2", CreatedAt: time.Now()},
	}

	for _, e := range events {
		require.NoError(t, s.CreateListeningEvent(ctx, e))
	}

	// Get book-1's events (from both users)
	book1Events, err := s.GetEventsForBook(ctx, "book-1")
	require.NoError(t, err)
	assert.Len(t, book1Events, 2)
}

func TestGetEventsForUserBook(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	events := []*domain.ListeningEvent{
		{ID: "evt-1", UserID: "user-A", BookID: "book-1", CreatedAt: time.Now()},
		{ID: "evt-2", UserID: "user-A", BookID: "book-1", CreatedAt: time.Now()},
		{ID: "evt-3", UserID: "user-A", BookID: "book-2", CreatedAt: time.Now()},
		{ID: "evt-4", UserID: "user-B", BookID: "book-1", CreatedAt: time.Now()},
	}

	for _, e := range events {
		require.NoError(t, s.CreateListeningEvent(ctx, e))
	}

	// Get user-A + book-1 events only
	result, err := s.GetEventsForUserBook(ctx, "user-A", "book-1")
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestGetEventsForUserInRange(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create events at different times
	events := []*domain.ListeningEvent{
		{ID: "evt-1", UserID: "user-A", BookID: "book-1", EndedAt: now.Add(-3 * 24 * time.Hour)}, // 3 days ago
		{ID: "evt-2", UserID: "user-A", BookID: "book-1", EndedAt: now.Add(-2 * 24 * time.Hour)}, // 2 days ago
		{ID: "evt-3", UserID: "user-A", BookID: "book-1", EndedAt: now.Add(-1 * 24 * time.Hour)}, // 1 day ago
		{ID: "evt-4", UserID: "user-A", BookID: "book-1", EndedAt: now},                          // now
		{ID: "evt-5", UserID: "user-B", BookID: "book-1", EndedAt: now.Add(-1 * 24 * time.Hour)}, // different user
	}

	for _, e := range events {
		e.StartPositionMs = 0
		e.EndPositionMs = 1000
		e.StartedAt = e.EndedAt.Add(-time.Minute)
		e.PlaybackSpeed = 1.0
		e.DeviceID = "device-1"
		e.DurationMs = 1000
		e.CreatedAt = e.EndedAt
		require.NoError(t, s.CreateListeningEvent(ctx, e))
	}

	// Query last 2 days (should get evt-3 and evt-4, not evt-1, evt-2, or user-B's event)
	start := now.Add(-2*24*time.Hour + time.Hour) // 2 days ago + 1 hour (after evt-2)
	end := now.Add(time.Hour)                     // 1 hour from now

	result, err := s.GetEventsForUserInRange(ctx, "user-A", start, end)
	require.NoError(t, err)
	assert.Len(t, result, 2, "should return events within range")

	// Verify we got the right events
	ids := make(map[string]bool)
	for _, e := range result {
		ids[e.ID] = true
	}
	assert.True(t, ids["evt-3"], "should include evt-3")
	assert.True(t, ids["evt-4"], "should include evt-4")
	assert.False(t, ids["evt-1"], "should not include evt-1 (too old)")
	assert.False(t, ids["evt-2"], "should not include evt-2 (too old)")
	assert.False(t, ids["evt-5"], "should not include evt-5 (different user)")

	// Query with zero start (all time)
	allEvents, err := s.GetEventsForUserInRange(ctx, "user-A", time.Time{}, now.Add(time.Hour))
	require.NoError(t, err)
	assert.Len(t, allEvents, 4, "should return all events for user-A")
}

func TestProgressCRUD(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	progress := &domain.PlaybackProgress{
		UserID:            "user-123",
		BookID:            "book-456",
		CurrentPositionMs: 1800000,
		Progress:          0.5,
		TotalListenTimeMs: 1800000,
		StartedAt:         time.Now(),
		LastPlayedAt:      time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Create
	err := s.UpsertProgress(ctx, progress)
	require.NoError(t, err)

	// Read
	retrieved, err := s.GetProgress(ctx, "user-123", "book-456")
	require.NoError(t, err)
	assert.Equal(t, progress.CurrentPositionMs, retrieved.CurrentPositionMs)
	assert.Equal(t, progress.Progress, retrieved.Progress)

	// Update
	progress.CurrentPositionMs = 2700000
	progress.Progress = 0.75
	err = s.UpsertProgress(ctx, progress)
	require.NoError(t, err)

	retrieved, err = s.GetProgress(ctx, "user-123", "book-456")
	require.NoError(t, err)
	assert.Equal(t, int64(2700000), retrieved.CurrentPositionMs)

	// Delete
	err = s.DeleteProgress(ctx, "user-123", "book-456")
	require.NoError(t, err)

	_, err = s.GetProgress(ctx, "user-123", "book-456")
	assert.ErrorIs(t, err, store.ErrProgressNotFound)
}

func TestGetProgressForUser(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create progress for multiple books
	for i, bookID := range []string{"book-1", "book-2", "book-3"} {
		progress := &domain.PlaybackProgress{
			UserID:            "user-123",
			BookID:            bookID,
			CurrentPositionMs: int64(i * 1000),
			UpdatedAt:         time.Now(),
		}
		require.NoError(t, s.UpsertProgress(ctx, progress))
	}

	// Also create progress for another user
	require.NoError(t, s.UpsertProgress(ctx, &domain.PlaybackProgress{
		UserID:    "user-other",
		BookID:    "book-1",
		UpdatedAt: time.Now(),
	}))

	// Get user-123's progress
	allProgress, err := s.GetProgressForUser(ctx, "user-123")
	require.NoError(t, err)
	assert.Len(t, allProgress, 3)
}

func TestGetContinueListening_FiltersCorrectly(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create various progress states
	progressData := []struct {
		bookID       string
		progress     float64
		isFinished   bool
		lastPlayed   time.Time
		hideFromCont bool
	}{
		{"book-active-1", 0.5, false, now.Add(-1 * time.Hour), false},    // Should include
		{"book-active-2", 0.3, false, now.Add(-2 * time.Hour), false},    // Should include
		{"book-finished", 1.0, true, now.Add(-30 * time.Minute), false},  // Exclude: finished
		{"book-hidden", 0.6, false, now.Add(-10 * time.Minute), true},    // Exclude: hidden
		{"book-not-started", 0.0, false, now.Add(-3 * time.Hour), false}, // Exclude: no progress
	}

	for _, pd := range progressData {
		progress := &domain.PlaybackProgress{
			UserID:       "user-123",
			BookID:       pd.bookID,
			Progress:     pd.progress,
			IsFinished:   pd.isFinished,
			LastPlayedAt: pd.lastPlayed,
			UpdatedAt:    now,
		}
		require.NoError(t, s.UpsertProgress(ctx, progress))

		if pd.hideFromCont {
			prefs := &domain.BookPreferences{
				UserID:                    "user-123",
				BookID:                    pd.bookID,
				HideFromContinueListening: true,
				UpdatedAt:                 now,
			}
			require.NoError(t, s.UpsertBookPreferences(ctx, prefs))
		}
	}

	// Get continue listening
	results, err := s.GetContinueListening(ctx, "user-123", 10)
	require.NoError(t, err)

	// Should only have the two active, non-hidden books
	assert.Len(t, results, 2)

	// Should be sorted by LastPlayedAt descending (most recent first)
	assert.Equal(t, "book-active-1", results[0].BookID)
	assert.Equal(t, "book-active-2", results[1].BookID)
}

func TestGetContinueListening_RespectsLimit(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create 5 active books
	for i := 1; i <= 5; i++ {
		progress := &domain.PlaybackProgress{
			UserID:       "user-123",
			BookID:       fmt.Sprintf("book-%d", i),
			Progress:     0.5,
			LastPlayedAt: now.Add(time.Duration(-i) * time.Hour),
			UpdatedAt:    now,
		}
		require.NoError(t, s.UpsertProgress(ctx, progress))
	}

	// Get with limit of 3
	results, err := s.GetContinueListening(ctx, "user-123", 3)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}
