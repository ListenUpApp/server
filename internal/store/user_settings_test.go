package store_test

import (
	"context"
	"testing"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserSettingsCRUD(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	settings := domain.NewUserSettings("user-123")
	settings.DefaultPlaybackSpeed = 1.5
	settings.DefaultSkipForwardSec = 45

	// Create
	err := s.UpsertUserSettings(ctx, settings)
	require.NoError(t, err)

	// Read
	retrieved, err := s.GetUserSettings(ctx, "user-123")
	require.NoError(t, err)
	assert.Equal(t, float32(1.5), retrieved.DefaultPlaybackSpeed)
	assert.Equal(t, 45, retrieved.DefaultSkipForwardSec)

	// Update
	settings.DefaultPlaybackSpeed = 2.0
	err = s.UpsertUserSettings(ctx, settings)
	require.NoError(t, err)

	retrieved, err = s.GetUserSettings(ctx, "user-123")
	require.NoError(t, err)
	assert.Equal(t, float32(2.0), retrieved.DefaultPlaybackSpeed)

	// Delete
	err = s.DeleteUserSettings(ctx, "user-123")
	require.NoError(t, err)

	_, err = s.GetUserSettings(ctx, "user-123")
	assert.ErrorIs(t, err, store.ErrUserSettingsNotFound)
}

func TestGetOrCreateUserSettings_ExistingReturnsExisting(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create custom settings first
	settings := domain.NewUserSettings("user-123")
	settings.DefaultPlaybackSpeed = 1.75
	require.NoError(t, s.UpsertUserSettings(ctx, settings))

	// GetOrCreate should return existing
	retrieved, err := s.GetOrCreateUserSettings(ctx, "user-123")
	require.NoError(t, err)
	assert.Equal(t, float32(1.75), retrieved.DefaultPlaybackSpeed)
}

func TestGetOrCreateUserSettings_MissingCreatesDefaults(t *testing.T) {
	s, cleanup := setupTestListeningStore(t)
	defer cleanup()

	ctx := context.Background()

	// GetOrCreate on missing user should create defaults
	settings, err := s.GetOrCreateUserSettings(ctx, "new-user")
	require.NoError(t, err)

	// Should have defaults
	assert.Equal(t, "new-user", settings.UserID)
	assert.Equal(t, float32(1.0), settings.DefaultPlaybackSpeed)
	assert.Equal(t, 30, settings.DefaultSkipForwardSec)
	assert.Equal(t, 10, settings.DefaultSkipBackwardSec)

	// Should be persisted
	retrieved, err := s.GetUserSettings(ctx, "new-user")
	require.NoError(t, err)
	assert.Equal(t, float32(1.0), retrieved.DefaultPlaybackSpeed)
}
