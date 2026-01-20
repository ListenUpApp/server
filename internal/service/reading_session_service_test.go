package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/listenupapp/listenup-server/internal/color"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

func setupTestReadingSession(t *testing.T) (*ReadingSessionService, *store.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "reading-session-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	testStore, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	logger := slog.New(slog.DiscardHandler)
	svc := NewReadingSessionService(testStore, store.NewNoopEmitter(), logger)

	cleanup := func() {
		testStore.Close()
		os.RemoveAll(tmpDir)
	}

	return svc, testStore, cleanup
}

func createTestBookForSession(t *testing.T, s *store.Store, bookID string, durationMs int64) {
	t.Helper()
	ctx := context.Background()

	book := &domain.Book{
		Syncable: domain.Syncable{
			ID: bookID,
		},
		Title:         "Test Book " + bookID,
		TotalDuration: durationMs,
	}
	book.InitTimestamps()
	require.NoError(t, s.CreateBook(ctx, book))
}

func TestEnsureActiveSession_CreateNew(t *testing.T) {
	svc, _, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// No active session exists
	session, err := svc.EnsureActiveSession(ctx, "user-1", "book-1")
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, "user-1", session.UserID)
	assert.Equal(t, "book-1", session.BookID)
	assert.True(t, session.IsActive())
	assert.False(t, session.IsCompleted)
	assert.Equal(t, int64(0), session.ListenTimeMs)
}

func TestEnsureActiveSession_ReturnExisting(t *testing.T) {
	svc, _, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Create first session
	session1, err := svc.EnsureActiveSession(ctx, "user-1", "book-1")
	require.NoError(t, err)

	// Ensure again - should return same session
	session2, err := svc.EnsureActiveSession(ctx, "user-1", "book-1")
	require.NoError(t, err)

	assert.Equal(t, session1.ID, session2.ID)
	assert.True(t, session1.StartedAt.Equal(session2.StartedAt))
}

func TestEnsureActiveSession_AbandonStale(t *testing.T) {
	svc, s, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Create a session and make it stale by backdating UpdatedAt
	oldSession := domain.NewBookReadingSession("old-session", "user-1", "book-1")
	oldSession.UpdatedAt = time.Now().Add(-7 * 30 * 24 * time.Hour) // 7 months ago
	err := s.CreateReadingSession(ctx, oldSession)
	require.NoError(t, err)

	// Ensure active session - should create new and abandon old
	newSession, err := svc.EnsureActiveSession(ctx, "user-1", "book-1")
	require.NoError(t, err)
	require.NotNil(t, newSession)

	assert.NotEqual(t, oldSession.ID, newSession.ID)
	assert.True(t, newSession.IsActive())

	// Verify old session was abandoned
	abandoned, err := s.GetReadingSession(ctx, oldSession.ID)
	require.NoError(t, err)
	assert.False(t, abandoned.IsActive())
	assert.NotNil(t, abandoned.FinishedAt)
}

func TestUpdateSessionProgress(t *testing.T) {
	svc, s, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Create session
	session, err := svc.EnsureActiveSession(ctx, "user-1", "book-1")
	require.NoError(t, err)

	// Update progress
	err = svc.UpdateSessionProgress(ctx, "user-1", "book-1", 120000) // 2 minutes
	require.NoError(t, err)

	// Verify update
	updated, err := s.GetReadingSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(120000), updated.ListenTimeMs)
	assert.True(t, updated.UpdatedAt.After(session.UpdatedAt))
}

func TestUpdateSessionProgress_NoActiveSession(t *testing.T) {
	svc, _, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Try to update non-existent session
	err := svc.UpdateSessionProgress(ctx, "user-1", "book-1", 120000)
	assert.Error(t, err)
}

func TestCompleteSession(t *testing.T) {
	svc, s, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Create session
	session, err := svc.EnsureActiveSession(ctx, "user-1", "book-1")
	require.NoError(t, err)

	// Complete session
	err = svc.CompleteSession(ctx, "user-1", "book-1", 0.995, 3600000) // 99.5% complete, 1 hour
	require.NoError(t, err)

	// Verify completion
	completed, err := s.GetReadingSession(ctx, session.ID)
	require.NoError(t, err)
	assert.True(t, completed.IsCompleted)
	assert.False(t, completed.IsActive())
	assert.NotNil(t, completed.FinishedAt)
	assert.Equal(t, 0.995, completed.FinalProgress)
	assert.Equal(t, int64(3600000), completed.ListenTimeMs)
}

func TestAbandonSession(t *testing.T) {
	svc, s, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Create book first (1 hour duration)
	createTestBookForSession(t, s, "book-1", 3600000)

	// Create session and some progress
	session, err := svc.EnsureActiveSession(ctx, "user-1", "book-1")
	require.NoError(t, err)

	// Create progress in store (CurrentPositionMs = 45% of 60 minutes = 27 minutes)
	progress := &domain.PlaybackState{
		UserID:            "user-1",
		BookID:            "book-1",
		CurrentPositionMs: 1620000, // 27 minutes = 45% of 60 min
		TotalListenTimeMs: 1800000, // 30 minutes
	}
	err = s.UpsertState(ctx, progress)
	require.NoError(t, err)

	// Abandon session
	err = svc.AbandonSession(ctx, "user-1", "book-1")
	require.NoError(t, err)

	// Verify abandonment
	abandoned, err := s.GetReadingSession(ctx, session.ID)
	require.NoError(t, err)
	assert.False(t, abandoned.IsCompleted)
	assert.False(t, abandoned.IsActive())
	assert.NotNil(t, abandoned.FinishedAt)
	assert.Equal(t, 0.45, abandoned.FinalProgress)
	assert.Equal(t, int64(1800000), abandoned.ListenTimeMs)
}

