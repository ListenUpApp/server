package store

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixtures for access control tests.
const (
	ownerUserID   = "owner-user-123"
	memberUserID  = "member-user-456"
	otherUserID   = "other-user-789"
	testLibraryID = "lib-test-001"
)

// setupLibraryWithAccessMode creates a library with the specified access mode.
func setupLibraryWithAccessMode(t *testing.T, s *Store, mode domain.AccessMode) *domain.Library {
	t.Helper()
	ctx := context.Background()

	lib := &domain.Library{
		ID:         testLibraryID,
		OwnerID:    ownerUserID,
		Name:       "Test Library",
		ScanPaths:  []string{"/test/audiobooks"},
		AccessMode: mode,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err := s.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	return lib
}

// setupCollection creates a collection for testing.
func setupCollection(t *testing.T, s *Store, id, ownerID string, bookIDs []string, isGlobalAccess bool) *domain.Collection {
	t.Helper()
	ctx := context.Background()

	coll := &domain.Collection{
		ID:             id,
		LibraryID:      testLibraryID,
		OwnerID:        ownerID,
		Name:           "Test Collection " + id,
		BookIDs:        bookIDs,
		IsGlobalAccess: isGlobalAccess,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := s.CreateCollection(ctx, coll)
	require.NoError(t, err)

	return coll
}

// setupShare creates a share between collection and user.
func setupShare(t *testing.T, s *Store, collID, userID string) *domain.CollectionShare {
	t.Helper()
	ctx := context.Background()

	shareID := "share-" + collID + "-" + userID
	share := &domain.CollectionShare{
		Syncable: domain.Syncable{
			ID:        shareID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		CollectionID:     collID,
		SharedWithUserID: userID,
		Permission:       domain.PermissionRead,
	}
	err := s.CollectionShares.Create(ctx, shareID, share)
	require.NoError(t, err)

	return share
}

// setupBook creates a book for testing.
func setupBook(t *testing.T, s *Store, id string) *domain.Book {
	t.Helper()
	ctx := context.Background()

	book := createTestBook(id)
	err := s.CreateBook(ctx, book)
	require.NoError(t, err)

	return book
}

// TestGetBooksForUser_OpenMode_UncollectedBooksArePublic verifies that in open mode,
// books not in any collection are visible to all users.
func TestGetBooksForUser_OpenMode_UncollectedBooksArePublic(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in open mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeOpen)

	// Create books - not added to any collection
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Member user should see all uncollected books
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 2, "uncollected books should be visible to all users in open mode")
}

// TestGetBooksForUser_OpenMode_BooksInCollectionRequireAccess verifies that in open mode,
// books in a collection are only visible to users with collection access.
func TestGetBooksForUser_OpenMode_BooksInCollectionRequireAccess(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in open mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeOpen)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")
	setupBook(t, store, "book-003")

	// Put book-001 in a collection owned by owner
	setupCollection(t, store, "coll-001", ownerUserID, []string{"book-001"}, false)

	// Owner should see all books (owns collection + uncollected)
	ownerBooks, err := store.GetBooksForUser(ctx, ownerUserID)
	require.NoError(t, err)
	assert.Len(t, ownerBooks, 3)

	// Member should only see uncollected books
	memberBooks, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, memberBooks, 2, "member should not see books in owner's private collection")

	// Verify member doesn't have book-001
	for _, book := range memberBooks {
		assert.NotEqual(t, "book-001", book.ID, "book-001 should be hidden from member")
	}
}

// TestGetBooksForUser_RestrictedMode_OnlyExplicitAccess verifies that in restricted mode,
// users only see books they've been explicitly granted access to.
func TestGetBooksForUser_RestrictedMode_OnlyExplicitAccess(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")
	setupBook(t, store, "book-003")

	// Member should see NOTHING - no collection access
	memberBooks, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, memberBooks, 0, "user with no collection access should see no books in restricted mode")
}

// TestGetBooksForUser_RestrictedMode_CollectionGrantsAccess verifies that in restricted mode,
// users can see books in collections they have access to.
func TestGetBooksForUser_RestrictedMode_CollectionGrantsAccess(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")
	setupBook(t, store, "book-003")

	// Create a collection with book-001 and book-002
	coll := setupCollection(t, store, "coll-001", ownerUserID, []string{"book-001", "book-002"}, false)

	// Share collection with member
	setupShare(t, store, coll.ID, memberUserID)

	// Member should see only books in shared collection
	memberBooks, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, memberBooks, 2, "member should see books from shared collection")

	// Verify correct books
	bookIDs := make(map[string]bool)
	for _, book := range memberBooks {
		bookIDs[book.ID] = true
	}
	assert.True(t, bookIDs["book-001"])
	assert.True(t, bookIDs["book-002"])
	assert.False(t, bookIDs["book-003"], "book-003 should not be visible")
}

// TestGetBooksForUser_GlobalAccessCollection_OpenMode verifies global access behavior in open mode.
func TestGetBooksForUser_GlobalAccessCollection_OpenMode(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in open mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeOpen)

	// Create books - put them all in collections to restrict access
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")
	setupBook(t, store, "book-003")

	// Put all books in a private collection
	setupCollection(t, store, "coll-private", ownerUserID, []string{"book-001", "book-002", "book-003"}, false)

	// Member should see nothing (books are in private collection)
	memberBooks, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, memberBooks, 0, "member should not see books in private collection")

	// Create a global access collection and share with member
	globalColl := setupCollection(t, store, "coll-global", ownerUserID, []string{}, true)
	setupShare(t, store, globalColl.ID, memberUserID)

	// Now member should see ALL books (global access bypasses restrictions)
	memberBooks, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, memberBooks, 3, "member with global access should see all books")
}

