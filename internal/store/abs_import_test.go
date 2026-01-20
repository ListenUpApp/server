package store

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ABSImport CRUD Tests ---

func TestCreateABSImport(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	imp := &domain.ABSImport{
		ID:            "import_123",
		Name:          "ABS Backup 2024",
		BackupPath:    "/path/to/backup.tar",
		Status:        domain.ABSImportStatusActive,
		TotalUsers:    5,
		TotalBooks:    100,
		TotalSessions: 500,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := store.CreateABSImport(ctx, imp)
	require.NoError(t, err)

	// Verify import can be retrieved
	retrieved, err := store.GetABSImport(ctx, imp.ID)
	require.NoError(t, err)
	assert.Equal(t, imp.ID, retrieved.ID)
	assert.Equal(t, imp.Name, retrieved.Name)
	assert.Equal(t, imp.BackupPath, retrieved.BackupPath)
	assert.Equal(t, imp.Status, retrieved.Status)
	assert.Equal(t, imp.TotalUsers, retrieved.TotalUsers)
	assert.Equal(t, imp.TotalBooks, retrieved.TotalBooks)
	assert.Equal(t, imp.TotalSessions, retrieved.TotalSessions)
}

func TestCreateABSImport_DuplicateID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	imp := &domain.ABSImport{
		ID:        "import_123",
		Name:      "First Import",
		Status:    domain.ABSImportStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateABSImport(ctx, imp)
	require.NoError(t, err)

	// Second creation with same ID should fail
	imp2 := &domain.ABSImport{
		ID:        "import_123",
		Name:      "Second Import",
		Status:    domain.ABSImportStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.CreateABSImport(ctx, imp2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrABSImportExists)
}

func TestGetABSImport_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetABSImport(ctx, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrABSImportNotFound)
}

func TestListABSImports(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple imports
	imports := []*domain.ABSImport{
		{ID: "import_1", Name: "Import 1", Status: domain.ABSImportStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "import_2", Name: "Import 2", Status: domain.ABSImportStatusCompleted, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "import_3", Name: "Import 3", Status: domain.ABSImportStatusArchived, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	for _, imp := range imports {
		err := store.CreateABSImport(ctx, imp)
		require.NoError(t, err)
	}

	// List all imports
	list, err := store.ListABSImports(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestListABSImports_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	list, err := store.ListABSImports(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestUpdateABSImport(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	imp := &domain.ABSImport{
		ID:        "import_123",
		Name:      "Original Name",
		Status:    domain.ABSImportStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateABSImport(ctx, imp)
	require.NoError(t, err)

	// Update import
	imp.Name = "Updated Name"
	imp.UsersMapped = 5
	imp.BooksMapped = 50
	err = store.UpdateABSImport(ctx, imp)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetABSImport(ctx, imp.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, 5, updated.UsersMapped)
	assert.Equal(t, 50, updated.BooksMapped)
}

func TestUpdateABSImport_StatusChange(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	imp := &domain.ABSImport{
		ID:        "import_123",
		Name:      "Test Import",
		Status:    domain.ABSImportStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateABSImport(ctx, imp)
	require.NoError(t, err)

	// Change status to completed
	imp.Status = domain.ABSImportStatusCompleted
	now := time.Now()
	imp.CompletedAt = &now
	err = store.UpdateABSImport(ctx, imp)
	require.NoError(t, err)

	// Verify status change
	updated, err := store.GetABSImport(ctx, imp.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ABSImportStatusCompleted, updated.Status)
	assert.NotNil(t, updated.CompletedAt)
}

func TestUpdateABSImport_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	imp := &domain.ABSImport{
		ID:        "nonexistent",
		Name:      "Test Import",
		Status:    domain.ABSImportStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.UpdateABSImport(ctx, imp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrABSImportNotFound)
}

func TestDeleteABSImport(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create import with users, books, and sessions
	imp := &domain.ABSImport{
		ID:        "import_123",
		Name:      "Test Import",
		Status:    domain.ABSImportStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateABSImport(ctx, imp)
	require.NoError(t, err)

	// Add associated data
	user := &domain.ABSImportUser{
		ImportID:    "import_123",
		ABSUserID:   "user_1",
		ABSUsername: "testuser",
	}
	err = store.CreateABSImportUser(ctx, user)
	require.NoError(t, err)

	book := &domain.ABSImportBook{
		ImportID:   "import_123",
		ABSMediaID: "book_1",
		ABSTitle:   "Test Book",
	}
	err = store.CreateABSImportBook(ctx, book)
	require.NoError(t, err)

	session := &domain.ABSImportSession{
		ImportID:     "import_123",
		ABSSessionID: "session_1",
		ABSUserID:    "user_1",
		ABSMediaID:   "book_1",
		Status:       domain.SessionStatusPendingUser,
	}
	err = store.CreateABSImportSession(ctx, session)
	require.NoError(t, err)

	// Delete import
	err = store.DeleteABSImport(ctx, "import_123")
	require.NoError(t, err)

	// Verify import is gone
	_, err = store.GetABSImport(ctx, "import_123")
	assert.ErrorIs(t, err, ErrABSImportNotFound)

	// Verify associated data is also gone
	_, err = store.GetABSImportUser(ctx, "import_123", "user_1")
	assert.ErrorIs(t, err, ErrABSImportUserNotFound)

	_, err = store.GetABSImportBook(ctx, "import_123", "book_1")
	assert.ErrorIs(t, err, ErrABSImportBookNotFound)

	_, err = store.GetABSImportSession(ctx, "import_123", "session_1")
	assert.ErrorIs(t, err, ErrABSImportSessionNotFound)
}

func TestDeleteABSImport_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	err := store.DeleteABSImport(ctx, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrABSImportNotFound)
}

// --- ABSImportUser Tests ---

func TestCreateABSImportUser(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.ABSImportUser{
		ImportID:      "import_123",
		ABSUserID:     "user_1",
		ABSUsername:   "testuser",
		ABSEmail:      "test@example.com",
		SessionCount:  10,
		TotalListenMs: 3600000,
		Confidence:    "high",
		MatchReason:   "email_match",
	}

	err := store.CreateABSImportUser(ctx, user)
	require.NoError(t, err)

	// Verify user can be retrieved
	retrieved, err := store.GetABSImportUser(ctx, "import_123", "user_1")
	require.NoError(t, err)
	assert.Equal(t, user.ABSUsername, retrieved.ABSUsername)
	assert.Equal(t, user.ABSEmail, retrieved.ABSEmail)
	assert.Equal(t, user.SessionCount, retrieved.SessionCount)
	assert.Equal(t, user.Confidence, retrieved.Confidence)
}

func TestGetABSImportUser_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetABSImportUser(ctx, "import_123", "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrABSImportUserNotFound)
}

func TestListABSImportUsers_AllFilter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create users - some mapped, some unmapped
	users := []*domain.ABSImportUser{
		{ImportID: "import_123", ABSUserID: "user_1", ABSUsername: "user1"},
		{ImportID: "import_123", ABSUserID: "user_2", ABSUsername: "user2", ListenUpID: strPtr("lu_user_1")},
		{ImportID: "import_123", ABSUserID: "user_3", ABSUsername: "user3"},
	}

	for _, u := range users {
		err := store.CreateABSImportUser(ctx, u)
		require.NoError(t, err)
	}

	// List all
	list, err := store.ListABSImportUsers(ctx, "import_123", domain.MappingFilterAll)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestListABSImportUsers_MappedFilter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	users := []*domain.ABSImportUser{
		{ImportID: "import_123", ABSUserID: "user_1", ABSUsername: "user1"},
		{ImportID: "import_123", ABSUserID: "user_2", ABSUsername: "user2", ListenUpID: strPtr("lu_user_1")},
		{ImportID: "import_123", ABSUserID: "user_3", ABSUsername: "user3", ListenUpID: strPtr("lu_user_2")},
	}

	for _, u := range users {
		if u.ListenUpID != nil {
			now := time.Now()
			u.MappedAt = &now
		}
		err := store.CreateABSImportUser(ctx, u)
		require.NoError(t, err)
	}

	// List only mapped
	list, err := store.ListABSImportUsers(ctx, "import_123", domain.MappingFilterMapped)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestListABSImportUsers_UnmappedFilter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	users := []*domain.ABSImportUser{
		{ImportID: "import_123", ABSUserID: "user_1", ABSUsername: "user1"},
		{ImportID: "import_123", ABSUserID: "user_2", ABSUsername: "user2", ListenUpID: strPtr("lu_user_1")},
		{ImportID: "import_123", ABSUserID: "user_3", ABSUsername: "user3"},
	}

	for _, u := range users {
		if u.ListenUpID != nil {
			now := time.Now()
			u.MappedAt = &now
		}
		err := store.CreateABSImportUser(ctx, u)
		require.NoError(t, err)
	}

	// List only unmapped
	list, err := store.ListABSImportUsers(ctx, "import_123", domain.MappingFilterUnmapped)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestUpdateABSImportUserMapping_MapUser(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.ABSImportUser{
		ImportID:    "import_123",
		ABSUserID:   "user_1",
		ABSUsername: "testuser",
	}
	err := store.CreateABSImportUser(ctx, user)
	require.NoError(t, err)

	// Initially unmapped
	assert.False(t, user.IsMapped())

	// Map user
	listenUpID := "lu_user_1"
	err = store.UpdateABSImportUserMapping(ctx, "import_123", "user_1", &listenUpID)
	require.NoError(t, err)

	// Verify mapping
	updated, err := store.GetABSImportUser(ctx, "import_123", "user_1")
	require.NoError(t, err)
	assert.True(t, updated.IsMapped())
	assert.Equal(t, listenUpID, *updated.ListenUpID)
	assert.NotNil(t, updated.MappedAt)
}

func TestUpdateABSImportUserMapping_ClearMapping(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	now := time.Now()
	listenUpID := "lu_user_1"
	user := &domain.ABSImportUser{
		ImportID:    "import_123",
		ABSUserID:   "user_1",
		ABSUsername: "testuser",
		ListenUpID:  &listenUpID,
		MappedAt:    &now,
	}
	err := store.CreateABSImportUser(ctx, user)
	require.NoError(t, err)

	// Clear mapping
	err = store.UpdateABSImportUserMapping(ctx, "import_123", "user_1", nil)
	require.NoError(t, err)

	// Verify mapping cleared
	updated, err := store.GetABSImportUser(ctx, "import_123", "user_1")
	require.NoError(t, err)
	assert.False(t, updated.IsMapped())
	assert.Nil(t, updated.ListenUpID)
	assert.Nil(t, updated.MappedAt)
}

func TestUpdateABSImportUserMapping_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	listenUpID := "lu_user_1"
	err := store.UpdateABSImportUserMapping(ctx, "import_123", "nonexistent", &listenUpID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrABSImportUserNotFound)
}

// --- ABSImportBook Tests ---

func TestCreateABSImportBook(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	book := &domain.ABSImportBook{
		ImportID:      "import_123",
		ABSMediaID:    "book_1",
		ABSTitle:      "Test Book",
		ABSAuthor:     "Test Author",
		ABSDurationMs: 36000000,
		SessionCount:  5,
		Confidence:    "high",
		MatchReason:   "title_author_match",
	}

	err := store.CreateABSImportBook(ctx, book)
	require.NoError(t, err)

	retrieved, err := store.GetABSImportBook(ctx, "import_123", "book_1")
	require.NoError(t, err)
	assert.Equal(t, book.ABSTitle, retrieved.ABSTitle)
	assert.Equal(t, book.ABSAuthor, retrieved.ABSAuthor)
	assert.Equal(t, book.ABSDurationMs, retrieved.ABSDurationMs)
}

func TestGetABSImportBook_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetABSImportBook(ctx, "import_123", "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrABSImportBookNotFound)
}

func TestListABSImportBooks_Filters(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	books := []*domain.ABSImportBook{
		{ImportID: "import_123", ABSMediaID: "book_1", ABSTitle: "Book 1"},
		{ImportID: "import_123", ABSMediaID: "book_2", ABSTitle: "Book 2", ListenUpID: strPtr("lu_book_1")},
		{ImportID: "import_123", ABSMediaID: "book_3", ABSTitle: "Book 3"},
	}

	for _, b := range books {
		if b.ListenUpID != nil {
			now := time.Now()
			b.MappedAt = &now
		}
		err := store.CreateABSImportBook(ctx, b)
		require.NoError(t, err)
	}

	// All
	all, err := store.ListABSImportBooks(ctx, "import_123", domain.MappingFilterAll)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Mapped
	mapped, err := store.ListABSImportBooks(ctx, "import_123", domain.MappingFilterMapped)
	require.NoError(t, err)
	assert.Len(t, mapped, 1)

	// Unmapped
	unmapped, err := store.ListABSImportBooks(ctx, "import_123", domain.MappingFilterUnmapped)
	require.NoError(t, err)
	assert.Len(t, unmapped, 2)
}

func TestUpdateABSImportBookMapping(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	book := &domain.ABSImportBook{
		ImportID:   "import_123",
		ABSMediaID: "book_1",
		ABSTitle:   "Test Book",
	}
	err := store.CreateABSImportBook(ctx, book)
	require.NoError(t, err)

	// Map book
	listenUpID := "lu_book_1"
	err = store.UpdateABSImportBookMapping(ctx, "import_123", "book_1", &listenUpID)
	require.NoError(t, err)

	// Verify mapping
	updated, err := store.GetABSImportBook(ctx, "import_123", "book_1")
	require.NoError(t, err)
	assert.True(t, updated.IsMapped())
	assert.Equal(t, listenUpID, *updated.ListenUpID)
}

// --- ABSImportSession Tests ---

func TestCreateABSImportSession(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.ABSImportSession{
		ImportID:      "import_123",
		ABSSessionID:  "session_1",
		ABSUserID:     "user_1",
		ABSMediaID:    "book_1",
		StartTime:     time.Now().Add(-time.Hour),
		Duration:      3600,
		StartPosition: 0,
		EndPosition:   3600,
		Status:        domain.SessionStatusPendingUser,
	}

	err := store.CreateABSImportSession(ctx, session)
	require.NoError(t, err)

	retrieved, err := store.GetABSImportSession(ctx, "import_123", "session_1")
	require.NoError(t, err)
	assert.Equal(t, session.ABSUserID, retrieved.ABSUserID)
	assert.Equal(t, session.ABSMediaID, retrieved.ABSMediaID)
	assert.Equal(t, session.Status, retrieved.Status)
}

func TestGetABSImportSession_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetABSImportSession(ctx, "import_123", "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrABSImportSessionNotFound)
}

func TestListABSImportSessions_StatusFilters(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	sessions := []*domain.ABSImportSession{
		{ImportID: "import_123", ABSSessionID: "s1", ABSUserID: "u1", ABSMediaID: "b1", Status: domain.SessionStatusPendingUser},
		{ImportID: "import_123", ABSSessionID: "s2", ABSUserID: "u1", ABSMediaID: "b2", Status: domain.SessionStatusPendingBook},
		{ImportID: "import_123", ABSSessionID: "s3", ABSUserID: "u2", ABSMediaID: "b1", Status: domain.SessionStatusReady},
		{ImportID: "import_123", ABSSessionID: "s4", ABSUserID: "u2", ABSMediaID: "b2", Status: domain.SessionStatusImported},
		{ImportID: "import_123", ABSSessionID: "s5", ABSUserID: "u3", ABSMediaID: "b3", Status: domain.SessionStatusSkipped},
	}

	for _, s := range sessions {
		err := store.CreateABSImportSession(ctx, s)
		require.NoError(t, err)
	}

	// All
	all, err := store.ListABSImportSessions(ctx, "import_123", domain.SessionFilterAll)
	require.NoError(t, err)
	assert.Len(t, all, 5)

	// Pending (user + book)
	pending, err := store.ListABSImportSessions(ctx, "import_123", domain.SessionFilterPending)
	require.NoError(t, err)
	assert.Len(t, pending, 2)

	// Ready
	ready, err := store.ListABSImportSessions(ctx, "import_123", domain.SessionFilterReady)
	require.NoError(t, err)
	assert.Len(t, ready, 1)

	// Imported
	imported, err := store.ListABSImportSessions(ctx, "import_123", domain.SessionFilterImported)
	require.NoError(t, err)
	assert.Len(t, imported, 1)

	// Skipped
	skipped, err := store.ListABSImportSessions(ctx, "import_123", domain.SessionFilterSkipped)
	require.NoError(t, err)
	assert.Len(t, skipped, 1)
}

func TestUpdateABSImportSessionStatus(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.ABSImportSession{
		ImportID:     "import_123",
		ABSSessionID: "session_1",
		ABSUserID:    "user_1",
		ABSMediaID:   "book_1",
		Status:       domain.SessionStatusPendingUser,
	}
	err := store.CreateABSImportSession(ctx, session)
	require.NoError(t, err)

	// Update to ready
	err = store.UpdateABSImportSessionStatus(ctx, "import_123", "session_1", domain.SessionStatusReady)
	require.NoError(t, err)

	updated, err := store.GetABSImportSession(ctx, "import_123", "session_1")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionStatusReady, updated.Status)
}

func TestUpdateABSImportSessionStatus_ToImported(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.ABSImportSession{
		ImportID:     "import_123",
		ABSSessionID: "session_1",
		ABSUserID:    "user_1",
		ABSMediaID:   "book_1",
		Status:       domain.SessionStatusReady,
	}
	err := store.CreateABSImportSession(ctx, session)
	require.NoError(t, err)

	// Mark as imported
	err = store.UpdateABSImportSessionStatus(ctx, "import_123", "session_1", domain.SessionStatusImported)
	require.NoError(t, err)

	updated, err := store.GetABSImportSession(ctx, "import_123", "session_1")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionStatusImported, updated.Status)
	assert.NotNil(t, updated.ImportedAt)
}

func TestSkipABSImportSession(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &domain.ABSImportSession{
		ImportID:     "import_123",
		ABSSessionID: "session_1",
		ABSUserID:    "user_1",
		ABSMediaID:   "book_1",
		Status:       domain.SessionStatusPendingUser,
	}
	err := store.CreateABSImportSession(ctx, session)
	require.NoError(t, err)

	// Skip session
	reason := "User requested skip"
	err = store.SkipABSImportSession(ctx, "import_123", "session_1", reason)
	require.NoError(t, err)

	updated, err := store.GetABSImportSession(ctx, "import_123", "session_1")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionStatusSkipped, updated.Status)
	assert.NotNil(t, updated.SkipReason)
	assert.Equal(t, reason, *updated.SkipReason)
}

// --- RecalculateSessionStatuses Tests ---

func TestRecalculateSessionStatuses_UserMapped(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create user and book
	user := &domain.ABSImportUser{ImportID: "import_123", ABSUserID: "user_1", ABSUsername: "user1"}
	err := store.CreateABSImportUser(ctx, user)
	require.NoError(t, err)

	book := &domain.ABSImportBook{ImportID: "import_123", ABSMediaID: "book_1", ABSTitle: "book1"}
	err = store.CreateABSImportBook(ctx, book)
	require.NoError(t, err)

	// Create session pending user
	session := &domain.ABSImportSession{
		ImportID:     "import_123",
		ABSSessionID: "session_1",
		ABSUserID:    "user_1",
		ABSMediaID:   "book_1",
		Status:       domain.SessionStatusPendingUser,
	}
	err = store.CreateABSImportSession(ctx, session)
	require.NoError(t, err)

	// Map user
	listenUpUserID := "lu_user_1"
	err = store.UpdateABSImportUserMapping(ctx, "import_123", "user_1", &listenUpUserID)
	require.NoError(t, err)

	// Recalculate - should move to pending_book
	err = store.RecalculateSessionStatuses(ctx, "import_123")
	require.NoError(t, err)

	updated, err := store.GetABSImportSession(ctx, "import_123", "session_1")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionStatusPendingBook, updated.Status)
}

func TestRecalculateSessionStatuses_BothMapped(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create mapped user
	now := time.Now()
	listenUpUserID := "lu_user_1"
	user := &domain.ABSImportUser{
		ImportID:    "import_123",
		ABSUserID:   "user_1",
		ABSUsername: "user1",
		ListenUpID:  &listenUpUserID,
		MappedAt:    &now,
	}
	err := store.CreateABSImportUser(ctx, user)
	require.NoError(t, err)

	// Create mapped book
	listenUpBookID := "lu_book_1"
	book := &domain.ABSImportBook{
		ImportID:   "import_123",
		ABSMediaID: "book_1",
		ABSTitle:   "book1",
		ListenUpID: &listenUpBookID,
		MappedAt:   &now,
	}
	err = store.CreateABSImportBook(ctx, book)
	require.NoError(t, err)

	// Create session pending
	session := &domain.ABSImportSession{
		ImportID:     "import_123",
		ABSSessionID: "session_1",
		ABSUserID:    "user_1",
		ABSMediaID:   "book_1",
		Status:       domain.SessionStatusPendingUser,
	}
	err = store.CreateABSImportSession(ctx, session)
	require.NoError(t, err)

	// Recalculate - should become ready
	err = store.RecalculateSessionStatuses(ctx, "import_123")
	require.NoError(t, err)

	updated, err := store.GetABSImportSession(ctx, "import_123", "session_1")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionStatusReady, updated.Status)
}

func TestRecalculateSessionStatuses_SkipsImported(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create user and book (unmapped)
	user := &domain.ABSImportUser{ImportID: "import_123", ABSUserID: "user_1", ABSUsername: "user1"}
	err := store.CreateABSImportUser(ctx, user)
	require.NoError(t, err)

	book := &domain.ABSImportBook{ImportID: "import_123", ABSMediaID: "book_1", ABSTitle: "book1"}
	err = store.CreateABSImportBook(ctx, book)
	require.NoError(t, err)

	// Create already imported session
	session := &domain.ABSImportSession{
		ImportID:     "import_123",
		ABSSessionID: "session_1",
		ABSUserID:    "user_1",
		ABSMediaID:   "book_1",
		Status:       domain.SessionStatusImported,
	}
	err = store.CreateABSImportSession(ctx, session)
	require.NoError(t, err)

	// Recalculate - should NOT change imported session
	err = store.RecalculateSessionStatuses(ctx, "import_123")
	require.NoError(t, err)

	updated, err := store.GetABSImportSession(ctx, "import_123", "session_1")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionStatusImported, updated.Status)
}

// --- GetABSImportStats Tests ---

func TestGetABSImportStats(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create mixed data
	now := time.Now()
	listenUpID := "lu_1"

	// 2 users: 1 mapped, 1 unmapped
	user1 := &domain.ABSImportUser{ImportID: "import_123", ABSUserID: "user_1", ABSUsername: "user1", ListenUpID: &listenUpID, MappedAt: &now}
	user2 := &domain.ABSImportUser{ImportID: "import_123", ABSUserID: "user_2", ABSUsername: "user2"}
	err := store.CreateABSImportUser(ctx, user1)
	require.NoError(t, err)
	err = store.CreateABSImportUser(ctx, user2)
	require.NoError(t, err)

	// 3 books: 2 mapped, 1 unmapped
	book1 := &domain.ABSImportBook{ImportID: "import_123", ABSMediaID: "book_1", ABSTitle: "b1", ListenUpID: &listenUpID, MappedAt: &now}
	book2 := &domain.ABSImportBook{ImportID: "import_123", ABSMediaID: "book_2", ABSTitle: "b2", ListenUpID: &listenUpID, MappedAt: &now}
	book3 := &domain.ABSImportBook{ImportID: "import_123", ABSMediaID: "book_3", ABSTitle: "b3"}
	for _, b := range []*domain.ABSImportBook{book1, book2, book3} {
		err := store.CreateABSImportBook(ctx, b)
		require.NoError(t, err)
	}

	// Sessions: 1 ready, 2 imported
	s1 := &domain.ABSImportSession{ImportID: "import_123", ABSSessionID: "s1", ABSUserID: "u1", ABSMediaID: "b1", Status: domain.SessionStatusReady}
	s2 := &domain.ABSImportSession{ImportID: "import_123", ABSSessionID: "s2", ABSUserID: "u1", ABSMediaID: "b1", Status: domain.SessionStatusImported}
	s3 := &domain.ABSImportSession{ImportID: "import_123", ABSSessionID: "s3", ABSUserID: "u1", ABSMediaID: "b1", Status: domain.SessionStatusImported}
	for _, s := range []*domain.ABSImportSession{s1, s2, s3} {
		err := store.CreateABSImportSession(ctx, s)
		require.NoError(t, err)
	}

	mapped, unmapped, ready, imported, err := store.GetABSImportStats(ctx, "import_123")
	require.NoError(t, err)
	assert.Equal(t, 3, mapped)   // 1 user + 2 books
	assert.Equal(t, 2, unmapped) // 1 user + 1 book
	assert.Equal(t, 1, ready)
	assert.Equal(t, 2, imported)
}

func TestFindABSImportProgressByListenUpBook(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create import
	imp := &domain.ABSImport{ID: "import_123", Status: domain.ABSImportStatusActive}
	require.NoError(t, store.CreateABSImport(ctx, imp))

	// Create a book mapping: absMediaID1 -> listenUpBookID1
	book := &domain.ABSImportBook{
		ImportID:   "import_123",
		ABSMediaID: "abs-media-id-1",
		ListenUpID: strPtr("listenup-book-1"),
	}
	require.NoError(t, store.CreateABSImportBook(ctx, book))

	// Create progress with different ABS media ID (simulates the ABS schema inconsistency)
	// Progress uses a different mediaItemId but still maps to the same ListenUp book
	progress := &domain.ABSImportProgress{
		ImportID:    "import_123",
		ABSUserID:   "user-1",
		ABSMediaID:  "abs-media-id-1", // Same as book mapping
		CurrentTime: 12345,
		Duration:    50000,
		IsFinished:  true,
	}
	require.NoError(t, store.CreateABSImportProgress(ctx, progress))

	// Test finding progress by ListenUp book ID (should work)
	found, err := store.FindABSImportProgressByListenUpBook(ctx, "import_123", "user-1", "listenup-book-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "abs-media-id-1", found.ABSMediaID)
	assert.True(t, found.IsFinished)

	// Test finding progress for non-existent book (should return nil)
	notFound, err := store.FindABSImportProgressByListenUpBook(ctx, "import_123", "user-1", "non-existent-book")
	require.NoError(t, err)
	assert.Nil(t, notFound)

	// Test finding progress for non-existent user (should return nil)
	notFound, err = store.FindABSImportProgressByListenUpBook(ctx, "import_123", "non-existent-user", "listenup-book-1")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestFindABSImportProgressByListenUpBook_DifferentABSMediaIDs(t *testing.T) {
	// This test simulates the real ABS schema issue where:
	// - playbackSessions.mediaItemId = "session-media-id-A"
	// - mediaProgresses.mediaItemId = "progress-media-id-B"
	// Both refer to the same logical book, but have different UUIDs
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	imp := &domain.ABSImport{ID: "import_123", Status: domain.ABSImportStatusActive}
	require.NoError(t, store.CreateABSImport(ctx, imp))

	// Book is mapped using the session's media ID (from sessions)
	book := &domain.ABSImportBook{
		ImportID:   "import_123",
		ABSMediaID: "session-media-id-A",
		ListenUpID: strPtr("listenup-book-1"),
	}
	require.NoError(t, store.CreateABSImportBook(ctx, book))

	// But progress is stored with a DIFFERENT ABS media ID (from mediaProgresses table)
	progress := &domain.ABSImportProgress{
		ImportID:    "import_123",
		ABSUserID:   "user-1",
		ABSMediaID:  "progress-media-id-B", // Different from book!
		CurrentTime: 12345,
		IsFinished:  true,
	}
	require.NoError(t, store.CreateABSImportProgress(ctx, progress))

	// Create a second book mapping for the progress's media ID pointing to same ListenUp book
	// (This simulates what happens when ABS has two entries for the same logical book)
	book2 := &domain.ABSImportBook{
		ImportID:   "import_123",
		ABSMediaID: "progress-media-id-B",
		ListenUpID: strPtr("listenup-book-1"),
	}
	require.NoError(t, store.CreateABSImportBook(ctx, book2))

	// Now finding by ListenUp book ID should work
	found, err := store.FindABSImportProgressByListenUpBook(ctx, "import_123", "user-1", "listenup-book-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "progress-media-id-B", found.ABSMediaID) // Found via the progress's media ID
	assert.True(t, found.IsFinished)
}

func TestFindABSImportProgressByListenUpBook_NoMatchWhenIDsDiffer(t *testing.T) {
	// This test verifies that when there's NO matching ABSMediaID between
	// progress and book mappings, no match is returned. Duration-based matching
	// (Strategy 3) was removed because it caused incorrect matches.
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	imp := &domain.ABSImport{ID: "import_123", Status: domain.ABSImportStatusActive}
	require.NoError(t, store.CreateABSImport(ctx, imp))

	// Book mapping exists with a specific ABS media ID
	book := &domain.ABSImportBook{
		ImportID:      "import_123",
		ABSMediaID:    "book-media-id-X",
		ListenUpID:    strPtr("listenup-book-1"),
		ABSDurationMs: 3600000, // 1 hour in milliseconds
	}
	require.NoError(t, store.CreateABSImportBook(ctx, book))

	// Progress has a COMPLETELY DIFFERENT ABS media ID that doesn't match any book
	progress := &domain.ABSImportProgress{
		ImportID:    "import_123",
		ABSUserID:   "user-1",
		ABSMediaID:  "completely-different-progress-id", // No book mapping exists for this!
		CurrentTime: 1800000,                            // 30 min
		Duration:    3650000,                            // ~1 hour (similar duration but IDs don't match)
		IsFinished:  true,
	}
	require.NoError(t, store.CreateABSImportProgress(ctx, progress))

	// Should NOT find the progress because IDs don't match (duration matching was removed)
	found, err := store.FindABSImportProgressByListenUpBook(ctx, "import_123", "user-1", "listenup-book-1")
	require.NoError(t, err)
	assert.Nil(t, found) // No match when ABSMediaIDs don't align
}

func TestFindABSImportProgressByListenUpBook_DurationMismatch(t *testing.T) {
	// Test that duration matching doesn't match books with very different durations
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	imp := &domain.ABSImport{ID: "import_123", Status: domain.ABSImportStatusActive}
	require.NoError(t, store.CreateABSImport(ctx, imp))

	// Book mapping with 1 hour duration
	book := &domain.ABSImportBook{
		ImportID:      "import_123",
		ABSMediaID:    "book-media-id-X",
		ListenUpID:    strPtr("listenup-book-1"),
		ABSDurationMs: 3600000, // 1 hour
	}
	require.NoError(t, store.CreateABSImportBook(ctx, book))

	// Progress with very different duration (10 hours - not within 5%)
	progress := &domain.ABSImportProgress{
		ImportID:    "import_123",
		ABSUserID:   "user-1",
		ABSMediaID:  "different-progress-id",
		CurrentTime: 1000000,
		Duration:    36000000, // 10 hours - way different!
		IsFinished:  true,
	}
	require.NoError(t, store.CreateABSImportProgress(ctx, progress))

	// Should NOT find a match because duration is too different
	found, err := store.FindABSImportProgressByListenUpBook(ctx, "import_123", "user-1", "listenup-book-1")
	require.NoError(t, err)
	assert.Nil(t, found) // No match expected
}

// --- Helper ---

func strPtr(s string) *string {
	return &s
}
