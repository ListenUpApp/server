package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestInbox creates a test inbox service with temp database.
func setupTestInbox(t *testing.T) (*InboxService, *store.Store, func()) {
	t.Helper()

	// Create temp directory for test database.
	tmpDir, err := os.MkdirTemp("", "listenup-inbox-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create logger.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create SSE manager (needed for events).
	sseManager := sse.NewManager(logger)

	// Create store.
	testStore, err := store.New(dbPath, nil, sseManager)
	require.NoError(t, err)
	require.NotNil(t, testStore)

	// Create inbox service (enricher is nil - we won't test enrichment).
	inboxService := NewInboxService(testStore, nil, sseManager, logger)

	// Return cleanup function.
	cleanup := func() {
		_ = testStore.Close()    //nolint:errcheck // Test cleanup
		_ = os.RemoveAll(tmpDir) //nolint:errcheck // Test cleanup
	}

	return inboxService, testStore, cleanup
}

// createTestLibraryInbox creates a default library for testing.
func createTestLibraryInbox(t *testing.T, ctx context.Context, testStore *store.Store) *domain.Library {
	t.Helper()

	library := &domain.Library{
		ID:        "library-test",
		Name:      "Test Library",
		OwnerID:   "owner-test",
		ScanPaths: []string{"/test/path"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := testStore.CreateLibrary(ctx, library)
	require.NoError(t, err)

	return library
}

// createTestInboxCollection creates the inbox collection for testing.
func createTestInboxCollection(t *testing.T, ctx context.Context, testStore *store.Store, libraryID string) *domain.Collection {
	t.Helper()

	inbox := &domain.Collection{
		ID:        "inbox-" + libraryID,
		LibraryID: libraryID,
		OwnerID:   "owner-test",
		Name:      "Inbox",
		IsInbox:   true,
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := testStore.CreateCollection(ctx, inbox)
	require.NoError(t, err)

	return inbox
}

// createTestCollectionInbox creates a regular collection for testing.
func createTestCollectionInbox(t *testing.T, ctx context.Context, testStore *store.Store, id, name, libraryID string) *domain.Collection {
	t.Helper()

	collection := &domain.Collection{
		ID:        id,
		LibraryID: libraryID,
		OwnerID:   "owner-test",
		Name:      name,
		IsInbox:   false,
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := testStore.CreateCollection(ctx, collection)
	require.NoError(t, err)

	return collection
}

// createInboxBook creates a test book with the given ID and adds it to inbox.
func createInboxBook(t *testing.T, ctx context.Context, testStore *store.Store, id, inboxID string) *domain.Book {
	t.Helper()

	book := createTestBook(id, time.Now())
	err := testStore.CreateBook(ctx, book)
	require.NoError(t, err)

	// Add to inbox.
	err = testStore.AdminAddBookToCollection(ctx, id, inboxID)
	require.NoError(t, err)

	return book
}

func TestInboxService_ListBooks_Empty(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library and inbox.
	library := createTestLibraryInbox(t, ctx, testStore)
	createTestInboxCollection(t, ctx, testStore, library.ID)

	// List books - should be empty.
	books, err := inboxService.ListBooks(ctx)
	require.NoError(t, err)
	assert.Empty(t, books)
}

func TestInboxService_ListBooks_WithBooks(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library and inbox.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)

	// Add books to inbox.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)
	createInboxBook(t, ctx, testStore, "book-002", inbox.ID)
	createInboxBook(t, ctx, testStore, "book-003", inbox.ID)

	// List books.
	books, err := inboxService.ListBooks(ctx)
	require.NoError(t, err)
	assert.Len(t, books, 3)

	// Verify book IDs.
	bookIDs := make(map[string]bool)
	for _, book := range books {
		bookIDs[book.ID] = true
	}
	assert.True(t, bookIDs["book-001"])
	assert.True(t, bookIDs["book-002"])
	assert.True(t, bookIDs["book-003"])
}

func TestInboxService_StageCollection(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library, inbox, and a target collection.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)
	collection := createTestCollectionInbox(t, ctx, testStore, "coll-001", "Sci-Fi", library.ID)

	// Add a book to inbox.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)

	// Stage the collection.
	err := inboxService.StageCollection(ctx, "book-001", collection.ID)
	require.NoError(t, err)

	// Verify book has staged collection.
	book, err := testStore.GetBookNoAccessCheck(ctx, "book-001")
	require.NoError(t, err)
	assert.Contains(t, book.StagedCollectionIDs, collection.ID)
}

func TestInboxService_StageCollection_Idempotent(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library, inbox, and a target collection.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)
	collection := createTestCollectionInbox(t, ctx, testStore, "coll-001", "Sci-Fi", library.ID)

	// Add a book to inbox.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)

	// Stage the collection twice.
	err := inboxService.StageCollection(ctx, "book-001", collection.ID)
	require.NoError(t, err)
	err = inboxService.StageCollection(ctx, "book-001", collection.ID)
	require.NoError(t, err)

	// Verify book has only one staged collection (not duplicated).
	book, err := testStore.GetBookNoAccessCheck(ctx, "book-001")
	require.NoError(t, err)
	assert.Len(t, book.StagedCollectionIDs, 1)
}

func TestInboxService_StageCollection_MultipleCollections(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library, inbox, and multiple collections.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)
	coll1 := createTestCollectionInbox(t, ctx, testStore, "coll-001", "Sci-Fi", library.ID)
	coll2 := createTestCollectionInbox(t, ctx, testStore, "coll-002", "Fantasy", library.ID)

	// Add a book to inbox.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)

	// Stage multiple collections.
	err := inboxService.StageCollection(ctx, "book-001", coll1.ID)
	require.NoError(t, err)
	err = inboxService.StageCollection(ctx, "book-001", coll2.ID)
	require.NoError(t, err)

	// Verify book has both staged collections.
	book, err := testStore.GetBookNoAccessCheck(ctx, "book-001")
	require.NoError(t, err)
	assert.Len(t, book.StagedCollectionIDs, 2)
	assert.Contains(t, book.StagedCollectionIDs, coll1.ID)
	assert.Contains(t, book.StagedCollectionIDs, coll2.ID)
}

func TestInboxService_StageCollection_BookNotInInbox(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library, inbox, and a target collection.
	library := createTestLibraryInbox(t, ctx, testStore)
	createTestInboxCollection(t, ctx, testStore, library.ID)
	collection := createTestCollectionInbox(t, ctx, testStore, "coll-001", "Sci-Fi", library.ID)

	// Create a book NOT in inbox.
	book := createTestBook("book-001", time.Now())
	err := testStore.CreateBook(ctx, book)
	require.NoError(t, err)

	// Attempt to stage a collection should fail.
	err = inboxService.StageCollection(ctx, "book-001", collection.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in inbox")
}

func TestInboxService_UnstageCollection(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library, inbox, and collections.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)
	coll1 := createTestCollectionInbox(t, ctx, testStore, "coll-001", "Sci-Fi", library.ID)
	coll2 := createTestCollectionInbox(t, ctx, testStore, "coll-002", "Fantasy", library.ID)

	// Add a book to inbox with staged collections.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)
	err := inboxService.StageCollection(ctx, "book-001", coll1.ID)
	require.NoError(t, err)
	err = inboxService.StageCollection(ctx, "book-001", coll2.ID)
	require.NoError(t, err)

	// Unstage one collection.
	err = inboxService.UnstageCollection(ctx, "book-001", coll1.ID)
	require.NoError(t, err)

	// Verify only coll2 remains.
	book, err := testStore.GetBookNoAccessCheck(ctx, "book-001")
	require.NoError(t, err)
	assert.Len(t, book.StagedCollectionIDs, 1)
	assert.Contains(t, book.StagedCollectionIDs, coll2.ID)
	assert.NotContains(t, book.StagedCollectionIDs, coll1.ID)
}

func TestInboxService_UnstageCollection_NotStaged(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library, inbox, and a collection.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)
	collection := createTestCollectionInbox(t, ctx, testStore, "coll-001", "Sci-Fi", library.ID)

	// Add a book to inbox without staging.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)

	// Unstage should be a no-op (no error).
	err := inboxService.UnstageCollection(ctx, "book-001", collection.ID)
	require.NoError(t, err)

	// Verify book still has no staged collections.
	book, err := testStore.GetBookNoAccessCheck(ctx, "book-001")
	require.NoError(t, err)
	assert.Empty(t, book.StagedCollectionIDs)
}

