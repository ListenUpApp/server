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

// =============================================================================
// Multi-Collection Edge Cases - Security Critical
// =============================================================================

// TestGetBooksForUser_BookInMultipleCollections_AccessViaOne verifies that a user
// with access to ONE of multiple collections containing a book can see that book.
func TestGetBooksForUser_BookInMultipleCollections_AccessViaOne(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode (most restrictive)
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create a book
	setupBook(t, store, "shared-book")

	// Put the book in THREE collections owned by different users
	setupCollection(t, store, "coll-owner", ownerUserID, []string{"shared-book"}, false)
	setupCollection(t, store, "coll-other", otherUserID, []string{"shared-book"}, false)
	setupCollection(t, store, "coll-admin", "admin-user", []string{"shared-book"}, false)

	// Member has NO access initially
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 0, "member should not see book without any share")

	// Share ONE collection (coll-other) with member
	setupShare(t, store, "coll-other", memberUserID)

	// Now member should see the book
	books, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 1, "member should see book via one shared collection")
	assert.Equal(t, "shared-book", books[0].ID)
}

// TestGetBooksForUser_BookInMultipleCollections_NoAccess verifies that a user
// without access to ANY collection containing a book cannot see it.
func TestGetBooksForUser_BookInMultipleCollections_NoAccess(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "private-book")
	setupBook(t, store, "also-private")

	// Put books in multiple collections, none shared with member
	setupCollection(t, store, "coll-a", ownerUserID, []string{"private-book", "also-private"}, false)
	setupCollection(t, store, "coll-b", otherUserID, []string{"private-book"}, false)
	setupCollection(t, store, "coll-c", "admin-user", []string{"also-private"}, false)

	// Share these collections with OTHER users, but NOT member
	setupShare(t, store, "coll-a", "random-user-1")
	setupShare(t, store, "coll-b", "random-user-2")
	setupShare(t, store, "coll-c", "random-user-3")

	// Member should see NOTHING
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 0, "member should not see any books without collection access")
}

// TestGetBooksForUser_ShareRevoked verifies that revoking a share immediately
// removes access to books in that collection.
func TestGetBooksForUser_ShareRevoked(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Create collection with both books and share with member
	coll := setupCollection(t, store, "coll-001", ownerUserID, []string{"book-001", "book-002"}, false)
	share := setupShare(t, store, coll.ID, memberUserID)

	// Member can see books
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 2, "member should see books via share")

	// REVOKE the share
	err = store.CollectionShares.Delete(ctx, share.ID)
	require.NoError(t, err)

	// Member should NO LONGER see books
	books, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 0, "member should lose access immediately when share is revoked")
}

// TestGetBooksForUser_ShareRevokedPartialAccess verifies that revoking ONE share
// doesn't affect access from OTHER shares.
func TestGetBooksForUser_ShareRevokedPartialAccess(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")
	setupBook(t, store, "book-003")

	// Create two collections
	coll1 := setupCollection(t, store, "coll-001", ownerUserID, []string{"book-001", "book-002"}, false)
	coll2 := setupCollection(t, store, "coll-002", otherUserID, []string{"book-002", "book-003"}, false)

	// Share both with member
	share1 := setupShare(t, store, coll1.ID, memberUserID)
	setupShare(t, store, coll2.ID, memberUserID)

	// Member can see all 3 books
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 3, "member should see all books from both collections")

	// Revoke share1 (coll-001: book-001, book-002)
	err = store.CollectionShares.Delete(ctx, share1.ID)
	require.NoError(t, err)

	// Member should still see books from coll-002
	books, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 2, "member should still see books from remaining collection")

	// Verify correct books (book-002 and book-003)
	bookIDs := make(map[string]bool)
	for _, book := range books {
		bookIDs[book.ID] = true
	}
	assert.False(t, bookIDs["book-001"], "book-001 should no longer be accessible")
	assert.True(t, bookIDs["book-002"], "book-002 should still be accessible via coll-002")
	assert.True(t, bookIDs["book-003"], "book-003 should still be accessible via coll-002")
}

