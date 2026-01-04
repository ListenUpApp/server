package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createReadingSessionService creates a ReadingSessionService for testing.
func createReadingSessionService(t *testing.T, ts *testServer) *service.ReadingSessionService {
	t.Helper()
	return service.NewReadingSessionService(ts.store, store.NewNoopEmitter(), ts.logger)
}

// createTestUserAndLoginWithID creates a user and returns both access token and user ID.
func createTestUserAndLoginWithID(t *testing.T, ts *testServer) (string, string) {
	t.Helper()

	// Setup creates the first admin user
	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@test.com",
		"password":   "TestPassword123!",
		"first_name": "Test",
		"last_name":  "Admin",
	})

	require.Equal(t, http.StatusOK, resp.Code, "Setup failed: %s", resp.Body.String())

	var envelope testEnvelope[AuthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	return envelope.Data.AccessToken, envelope.Data.User.ID
}

// createTestBook creates a test book for testing.
func createTestBook(bookID string) *domain.Book {
	now := time.Now()
	return &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:         "Test Book",
		Path:          "/test/path/" + bookID,
		TotalDuration: 3600000,
		AudioFiles: []domain.AudioFileInfo{
			{
				ID:       "af-1",
				Path:     "/test/path/" + bookID + "/book.m4b",
				Filename: "book.m4b",
				Size:     1024000,
				Duration: 3600000,
				Format:   "m4b",
				Inode:    1001,
				ModTime:  now.Unix(),
			},
		},
	}
}

