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

func setupTestSocialService(t *testing.T) (*SocialService, *store.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "social-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	testStore, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewSocialService(testStore, logger)

	cleanup := func() {
		testStore.Close()
		os.RemoveAll(tmpDir)
	}

	return svc, testStore, cleanup
}

func createTestUserForSocial(t *testing.T, s *store.Store, id, displayName string, role domain.Role) *domain.User {
	t.Helper()
	user := &domain.User{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Email:       id + "@test.com",
		DisplayName: displayName,
		Role:        role,
	}
	err := s.CreateUser(context.Background(), user)
	require.NoError(t, err)
	return user
}

func createTestBookForSocial(t *testing.T, s *store.Store, id, title string) *domain.Book {
	t.Helper()
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID: id,
		},
		Title:         title,
		TotalDuration: 3600000,
	}
	book.InitTimestamps()
	err := s.CreateBook(context.Background(), book)
	require.NoError(t, err)
	return book
}

func TestSocialService_GetCurrentlyListening(t *testing.T) {
	svc, s, cleanup := setupTestSocialService(t)
	defer cleanup()

	ctx := context.Background()

	// Create test users
	user1 := createTestUserForSocial(t, s, "user-1", "User One", domain.RoleAdmin)
	createTestUserForSocial(t, s, "user-2", "User Two", domain.RoleMember)
	createTestUserForSocial(t, s, "user-3", "User Three", domain.RoleMember)

	// Create test books
	createTestBookForSocial(t, s, "book-1", "Book One")

	// Create active sessions for user2 and user3
	session2 := domain.NewBookReadingSession("session-2", "user-2", "book-1")
	session3 := domain.NewBookReadingSession("session-3", "user-3", "book-1")
	require.NoError(t, s.CreateReadingSession(ctx, session2))
	require.NoError(t, s.CreateReadingSession(ctx, session3))

	// Verify sessions are active
	allActive, err := s.GetAllActiveSessions(ctx)
	require.NoError(t, err)
	t.Logf("Total active sessions: %d", len(allActive))
	for _, sess := range allActive {
		t.Logf("  Session %s: user=%s, book=%s, active=%v, finishedAt=%v",
			sess.ID, sess.UserID, sess.BookID, sess.IsActive(), sess.FinishedAt)
	}

	// Get currently listening for user1 (should see user2 and user3 listening to book-1)
	result, err := svc.GetCurrentlyListening(ctx, user1.ID, 10)
	require.NoError(t, err)

	t.Logf("GetCurrentlyListening returned %d books", len(result))
	for _, book := range result {
		t.Logf("  Book %s: %d readers", book.Book.ID, len(book.Readers))
	}

	// Verify we got book-1 with 2 readers
	require.Len(t, result, 1, "Expected 1 book being read by others")
	assert.Equal(t, "book-1", result[0].Book.ID)
	assert.Equal(t, 2, result[0].TotalReaderCount)
	assert.Len(t, result[0].Readers, 2)
}

func TestSocialService_GetCurrentlyListening_NoActiveSessions(t *testing.T) {
	svc, s, cleanup := setupTestSocialService(t)
	defer cleanup()

	ctx := context.Background()

	// Create test user
	createTestUserForSocial(t, s, "user-1", "User One", domain.RoleAdmin)

	// No sessions exist
	result, err := svc.GetCurrentlyListening(ctx, "user-1", 10)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSocialService_GetCurrentlyListening_ExcludesSelf(t *testing.T) {
	svc, s, cleanup := setupTestSocialService(t)
	defer cleanup()

	ctx := context.Background()

	// Create test users
	createTestUserForSocial(t, s, "user-1", "User One", domain.RoleAdmin)

	// Create a book
	createTestBookForSocial(t, s, "book-1", "Book One")

	// Create a session for user1 (the viewing user)
	session1 := domain.NewBookReadingSession("session-1", "user-1", "book-1")
	require.NoError(t, s.CreateReadingSession(ctx, session1))

	// Get currently listening for user1 - should NOT see their own session
	result, err := svc.GetCurrentlyListening(ctx, "user-1", 10)
	require.NoError(t, err)
	assert.Empty(t, result, "User should not see their own sessions")
}

func TestSocialService_GetCurrentlyListening_ExcludesCompletedSessions(t *testing.T) {
	svc, s, cleanup := setupTestSocialService(t)
	defer cleanup()

	ctx := context.Background()

	// Create test users
	createTestUserForSocial(t, s, "user-1", "User One", domain.RoleAdmin)
	createTestUserForSocial(t, s, "user-2", "User Two", domain.RoleMember)

	// Create a book
	createTestBookForSocial(t, s, "book-1", "Book One")

	// Create a COMPLETED session for user2
	session2 := domain.NewBookReadingSession("session-2", "user-2", "book-1")
	session2.MarkCompleted(0.99, 3600000) // Mark as completed
	require.NoError(t, s.CreateReadingSession(ctx, session2))

	// Get currently listening for user1 - should NOT see completed session
	result, err := svc.GetCurrentlyListening(ctx, "user-1", 10)
	require.NoError(t, err)
	assert.Empty(t, result, "Completed sessions should not appear")
}
