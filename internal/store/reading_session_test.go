package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_CreateReadingSession(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	session := domain.NewBookReadingSession("session-1", "user-1", "book-1")

	err := s.CreateReadingSession(context.Background(), session)
	require.NoError(t, err)

	// Verify retrieval
	retrieved, err := s.GetReadingSession(context.Background(), "session-1")
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.UserID, retrieved.UserID)
	assert.Equal(t, session.BookID, retrieved.BookID)
	assert.True(t, retrieved.IsActive())
}

func TestStore_CreateReadingSession_DuplicateID(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	session := domain.NewBookReadingSession("session-1", "user-1", "book-1")

	err := s.CreateReadingSession(context.Background(), session)
	require.NoError(t, err)

	// Try to create again with same ID
	err = s.CreateReadingSession(context.Background(), session)
	require.Error(t, err)
}

func TestStore_GetReadingSession_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := s.GetReadingSession(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestStore_UpdateReadingSession(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	session := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	err := s.CreateReadingSession(context.Background(), session)
	require.NoError(t, err)

	// Update progress
	session.UpdateProgress(1800000) // 30 minutes

	err = s.UpdateReadingSession(context.Background(), session)
	require.NoError(t, err)

	// Verify update
	retrieved, err := s.GetReadingSession(context.Background(), "session-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1800000), retrieved.ListenTimeMs)
}

func TestStore_DeleteReadingSession(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	session := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	err := s.CreateReadingSession(context.Background(), session)
	require.NoError(t, err)

	// Delete
	err = s.DeleteReadingSession(context.Background(), "session-1")
	require.NoError(t, err)

	// Verify deletion
	_, err = s.GetReadingSession(context.Background(), "session-1")
	require.Error(t, err)
}

func TestStore_DeleteReadingSession_Idempotent(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Delete non-existent session should not error
	err := s.DeleteReadingSession(context.Background(), "nonexistent")
	require.NoError(t, err)
}

func TestStore_GetActiveSession_NoSession(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	session, err := s.GetActiveSession(context.Background(), "user-1", "book-1")
	require.NoError(t, err)
	assert.Nil(t, session)
}

func TestStore_GetActiveSession_OneActive(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create an active session
	activeSession := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	err := s.CreateReadingSession(context.Background(), activeSession)
	require.NoError(t, err)

	// Retrieve active session
	retrieved, err := s.GetActiveSession(context.Background(), "user-1", "book-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "session-1", retrieved.ID)
	assert.True(t, retrieved.IsActive())
}

func TestStore_GetActiveSession_OnlyInactive(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create an inactive session
	inactiveSession := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	inactiveSession.MarkCompleted(0.99, 3600000)
	err := s.CreateReadingSession(context.Background(), inactiveSession)
	require.NoError(t, err)

	// Should not return inactive session
	retrieved, err := s.GetActiveSession(context.Background(), "user-1", "book-1")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestStore_GetActiveSession_MultipleActive(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create two active sessions for same user+book (data integrity issue)
	session1 := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	session1.UpdatedAt = time.Now().Add(-1 * time.Hour) // Older
	err := s.CreateReadingSession(context.Background(), session1)
	require.NoError(t, err)

	time.Sleep(2 * time.Millisecond) // Ensure different timestamps
	session2 := domain.NewBookReadingSession("session-2", "user-1", "book-1")
	session2.UpdatedAt = time.Now() // Newer
	err = s.CreateReadingSession(context.Background(), session2)
	require.NoError(t, err)

	// Should return the most recently updated session
	retrieved, err := s.GetActiveSession(context.Background(), "user-1", "book-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "session-2", retrieved.ID)
}

func TestStore_GetActiveSession_DifferentBooks(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create session for book-1
	session1 := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	err := s.CreateReadingSession(context.Background(), session1)
	require.NoError(t, err)

	// Create session for book-2
	session2 := domain.NewBookReadingSession("session-2", "user-1", "book-2")
	err = s.CreateReadingSession(context.Background(), session2)
	require.NoError(t, err)

	// Get active session for book-1
	retrieved, err := s.GetActiveSession(context.Background(), "user-1", "book-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "session-1", retrieved.ID)

	// Get active session for book-2
	retrieved, err = s.GetActiveSession(context.Background(), "user-1", "book-2")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "session-2", retrieved.ID)
}

func TestStore_GetUserReadingSessions_Empty(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	sessions, err := s.GetUserReadingSessions(context.Background(), "user-1", 0)
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestStore_GetUserReadingSessions_Multiple(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	now := time.Now()

	// Create sessions at different times
	session1 := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	session1.StartedAt = now.Add(-3 * 24 * time.Hour) // 3 days ago
	err := s.CreateReadingSession(context.Background(), session1)
	require.NoError(t, err)

	session2 := domain.NewBookReadingSession("session-2", "user-1", "book-2")
	session2.StartedAt = now.Add(-1 * 24 * time.Hour) // 1 day ago (most recent)
	err = s.CreateReadingSession(context.Background(), session2)
	require.NoError(t, err)

	session3 := domain.NewBookReadingSession("session-3", "user-1", "book-1")
	session3.StartedAt = now.Add(-7 * 24 * time.Hour) // 7 days ago (oldest)
	session3.MarkCompleted(1.0, 3600000)
	err = s.CreateReadingSession(context.Background(), session3)
	require.NoError(t, err)

	// Get all sessions - should be sorted by StartedAt descending
	sessions, err := s.GetUserReadingSessions(context.Background(), "user-1", 0)
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	// Verify order (most recent first)
	assert.Equal(t, "session-2", sessions[0].ID)
	assert.Equal(t, "session-1", sessions[1].ID)
	assert.Equal(t, "session-3", sessions[2].ID)
}

func TestStore_GetUserReadingSessions_WithLimit(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	now := time.Now()

	// Create 5 sessions
	for i := range 5 {
		session := domain.NewBookReadingSession(
			"session-"+string(rune('0'+i)),
			"user-1",
			"book-1",
		)
		session.StartedAt = now.Add(time.Duration(-i) * time.Hour)
		err := s.CreateReadingSession(context.Background(), session)
		require.NoError(t, err)
	}

	// Get only 3 sessions
	sessions, err := s.GetUserReadingSessions(context.Background(), "user-1", 3)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestStore_GetUserReadingSessions_OnlyUserSessions(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create sessions for user-1
	session1 := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	err := s.CreateReadingSession(context.Background(), session1)
	require.NoError(t, err)

	// Create sessions for user-2
	session2 := domain.NewBookReadingSession("session-2", "user-2", "book-1")
	err = s.CreateReadingSession(context.Background(), session2)
	require.NoError(t, err)

	// Get user-1's sessions - should not include user-2's
	sessions, err := s.GetUserReadingSessions(context.Background(), "user-1", 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "session-1", sessions[0].ID)
}

func TestStore_GetBookSessions(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create sessions for book-1 from different users
	session1 := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	err := s.CreateReadingSession(context.Background(), session1)
	require.NoError(t, err)

	session2 := domain.NewBookReadingSession("session-2", "user-2", "book-1")
	err = s.CreateReadingSession(context.Background(), session2)
	require.NoError(t, err)

	// Create session for book-2
	session3 := domain.NewBookReadingSession("session-3", "user-1", "book-2")
	err = s.CreateReadingSession(context.Background(), session3)
	require.NoError(t, err)

	// Get book-1 sessions
	sessions, err := s.GetBookSessions(context.Background(), "book-1")
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// Verify both users' sessions are returned
	userIDs := make(map[string]bool)
	for _, session := range sessions {
		assert.Equal(t, "book-1", session.BookID)
		userIDs[session.UserID] = true
	}
	assert.True(t, userIDs["user-1"])
	assert.True(t, userIDs["user-2"])
}

func TestStore_ListAllSessions(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create multiple sessions
	for i := 1; i <= 3; i++ {
		session := domain.NewBookReadingSession(
			"session-"+string(rune('0'+i)),
			"user-1",
			"book-1",
		)
		err := s.CreateReadingSession(context.Background(), session)
		require.NoError(t, err)
	}

	// List all sessions
	count := 0
	for session, err := range s.ListAllSessions(context.Background()) {
		require.NoError(t, err)
		require.NotNil(t, session)
		count++
	}

	assert.Equal(t, 3, count)
}

func TestStore_ReadingSession_IndexIntegrity(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create a session
	session := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	err := s.CreateReadingSession(context.Background(), session)
	require.NoError(t, err)

	// Verify all indexes work
	// 1. user_book index
	activeSession, err := s.GetActiveSession(context.Background(), "user-1", "book-1")
	require.NoError(t, err)
	require.NotNil(t, activeSession)

	// 2. user index
	userSessions, err := s.GetUserReadingSessions(context.Background(), "user-1", 0)
	require.NoError(t, err)
	assert.Len(t, userSessions, 1)

	// 3. book index
	bookSessions, err := s.GetBookSessions(context.Background(), "book-1")
	require.NoError(t, err)
	assert.Len(t, bookSessions, 1)

	// Delete and verify indexes are cleaned up
	err = s.DeleteReadingSession(context.Background(), "session-1")
	require.NoError(t, err)

	// All indexes should be empty
	activeSession, err = s.GetActiveSession(context.Background(), "user-1", "book-1")
	require.NoError(t, err)
	assert.Nil(t, activeSession)

	userSessions, err = s.GetUserReadingSessions(context.Background(), "user-1", 0)
	require.NoError(t, err)
	assert.Empty(t, userSessions)

	bookSessions, err = s.GetBookSessions(context.Background(), "book-1")
	require.NoError(t, err)
	assert.Empty(t, bookSessions)
}

func TestStore_GetAllActiveSessions(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Create 3 sessions: 2 active, 1 completed
	session1 := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	session2 := domain.NewBookReadingSession("session-2", "user-2", "book-2")
	session3 := domain.NewBookReadingSession("session-3", "user-3", "book-3")

	// Mark session3 as completed (not active)
	session3.MarkCompleted(0.99, 3600000)

	require.NoError(t, s.CreateReadingSession(context.Background(), session1))
	require.NoError(t, s.CreateReadingSession(context.Background(), session2))
	require.NoError(t, s.CreateReadingSession(context.Background(), session3))

	// Get all active sessions
	activeSessions, err := s.GetAllActiveSessions(context.Background())
	require.NoError(t, err)

	// Should only return 2 active sessions
	assert.Len(t, activeSessions, 2)

	// Verify they are the right ones
	sessionIDs := make(map[string]bool)
	for _, s := range activeSessions {
		sessionIDs[s.ID] = true
	}
	assert.True(t, sessionIDs["session-1"], "session-1 should be active")
	assert.True(t, sessionIDs["session-2"], "session-2 should be active")
	assert.False(t, sessionIDs["session-3"], "session-3 should NOT be active (completed)")
}
