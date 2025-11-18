package store

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSession(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.Session{
		ID:               "session_test123",
		UserID:           "user_test123",
		RefreshTokenHash: "hashed_token",
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
		DeviceType:       "mobile",
		Platform:         "iOS",
		ClientName:       "ListenUp Mobile",
		ClientVersion:    "1.0.0",
	}

	err := store.CreateSession(ctx, session)
	require.NoError(t, err)

	// Verify session can be retrieved
	retrieved, err := store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.UserID, retrieved.UserID)
	assert.Equal(t, session.RefreshTokenHash, retrieved.RefreshTokenHash)
	assert.Equal(t, session.DeviceType, retrieved.DeviceType)
}

func TestCreateSession_DuplicateID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.Session{
		ID:               "session_test123",
		UserID:           "user_test123",
		RefreshTokenHash: "hashed_token",
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
	}

	// First creation succeeds
	err := store.CreateSession(ctx, session)
	require.NoError(t, err)

	// Second creation with same ID fails
	session2 := &domain.Session{
		ID:               "session_test123",
		UserID:           "user_test123",
		RefreshTokenHash: "different_token",
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
	}

	err = store.CreateSession(ctx, session2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestGetSession_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetSession(ctx, "session_nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestGetSession_Expired(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.Session{
		ID:               "session_test123",
		UserID:           "user_test123",
		RefreshTokenHash: "hashed_token",
		ExpiresAt:        time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
	}

	err := store.CreateSession(ctx, session)
	require.NoError(t, err)

	// Getting expired session should return error
	_, err = store.GetSession(ctx, session.ID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionExpired)
}

func TestGetSessionByRefreshToken_Success(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	tokenHash := "unique_token_hash_123"
	session := &domain.Session{
		ID:               "session_test123",
		UserID:           "user_test123",
		RefreshTokenHash: tokenHash,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
	}

	err := store.CreateSession(ctx, session)
	require.NoError(t, err)

	// Retrieve by token
	retrieved, err := store.GetSessionByRefreshToken(ctx, tokenHash)
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.RefreshTokenHash, retrieved.RefreshTokenHash)
}

func TestGetSessionByRefreshToken_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetSessionByRefreshToken(ctx, "nonexistent_token")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestUpdateSession_Success(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.Session{
		ID:               "session_test123",
		UserID:           "user_test123",
		RefreshTokenHash: "original_token",
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
		IPAddress:        "192.168.1.1",
	}

	err := store.CreateSession(ctx, session)
	require.NoError(t, err)

	// Wait a moment
	time.Sleep(10 * time.Millisecond)

	// Update session
	session.IPAddress = "192.168.1.2"
	session.Touch()
	err = store.UpdateSession(ctx, session)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.2", updated.IPAddress)
	assert.True(t, updated.LastSeenAt.After(session.CreatedAt))
}

func TestUpdateSession_TokenRotation(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	oldTokenHash := "old_token_hash"
	session := &domain.Session{
		ID:               "session_test123",
		UserID:           "user_test123",
		RefreshTokenHash: oldTokenHash,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
	}

	err := store.CreateSession(ctx, session)
	require.NoError(t, err)

	// Rotate token
	newTokenHash := "new_token_hash"
	session.RefreshTokenHash = newTokenHash
	err = store.UpdateSession(ctx, session)
	require.NoError(t, err)

	// Old token should not work
	_, err = store.GetSessionByRefreshToken(ctx, oldTokenHash)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionNotFound)

	// New token should work
	retrieved, err := store.GetSessionByRefreshToken(ctx, newTokenHash)
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
}