func TestInboxService_ReleaseBooks_WithStagedCollections(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library, inbox, and a collection.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)
	collection := createTestCollectionInbox(t, ctx, testStore, "coll-001", "Sci-Fi", library.ID)

	// Add a book to inbox with staged collection.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)
	err := inboxService.StageCollection(ctx, "book-001", collection.ID)
	require.NoError(t, err)

	// Release the book.
	result, err := inboxService.ReleaseBooks(ctx, []string{"book-001"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Released)
	assert.Equal(t, 0, result.Public)
	assert.Equal(t, 1, result.ToCollections)

	// Verify book is no longer in inbox.
	inbox, err = testStore.GetInboxForLibrary(ctx, library.ID)
	require.NoError(t, err)
	assert.NotContains(t, inbox.BookIDs, "book-001")

	// Verify book is in the target collection.
	collection, err = testStore.AdminGetCollection(ctx, collection.ID)
	require.NoError(t, err)
	assert.Contains(t, collection.BookIDs, "book-001")

	// Verify staged collections are cleared.
	book, err := testStore.GetBookNoAccessCheck(ctx, "book-001")
	require.NoError(t, err)
	assert.Empty(t, book.StagedCollectionIDs)
}

func TestInboxService_ReleaseBooks_PublicBook(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library and inbox.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)

	// Add a book to inbox without staging any collections.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)

	// Release the book.
	result, err := inboxService.ReleaseBooks(ctx, []string{"book-001"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Released)
	assert.Equal(t, 1, result.Public) // No collections = public
	assert.Equal(t, 0, result.ToCollections)

	// Verify book is no longer in inbox.
	inbox, err = testStore.GetInboxForLibrary(ctx, library.ID)
	require.NoError(t, err)
	assert.NotContains(t, inbox.BookIDs, "book-001")
}