// TestCanUserAccessBook_AfterShareRevocation tests point-access check after revocation.
func TestCanUserAccessBook_AfterShareRevocation(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create book
	setupBook(t, store, "sensitive-book")

	// Create collection and share
	coll := setupCollection(t, store, "coll-001", ownerUserID, []string{"sensitive-book"}, false)
	share := setupShare(t, store, coll.ID, memberUserID)

	// Member can access
	canAccess, err := store.CanUserAccessBook(ctx, memberUserID, "sensitive-book")
	require.NoError(t, err)
	assert.True(t, canAccess, "member should have access via share")

	// Revoke
	err = store.CollectionShares.Delete(ctx, share.ID)
	require.NoError(t, err)

	// Member should NOT have access
	canAccess, err = store.CanUserAccessBook(ctx, memberUserID, "sensitive-book")
	require.NoError(t, err)
	assert.False(t, canAccess, "access should be revoked immediately")
}

// TestGetBooksForUser_CollectionDeleted verifies that deleting a collection
// removes access to books (unless accessible via other collections).
func TestGetBooksForUser_CollectionDeleted(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Create collection and share
	coll := setupCollection(t, store, "coll-001", ownerUserID, []string{"book-001", "book-002"}, false)
	setupShare(t, store, coll.ID, memberUserID)

	// Member can see books
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 2)

	// DELETE the collection
	err = store.DeleteCollection(ctx, coll.ID, ownerUserID)
	require.NoError(t, err)

	// Member should lose access
	books, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 0, "deleting collection should revoke access")
}

// TestGetBooksForUser_AccessModeTransition_OpenToRestricted tests access when
// library mode changes from Open to Restricted.
func TestGetBooksForUser_AccessModeTransition_OpenToRestricted(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in OPEN mode initially
	lib := setupLibraryWithAccessMode(t, store, domain.AccessModeOpen)

	// Create uncollected books
	setupBook(t, store, "public-book")
	setupBook(t, store, "another-public")

	// In open mode, member can see uncollected books
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 2, "open mode: uncollected books are visible")

	// Change to RESTRICTED mode
	lib.AccessMode = domain.AccessModeRestricted
	err = store.UpdateLibrary(ctx, lib)
	require.NoError(t, err)

	// Member should now see NOTHING
	books, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 0, "restricted mode: uncollected books are hidden")
}

// TestGetBooksForUser_AccessModeTransition_RestrictedToOpen tests access when
// library mode changes from Restricted to Open.
func TestGetBooksForUser_AccessModeTransition_RestrictedToOpen(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in RESTRICTED mode initially
	lib := setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create uncollected books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Create a private collection with one book (not shared)
	setupCollection(t, store, "private-coll", ownerUserID, []string{"book-001"}, false)

	// Member can't see anything in restricted mode
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 0)

	// Change to OPEN mode
	lib.AccessMode = domain.AccessModeOpen
	err = store.UpdateLibrary(ctx, lib)
	require.NoError(t, err)

	// Member should now see UNCOLLECTED books (book-002)
	// but NOT book-001 which is in a private collection
	books, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 1, "open mode: only uncollected books visible")
	assert.Equal(t, "book-002", books[0].ID)
}

// TestGetBooksForUser_BookRemovedFromCollection verifies that removing a book
// from a collection revokes access to that book (unless accessible elsewhere).
func TestGetBooksForUser_BookRemovedFromCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Create collection with both books and share
	coll := setupCollection(t, store, "coll-001", ownerUserID, []string{"book-001", "book-002"}, false)
	setupShare(t, store, coll.ID, memberUserID)

	// Member can see both books
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 2)

	// Remove book-001 from collection
	coll.BookIDs = []string{"book-002"}
	err = store.UpdateCollection(ctx, coll, ownerUserID)
	require.NoError(t, err)

	// Member should only see book-002 now
	books, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 1, "removed book should no longer be accessible")
	assert.Equal(t, "book-002", books[0].ID)
}

// TestCanUserAccessCollection_NonExistentCollection ensures queries for
// non-existent collections don't leak information.
func TestCanUserAccessCollection_NonExistentCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Query for a collection that doesn't exist
	canAccess, permission, isGlobal, err := store.CanUserAccessCollection(ctx, memberUserID, "nonexistent-collection")

	// Should NOT return an error (that would leak existence info)
	assert.NoError(t, err, "non-existent collection should not return error")
	assert.False(t, canAccess, "should not have access to non-existent collection")
	assert.Equal(t, domain.PermissionRead, permission)
	assert.False(t, isGlobal)
}