// TestGetBooksForUser_GlobalAccessCollection_RestrictedMode verifies global access in restricted mode.
func TestGetBooksForUser_GlobalAccessCollection_RestrictedMode(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books (not in any collection)
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")
	setupBook(t, store, "book-003")

	// Member should see nothing initially
	memberBooks, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, memberBooks, 0)

	// Create a global access collection and share with member
	globalColl := setupCollection(t, store, "coll-global", ownerUserID, []string{}, true)
	setupShare(t, store, globalColl.ID, memberUserID)

	// Now member should see ALL books
	memberBooks, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, memberBooks, 3, "member with global access should see all books in restricted mode")
}

// TestCanUserAccessBook_OpenMode tests book access checks in open mode.
func TestCanUserAccessBook_OpenMode(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in open mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeOpen)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Put book-001 in a private collection
	setupCollection(t, store, "coll-001", ownerUserID, []string{"book-001"}, false)

	// Test uncollected book - everyone can access
	canAccess, err := store.CanUserAccessBook(ctx, memberUserID, "book-002")
	require.NoError(t, err)
	assert.True(t, canAccess, "uncollected book should be accessible")

	// Test collected book - only owner can access
	canAccess, err = store.CanUserAccessBook(ctx, ownerUserID, "book-001")
	require.NoError(t, err)
	assert.True(t, canAccess, "owner should access their collection's book")

	canAccess, err = store.CanUserAccessBook(ctx, memberUserID, "book-001")
	require.NoError(t, err)
	assert.False(t, canAccess, "member should not access private collection's book")
}

// TestCanUserAccessBook_RestrictedMode tests book access checks in restricted mode.
func TestCanUserAccessBook_RestrictedMode(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// In restricted mode, uncollected books are NOT accessible
	canAccess, err := store.CanUserAccessBook(ctx, memberUserID, "book-001")
	require.NoError(t, err)
	assert.False(t, canAccess, "uncollected book should NOT be accessible in restricted mode")

	// Put book-001 in a collection and share
	coll := setupCollection(t, store, "coll-001", ownerUserID, []string{"book-001"}, false)
	setupShare(t, store, coll.ID, memberUserID)

	// Now member can access book-001
	canAccess, err = store.CanUserAccessBook(ctx, memberUserID, "book-001")
	require.NoError(t, err)
	assert.True(t, canAccess, "member should access book via shared collection")

	// book-002 still not accessible
	canAccess, err = store.CanUserAccessBook(ctx, memberUserID, "book-002")
	require.NoError(t, err)
	assert.False(t, canAccess, "book-002 should not be accessible without collection")
}

// TestCanUserAccessBook_GlobalAccess tests that global access grants book access.
func TestCanUserAccessBook_GlobalAccess(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Member can't access initially
	canAccess, err := store.CanUserAccessBook(ctx, memberUserID, "book-001")
	require.NoError(t, err)
	assert.False(t, canAccess)

	// Grant global access
	globalColl := setupCollection(t, store, "coll-global", ownerUserID, []string{}, true)
	setupShare(t, store, globalColl.ID, memberUserID)

	// Now member can access any book
	canAccess, err = store.CanUserAccessBook(ctx, memberUserID, "book-001")
	require.NoError(t, err)
	assert.True(t, canAccess, "global access should grant access to any book")

	canAccess, err = store.CanUserAccessBook(ctx, memberUserID, "book-002")
	require.NoError(t, err)
	assert.True(t, canAccess, "global access should grant access to any book")
}

// TestEnsureGlobalAccessCollection tests creating global access collection.
func TestEnsureGlobalAccessCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library
	setupLibraryWithAccessMode(t, store, domain.AccessModeOpen)

	// First call should create collection
	coll, err := store.EnsureGlobalAccessCollection(ctx, testLibraryID, ownerUserID)
	require.NoError(t, err)
	require.NotNil(t, coll)
	assert.True(t, coll.IsGlobalAccess)
	assert.Equal(t, "Full Library Access", coll.Name)
	assert.Equal(t, testLibraryID, coll.LibraryID)
	assert.Equal(t, ownerUserID, coll.OwnerID)

	firstID := coll.ID

	// Second call should return existing collection
	coll2, err := store.EnsureGlobalAccessCollection(ctx, testLibraryID, ownerUserID)
	require.NoError(t, err)
	require.NotNil(t, coll2)
	assert.Equal(t, firstID, coll2.ID, "should return same collection on second call")
}

// TestGetBooksForUser_ExcludesInboxBooks verifies inbox books are always hidden.
func TestGetBooksForUser_ExcludesInboxBooks(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library
	setupLibraryWithAccessMode(t, store, domain.AccessModeOpen)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Create inbox collection with book-001
	inbox := &domain.Collection{
		ID:        "inbox-001",
		LibraryID: testLibraryID,
		OwnerID:   ownerUserID,
		Name:      "Inbox",
		IsInbox:   true,
		BookIDs:   []string{"book-001"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateCollection(ctx, inbox)
	require.NoError(t, err)

	// Even owner should not see inbox books
	books, err := store.GetBooksForUser(ctx, ownerUserID)
	require.NoError(t, err)
	assert.Len(t, books, 1, "inbox books should be excluded")
	assert.Equal(t, "book-002", books[0].ID)
}
