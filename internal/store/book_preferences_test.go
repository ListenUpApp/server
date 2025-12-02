package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookPreferencesCRUD(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	speed := float32(1.25)
	prefs := &domain.BookPreferences{
		UserID:        "user-123",
		BookID:        "book-456",
		PlaybackSpeed: &speed,
		UpdatedAt:     time.Now(),
	}

	// Create
	err := s.UpsertBookPreferences(ctx, prefs)
	require.NoError(t, err)

	// Read
	retrieved, err := s.GetBookPreferences(ctx, "user-123", "book-456")
	require.NoError(t, err)
	require.NotNil(t, retrieved.PlaybackSpeed)
	assert.Equal(t, float32(1.25), *retrieved.PlaybackSpeed)

	// Update
	newSpeed := float32(1.5)
	prefs.PlaybackSpeed = &newSpeed
	prefs.HideFromContinueListening = true
	err = s.UpsertBookPreferences(ctx, prefs)
	require.NoError(t, err)

	retrieved, err = s.GetBookPreferences(ctx, "user-123", "book-456")
	require.NoError(t, err)
	assert.Equal(t, float32(1.5), *retrieved.PlaybackSpeed)
	assert.True(t, retrieved.HideFromContinueListening)

	// Delete
	err = s.DeleteBookPreferences(ctx, "user-123", "book-456")
	require.NoError(t, err)

	_, err = s.GetBookPreferences(ctx, "user-123", "book-456")
	assert.ErrorIs(t, err, store.ErrBookPreferencesNotFound)
}

func TestGetAllBookPreferences(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create preferences for multiple books
	bookIDs := []string{"book-1", "book-2", "book-3"}
	for _, bookID := range bookIDs {
		prefs := &domain.BookPreferences{
			UserID:    "user-123",
			BookID:    bookID,
			UpdatedAt: now,
		}
		require.NoError(t, s.UpsertBookPreferences(ctx, prefs))
	}

	// Also create preferences for another user
	require.NoError(t, s.UpsertBookPreferences(ctx, &domain.BookPreferences{
		UserID:    "user-other",
		BookID:    "book-1",
		UpdatedAt: now,
	}))

	// Get user-123's preferences
	allPrefs, err := s.GetAllBookPreferences(ctx, "user-123")
	require.NoError(t, err)
	assert.Len(t, allPrefs, 3)
}

func TestGetAllBookPreferences_Empty(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	// Get preferences for user with none
	allPrefs, err := s.GetAllBookPreferences(ctx, "nonexistent-user")
	require.NoError(t, err)
	assert.Empty(t, allPrefs)
}

func TestBookPreferences_NilFields(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create preferences with nil optional fields
	prefs := domain.NewBookPreferences("user-123", "book-456")
	require.NoError(t, s.UpsertBookPreferences(ctx, prefs))

	// Retrieve and verify nil fields stay nil
	retrieved, err := s.GetBookPreferences(ctx, "user-123", "book-456")
	require.NoError(t, err)
	assert.Nil(t, retrieved.PlaybackSpeed)
	assert.Nil(t, retrieved.SkipForwardSec)
	assert.False(t, retrieved.HideFromContinueListening)
}