func TestInboxService_ReleaseBooks_Multiple(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library, inbox, and collections.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)
	coll1 := createTestCollectionInbox(t, ctx, testStore, "coll-001", "Sci-Fi", library.ID)
	coll2 := createTestCollectionInbox(t, ctx, testStore, "coll-002", "Fantasy", library.ID)

	// Add books to inbox.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID) // Will go to coll1
	createInboxBook(t, ctx, testStore, "book-002", inbox.ID) // Will go to coll2
	createInboxBook(t, ctx, testStore, "book-003", inbox.ID) // Will be public

	// Stage collections.
	err := inboxService.StageCollection(ctx, "book-001", coll1.ID)
	require.NoError(t, err)
	err = inboxService.StageCollection(ctx, "book-002", coll2.ID)
	require.NoError(t, err)
	// book-003 has no staged collections

	// Release all books.
	result, err := inboxService.ReleaseBooks(ctx, []string{"book-001", "book-002", "book-003"})
	require.NoError(t, err)
	assert.Equal(t, 3, result.Released)
	assert.Equal(t, 1, result.Public)        // Only book-003
	assert.Equal(t, 2, result.ToCollections) // book-001 and book-002

	// Verify inbox is empty.
	inbox, err = testStore.GetInboxForLibrary(ctx, library.ID)
	require.NoError(t, err)
	assert.Empty(t, inbox.BookIDs)

	// Verify collections have their books.
	coll1, err = testStore.AdminGetCollection(ctx, coll1.ID)
	require.NoError(t, err)
	assert.Contains(t, coll1.BookIDs, "book-001")

	coll2, err = testStore.AdminGetCollection(ctx, coll2.ID)
	require.NoError(t, err)
	assert.Contains(t, coll2.BookIDs, "book-002")
}

func TestInboxService_GetInboxCount(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	ctx := context.Background()

	// Setup library and inbox.
	library := createTestLibraryInbox(t, ctx, testStore)
	inbox := createTestInboxCollection(t, ctx, testStore, library.ID)

	// Initially empty.
	count, err := inboxService.GetInboxCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Add books.
	createInboxBook(t, ctx, testStore, "book-001", inbox.ID)
	createInboxBook(t, ctx, testStore, "book-002", inbox.ID)

	// Count should be 2.
	count, err = inboxService.GetInboxCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestInboxService_ContextCancellation(t *testing.T) {
	inboxService, testStore, cleanup := setupTestInbox(t)
	defer cleanup()

	// Create a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// All operations should return context error.
	_, err := inboxService.ListBooks(ctx)
	assert.Error(t, err)

	library := createTestLibraryInbox(t, context.Background(), testStore)
	inbox := createTestInboxCollection(t, context.Background(), testStore, library.ID)
	createInboxBook(t, context.Background(), testStore, "book-001", inbox.ID)

	_, err = inboxService.ReleaseBooks(ctx, []string{"book-001"})
	assert.Error(t, err)

	err = inboxService.StageCollection(ctx, "book-001", "coll-001")
	assert.Error(t, err)

	err = inboxService.UnstageCollection(ctx, "book-001", "coll-001")
	assert.Error(t, err)
}