func TestDeleteSession_Success(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.Session{
		ID:               "session_test123",
		UserID:           "user_test123",
		RefreshTokenHash: "hashed_token",
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
	}

	err := store.CreateSession(ctx, session)
	require.NoError(t, err)

	// Delete session
	err = store.DeleteSession(ctx, session.ID)
	assert.NoError(t, err)

	// Session should not be found
	_, err = store.GetSession(ctx, session.ID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionNotFound)

	// Token should not work
	_, err = store.GetSessionByRefreshToken(ctx, session.RefreshTokenHash)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestDeleteSession_NonExistent(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Deleting non-existent session should not error
	err := store.DeleteSession(ctx, "session_nonexistent")
	assert.NoError(t, err)
}

func TestListUserSessions(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	userID := "user_test123"

	// Create multiple sessions for the same user
	sessions := []*domain.Session{
		{
			ID:               "session_test1",
			UserID:           userID,
			RefreshTokenHash: "token1",
			ExpiresAt:        time.Now().Add(24 * time.Hour),
			CreatedAt:        time.Now(),
			LastSeenAt:       time.Now(),
			DeviceType:       "mobile",
			Platform:         "iOS",
		},
		{
			ID:               "session_test2",
			UserID:           userID,
			RefreshTokenHash: "token2",
			ExpiresAt:        time.Now().Add(24 * time.Hour),
			CreatedAt:        time.Now(),
			LastSeenAt:       time.Now(),
			DeviceType:       "desktop",
			Platform:         "macOS",
		},
		{
			ID:               "session_test3",
			UserID:           userID,
			RefreshTokenHash: "token3",
			ExpiresAt:        time.Now().Add(24 * time.Hour),
			CreatedAt:        time.Now(),
			LastSeenAt:       time.Now(),
			DeviceType:       "web",
			Platform:         "Web",
		},
	}

	for _, session := range sessions {
		err := store.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// List sessions
	retrieved, err := store.ListUserSessions(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, retrieved, 3)

	// Verify all sessions are present
	ids := make(map[string]bool)
	for _, session := range retrieved {
		ids[session.ID] = true
		assert.Equal(t, userID, session.UserID)
	}

	assert.True(t, ids["session_test1"])
	assert.True(t, ids["session_test2"])
	assert.True(t, ids["session_test3"])
}

func TestListUserSessions_SkipsExpired(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	userID := "user_test123"

	// Create active session
	activeSession := &domain.Session{
		ID:               "session_active",
		UserID:           userID,
		RefreshTokenHash: "token_active",
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
	}
	err := store.CreateSession(ctx, activeSession)
	require.NoError(t, err)

	// Create expired session
	expiredSession := &domain.Session{
		ID:               "session_expired",
		UserID:           userID,
		RefreshTokenHash: "token_expired",
		ExpiresAt:        time.Now().Add(-1 * time.Hour),
		CreatedAt:        time.Now(),
		LastSeenAt:       time.Now(),
	}
	err = store.CreateSession(ctx, expiredSession)
	require.NoError(t, err)

	// List sessions should only return active
	retrieved, err := store.ListUserSessions(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, retrieved, 1)
	assert.Equal(t, "session_active", retrieved[0].ID)
}

func TestListUserSessions_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// List sessions for user with no sessions
	sessions, err := store.ListUserSessions(ctx, "user_nosessions")
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestDeleteExpiredSessions(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create mix of active and expired sessions
	activeSessions := []*domain.Session{
		{
			ID:               "session_active1",
			UserID:           "user1",
			RefreshTokenHash: "token_active1",
			ExpiresAt:        time.Now().Add(24 * time.Hour),
			CreatedAt:        time.Now(),
			LastSeenAt:       time.Now(),
		},
		{
			ID:               "session_active2",
			UserID:           "user2",
			RefreshTokenHash: "token_active2",
			ExpiresAt:        time.Now().Add(24 * time.Hour),
			CreatedAt:        time.Now(),
			LastSeenAt:       time.Now(),
		},
	}

	expiredSessions := []*domain.Session{
		{
			ID:               "session_expired1",
			UserID:           "user1",
			RefreshTokenHash: "token_expired1",
			ExpiresAt:        time.Now().Add(-1 * time.Hour),
			CreatedAt:        time.Now(),
			LastSeenAt:       time.Now(),
		},
		{
			ID:               "session_expired2",
			UserID:           "user2",
			RefreshTokenHash: "token_expired2",
			ExpiresAt:        time.Now().Add(-2 * time.Hour),
			CreatedAt:        time.Now(),
			LastSeenAt:       time.Now(),
		},
	}

	for _, session := range activeSessions {
		err := store.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	for _, session := range expiredSessions {
		err := store.CreateSession(ctx, session)
		require.NoError(t, err)
	}

	// Delete expired sessions
	count, err := store.DeleteExpiredSessions(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Verify active sessions still exist
	for _, session := range activeSessions {
		_, err := store.GetSession(ctx, session.ID)
		assert.NoError(t, err)
	}

	// Verify expired sessions are gone
	for _, session := range expiredSessions {
		_, err := store.GetSession(ctx, session.ID)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrSessionNotFound)
	}
}