// createTestUser creates a test user for testing.
func createTestUser(userID, email, firstName, lastName string, role domain.Role) *domain.User {
	now := time.Now()
	return &domain.User{
		Syncable: domain.Syncable{
			ID:        userID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Email:       email,
		FirstName:   firstName,
		LastName:    lastName,
		DisplayName: firstName + " " + lastName,
		Role:        role,
		Status:      domain.UserStatusActive,
	}
}

func TestGetBookReaders_Success(t *testing.T) {
	ts := setupTestServerWithReadingSession(t)
	defer ts.cleanup()

	// Create users and auth token
	token, userID := createTestUserAndLoginWithID(t, ts)

	// Create a book
	bookID, err := id.Generate("book")
	require.NoError(t, err)

	book := createTestBook(bookID)
	require.NoError(t, ts.store.CreateBook(context.Background(), book))

	// Create a reading session for the user
	sessionID, err := id.Generate("rsession")
	require.NoError(t, err)

	session := domain.NewBookReadingSession(sessionID, userID, bookID)
	session.UpdateProgress(1800000) // 30 minutes
	require.NoError(t, ts.store.CreateReadingSession(context.Background(), session))

	// Get book readers
	resp := ts.api.Get("/api/v1/books/"+bookID+"/readers?limit=10",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[BookReadersResponse]
	err = json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.Equal(t, 1, len(envelope.Data.YourSessions))
	assert.Equal(t, sessionID, envelope.Data.YourSessions[0].ID)
	assert.Equal(t, int64(1800000), envelope.Data.YourSessions[0].ListenTimeMs)
	assert.Equal(t, 1, envelope.Data.TotalReaders)
	assert.Equal(t, 0, len(envelope.Data.OtherReaders))
}

func TestGetBookReaders_Unauthorized(t *testing.T) {
	ts := setupTestServerWithReadingSession(t)
	defer ts.cleanup()

	// Create a book
	bookID, err := id.Generate("book")
	require.NoError(t, err)

	book := createTestBook(bookID)
	require.NoError(t, ts.store.CreateBook(context.Background(), book))

	// Try to get readers without auth
	resp := ts.api.Get("/api/v1/books/" + bookID + "/readers")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestGetBookReaders_BookNotFound(t *testing.T) {
	ts := setupTestServerWithReadingSession(t)
	defer ts.cleanup()

	token := ts.createTestUserAndLogin(t)

	// Try to get readers for non-existent book
	resp := ts.api.Get("/api/v1/books/book_nonexistent/readers",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusNotFound, resp.Code)
}

func TestGetBookReaders_MultipleReaders(t *testing.T) {
	ts := setupTestServerWithReadingSession(t)
	defer ts.cleanup()

	// Create first user (admin)
	token1, user1ID := createTestUserAndLoginWithID(t, ts)

	// Create a book
	bookID, err := id.Generate("book")
	require.NoError(t, err)

	book := createTestBook(bookID)
	require.NoError(t, ts.store.CreateBook(context.Background(), book))

	// Create session for first user
	sessionID1, err := id.Generate("rsession")
	require.NoError(t, err)
	session1 := domain.NewBookReadingSession(sessionID1, user1ID, bookID)
	session1.UpdateProgress(1800000)
	require.NoError(t, ts.store.CreateReadingSession(context.Background(), session1))

	// Create second user
	user2ID, err := id.Generate("user")
	require.NoError(t, err)
	user2 := createTestUser(user2ID, "user2@test.com", "Test", "User2", domain.RoleMember)
	require.NoError(t, ts.store.CreateUser(context.Background(), user2))

	// Create session for second user
	sessionID2, err := id.Generate("rsession")
	require.NoError(t, err)
	session2 := domain.NewBookReadingSession(sessionID2, user2ID, bookID)
	session2.MarkCompleted(1.0, 3600000)
	require.NoError(t, ts.store.CreateReadingSession(context.Background(), session2))

	// Get book readers as first user
	resp := ts.api.Get("/api/v1/books/"+bookID+"/readers?limit=10",
		"Authorization: Bearer "+token1)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[BookReadersResponse]
	err = json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.Equal(t, 1, len(envelope.Data.YourSessions))
	assert.Equal(t, 1, len(envelope.Data.OtherReaders))
	assert.Equal(t, 2, envelope.Data.TotalReaders)
	assert.Equal(t, 1, envelope.Data.TotalCompletions)

	// Check other reader details
	otherReader := envelope.Data.OtherReaders[0]
	assert.Equal(t, user2ID, otherReader.UserID)
	assert.Equal(t, "Test User2", otherReader.DisplayName)
	assert.Equal(t, 1, otherReader.CompletionCount)
	assert.False(t, otherReader.IsCurrentlyReading) // Completed sessions are not "currently reading"
}

func TestGetUserReadingHistory_Success(t *testing.T) {
	ts := setupTestServerWithReadingSession(t)
	defer ts.cleanup()

	token, userID := createTestUserAndLoginWithID(t, ts)

	// Create two books
	book1ID, err := id.Generate("book")
	require.NoError(t, err)
	book1 := createTestBook(book1ID)
	book1.Title = "Book One"
	require.NoError(t, ts.store.CreateBook(context.Background(), book1))

	book2ID, err := id.Generate("book")
	require.NoError(t, err)
	book2 := createTestBook(book2ID)
	book2.Title = "Book Two"
	book2.TotalDuration = 7200000
	book2.AudioFiles[0].Duration = 7200000
	require.NoError(t, ts.store.CreateBook(context.Background(), book2))

	// Create reading sessions
	session1ID, err := id.Generate("rsession")
	require.NoError(t, err)
	session1 := domain.NewBookReadingSession(session1ID, userID, book1ID)
	session1.MarkCompleted(1.0, 3600000)
	require.NoError(t, ts.store.CreateReadingSession(context.Background(), session1))

	// Wait a bit to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	session2ID, err := id.Generate("rsession")
	require.NoError(t, err)
	session2 := domain.NewBookReadingSession(session2ID, userID, book2ID)
	session2.UpdateProgress(1800000)
	require.NoError(t, ts.store.CreateReadingSession(context.Background(), session2))

	// Get reading history
	resp := ts.api.Get("/api/v1/users/me/reading-sessions?limit=20",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[UserReadingHistoryResponse]
	err = json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.Equal(t, 2, envelope.Data.TotalSessions)
	assert.Equal(t, 1, envelope.Data.TotalCompleted)
	assert.Equal(t, 2, len(envelope.Data.Sessions))

	// Sessions should be ordered by most recent first
	assert.Equal(t, session2ID, envelope.Data.Sessions[0].ID)
	assert.Equal(t, "Book Two", envelope.Data.Sessions[0].BookTitle)
	assert.False(t, envelope.Data.Sessions[0].IsCompleted)

	assert.Equal(t, session1ID, envelope.Data.Sessions[1].ID)
	assert.Equal(t, "Book One", envelope.Data.Sessions[1].BookTitle)
	assert.True(t, envelope.Data.Sessions[1].IsCompleted)
}

func TestGetUserReadingHistory_Unauthorized(t *testing.T) {
	ts := setupTestServerWithReadingSession(t)
	defer ts.cleanup()

	// Try to get reading history without auth
	resp := ts.api.Get("/api/v1/users/me/reading-sessions")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestGetUserReadingHistory_EmptyHistory(t *testing.T) {
	ts := setupTestServerWithReadingSession(t)
	defer ts.cleanup()

	token := ts.createTestUserAndLogin(t)

	// Get reading history when user has no sessions
	resp := ts.api.Get("/api/v1/users/me/reading-sessions",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[UserReadingHistoryResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.Equal(t, 0, envelope.Data.TotalSessions)
	assert.Equal(t, 0, envelope.Data.TotalCompleted)
	assert.Equal(t, 0, len(envelope.Data.Sessions))
}

func TestGetUserReadingHistory_LimitParameter(t *testing.T) {
	ts := setupTestServerWithReadingSession(t)
	defer ts.cleanup()

	token, userID := createTestUserAndLoginWithID(t, ts)

	// Create multiple sessions
	for i := 0; i < 5; i++ {
		bookID, err := id.Generate("book")
		require.NoError(t, err)

		book := createTestBook(bookID)
		book.Title = "Book " + string(rune('A'+i))
		require.NoError(t, ts.store.CreateBook(context.Background(), book))

		sessionID, err := id.Generate("rsession")
		require.NoError(t, err)
		session := domain.NewBookReadingSession(sessionID, userID, bookID)
		session.UpdateProgress(1800000)
		require.NoError(t, ts.store.CreateReadingSession(context.Background(), session))

		time.Sleep(5 * time.Millisecond) // Ensure different timestamps
	}

	// Get reading history with limit of 3
	resp := ts.api.Get("/api/v1/users/me/reading-sessions?limit=3",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[UserReadingHistoryResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.Equal(t, 3, len(envelope.Data.Sessions))
	assert.Equal(t, 3, envelope.Data.TotalSessions) // TotalSessions is the count of returned sessions, not all sessions
}

// setupTestServerWithReadingSession creates a test server with ReadingSession service wired up.
func setupTestServerWithReadingSession(t *testing.T) *testServer {
	t.Helper()

	ts := setupTestServer(t)

	// Add ReadingSession service to the services
	// It's not in the default test setup, so we create it manually
	if ts.services.ReadingSession == nil {
		readingSessionService := createReadingSessionService(t, ts)
		ts.services.ReadingSession = readingSessionService
	}

	// Re-register social routes to include the new endpoints
	ts.registerSocialRoutes()

	return ts
}