// TestGetBooksForUser_GlobalAccessRevoked verifies that revoking global access
// immediately restricts the user to their specific collection access.
func TestGetBooksForUser_GlobalAccessRevoked(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")
	setupBook(t, store, "book-003")

	// Create a regular collection with book-001
	coll := setupCollection(t, store, "regular-coll", ownerUserID, []string{"book-001"}, false)
	setupShare(t, store, coll.ID, memberUserID)

	// Create a global access collection and share
	globalColl := setupCollection(t, store, "global-coll", ownerUserID, []string{}, true)
	globalShare := setupShare(t, store, globalColl.ID, memberUserID)

	// Member can see ALL books via global access
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 3, "global access grants visibility to all books")

	// Revoke global access
	err = store.CollectionShares.Delete(ctx, globalShare.ID)
	require.NoError(t, err)

	// Member should now only see book-001 via regular collection
	books, err = store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 1, "after revoking global access, only specific collections apply")
	assert.Equal(t, "book-001", books[0].ID)
}

// TestGetBooksForUser_OwnerAlwaysSeesTheirCollectionBooks verifies that
// collection owners always have access to books in their collections.
func TestGetBooksForUser_OwnerAlwaysSeesTheirCollectionBooks(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library in restricted mode
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create books
	setupBook(t, store, "book-001")
	setupBook(t, store, "book-002")

	// Create collection as owner
	setupCollection(t, store, "owner-coll", ownerUserID, []string{"book-001", "book-002"}, false)

	// Owner should see their books (no explicit share needed)
	books, err := store.GetBooksForUser(ctx, ownerUserID)
	require.NoError(t, err)
	assert.Len(t, books, 2, "owner should see books in their own collection")
}

// TestCanUserAccessBook_InboxAlwaysBlocked verifies that inbox books
// are never accessible, even with global access.
func TestCanUserAccessBook_InboxAlwaysBlocked(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library
	setupLibraryWithAccessMode(t, store, domain.AccessModeOpen)

	// Create book and put in inbox
	setupBook(t, store, "inbox-book")

	inbox := &domain.Collection{
		ID:        "inbox-001",
		LibraryID: testLibraryID,
		OwnerID:   ownerUserID,
		Name:      "Inbox",
		IsInbox:   true,
		BookIDs:   []string{"inbox-book"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateCollection(ctx, inbox)
	require.NoError(t, err)

	// Grant global access to member
	globalColl := setupCollection(t, store, "global-coll", ownerUserID, []string{}, true)
	setupShare(t, store, globalColl.ID, memberUserID)

	// Even with global access, inbox books should be inaccessible
	canAccess, err := store.CanUserAccessBook(ctx, memberUserID, "inbox-book")
	require.NoError(t, err)
	assert.False(t, canAccess, "inbox books should never be accessible, even with global access")

	// Owner should also not be able to access inbox book via normal query
	canAccess, err = store.CanUserAccessBook(ctx, ownerUserID, "inbox-book")
	require.NoError(t, err)
	assert.False(t, canAccess, "inbox books blocked for everyone")
}

// TestGetBooksForUser_DuplicateBookDeduplication verifies that books in
// multiple accessible collections are only returned once.
func TestGetBooksForUser_DuplicateBookDeduplication(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library
	setupLibraryWithAccessMode(t, store, domain.AccessModeRestricted)

	// Create a single book
	setupBook(t, store, "shared-book")

	// Put the same book in multiple collections and share ALL of them
	coll1 := setupCollection(t, store, "coll-001", ownerUserID, []string{"shared-book"}, false)
	coll2 := setupCollection(t, store, "coll-002", ownerUserID, []string{"shared-book"}, false)
	coll3 := setupCollection(t, store, "coll-003", otherUserID, []string{"shared-book"}, false)

	setupShare(t, store, coll1.ID, memberUserID)
	setupShare(t, store, coll2.ID, memberUserID)
	setupShare(t, store, coll3.ID, memberUserID)

	// Member should see the book ONCE, not three times
	books, err := store.GetBooksForUser(ctx, memberUserID)
	require.NoError(t, err)
	assert.Len(t, books, 1, "book should appear only once despite being in multiple collections")
	assert.Equal(t, "shared-book", books[0].ID)
}