func TestAbandonSession_NoActiveSession(t *testing.T) {
	svc, _, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Abandon non-existent session should not error
	err := svc.AbandonSession(ctx, "user-1", "book-1")
	assert.NoError(t, err)
}

func TestGetBookReaders(t *testing.T) {
	svc, s, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Create test users
	user1 := createTestUserForSession(t, s, "user-1", "alice@example.com", "Alice")
	user2 := createTestUserForSession(t, s, "user-2", "bob@example.com", "Bob")

	// User 1: 2 sessions (1 completed, 1 active)
	session1 := domain.NewBookReadingSession("session-1", user1.ID, "book-1")
	session1.MarkCompleted(1.0, 3600000)
	err := s.CreateReadingSession(ctx, session1)
	require.NoError(t, err)

	session2 := domain.NewBookReadingSession("session-2", user1.ID, "book-1")
	err = s.CreateReadingSession(ctx, session2)
	require.NoError(t, err)

	// User 2: 1 session (completed)
	session3 := domain.NewBookReadingSession("session-3", user2.ID, "book-1")
	session3.MarkCompleted(1.0, 1800000)
	err = s.CreateReadingSession(ctx, session3)
	require.NoError(t, err)

	// Get book readers from user1's perspective
	response, err := svc.GetBookReaders(ctx, "book-1", user1.ID, 10)
	require.NoError(t, err)

	// Verify response
	assert.Len(t, response.YourSessions, 2)
	assert.Len(t, response.OtherReaders, 1)
	assert.Equal(t, 2, response.TotalReaders)
	assert.Equal(t, 2, response.TotalCompletions)

	// Verify user2's summary
	bobSummary := response.OtherReaders[0]
	assert.Equal(t, user2.ID, bobSummary.UserID)
	assert.Equal(t, "Bob", bobSummary.DisplayName)
	assert.NotEmpty(t, bobSummary.AvatarColor)
	assert.False(t, bobSummary.IsCurrentlyReading)
	assert.Equal(t, 1, bobSummary.CompletionCount)
}

func TestGetUserReadingHistory(t *testing.T) {
	svc, s, cleanup := setupTestReadingSession(t)
	defer cleanup()

	ctx := context.Background()

	// Create test user and books
	user := createTestUserForSession(t, s, "user-1", "alice@example.com", "Alice")

	book1 := &domain.Book{
		Syncable: domain.Syncable{
			ID: "book-1",
		},
		Title:         "The Great Book",
		TotalDuration: 3600000,
	}
	book1.InitTimestamps()
	err := s.CreateBook(ctx, book1)
	require.NoError(t, err)

	book2 := &domain.Book{
		Syncable: domain.Syncable{
			ID: "book-2",
		},
		Title:         "Another Book",
		TotalDuration: 1800000,
	}
	book2.InitTimestamps()
	err = s.CreateBook(ctx, book2)
	require.NoError(t, err)

	// Create sessions (session2 started later, so it should appear first in history)
	session1 := domain.NewBookReadingSession("session-1", user.ID, book1.ID)
	session1.StartedAt = time.Now().Add(-2 * time.Hour) // Started 2 hours ago
	session1.MarkCompleted(1.0, 3600000)
	err = s.CreateReadingSession(ctx, session1)
	require.NoError(t, err)

	session2 := domain.NewBookReadingSession("session-2", user.ID, book2.ID)
	session2.StartedAt = time.Now().Add(-1 * time.Hour) // Started 1 hour ago (more recent)
	session2.ListenTimeMs = 1800000
	err = s.CreateReadingSession(ctx, session2)
	require.NoError(t, err)

	// Get reading history
	response, err := svc.GetUserReadingHistory(ctx, user.ID, 10)
	require.NoError(t, err)

	// Verify response
	assert.Len(t, response.Sessions, 2)
	assert.Equal(t, 2, response.TotalSessions)
	assert.Equal(t, 1, response.TotalCompleted)

	// Verify first session details (most recent = session2)
	historyItem := response.Sessions[0]
	assert.Equal(t, "session-2", historyItem.ID)
	assert.Equal(t, book2.ID, historyItem.BookID)
	assert.Equal(t, "Another Book", historyItem.BookTitle)
	assert.False(t, historyItem.IsCompleted)
	assert.Equal(t, int64(1800000), historyItem.ListenTimeMs)

	// Verify second session details (older = session1)
	historyItem2 := response.Sessions[1]
	assert.Equal(t, "session-1", historyItem2.ID)
	assert.Equal(t, book1.ID, historyItem2.BookID)
	assert.Equal(t, "The Great Book", historyItem2.BookTitle)
	assert.True(t, historyItem2.IsCompleted)
	assert.Equal(t, int64(3600000), historyItem2.ListenTimeMs)
}

func TestAvatarColorForUser(t *testing.T) {
	// Test that avatar color is consistent for same user
	color1 := color.ForUser("user-123")
	color2 := color.ForUser("user-123")
	assert.Equal(t, color1, color2)

	// Test that different users get different colors
	color3 := color.ForUser("user-456")
	assert.NotEqual(t, color1, color3)

	// Test that color is valid hex
	assert.Regexp(t, `^#[0-9A-F]{6}$`, color1)
	assert.Regexp(t, `^#[0-9A-F]{6}$`, color3)
}

// Helper functions

func createTestUserForSession(t *testing.T, s *store.Store, id, email, displayName string) *domain.User {
	t.Helper()
	user := &domain.User{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Email:       email,
		DisplayName: displayName,
	}
	err := s.CreateUser(context.Background(), user)
	require.NoError(t, err)
	return user
}
