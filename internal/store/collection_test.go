package store

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test Library Operations.

func TestCreateLibrary(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Verify library was created.
	retrieved, err := store.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, lib.ID, retrieved.ID)
	assert.Equal(t, lib.Name, retrieved.Name)
	assert.Equal(t, lib.ScanPaths, retrieved.ScanPaths)
}

func TestCreateLibrary_Duplicate(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Create first time.
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Try to create again.
	err = store.CreateLibrary(ctx, lib)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateLibrary)
}

func TestGetLibrary_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetLibrary(ctx, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLibraryNotFound)
}

func TestUpdateLibrary(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Update the library.
	lib.Name = "Updated Library"
	lib.ScanPaths = append(lib.ScanPaths, "/another/path")
	err = store.UpdateLibrary(ctx, lib)
	require.NoError(t, err)

	// Verify update.
	retrieved, err := store.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Library", retrieved.Name)
	assert.Len(t, retrieved.ScanPaths, 2)
}

func TestUpdateLibrary_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	lib := &domain.Library{
		ID:        "nonexistent",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.UpdateLibrary(ctx, lib)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrLibraryNotFound)
}

func TestDeleteLibrary(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Create a collection in the library.
	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Delete library (should cascade delete collections).
	err = store.DeleteLibrary(ctx, lib.ID)
	require.NoError(t, err)

	// Verify library is gone.
	_, err = store.GetLibrary(ctx, lib.ID)
	assert.ErrorIs(t, err, ErrLibraryNotFound)

	// Verify collection is gone.
	_, err = store.GetCollection(ctx, coll.ID, testUserID)
	assert.ErrorIs(t, err, ErrCollectionNotFound)
}

func TestListLibraries(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Empty list.
	libraries, err := store.ListLibraries(ctx)
	require.NoError(t, err)
	assert.Empty(t, libraries)

	// Create libraries.
	lib1 := &domain.Library{
		ID:        "lib-001",
		Name:      "Library 1",
		ScanPaths: []string{"/path/1"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	lib2 := &domain.Library{
		ID:        "lib-002",
		Name:      "Library 2",
		ScanPaths: []string{"/path/2"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.CreateLibrary(ctx, lib1)
	require.NoError(t, err)
	err = store.CreateLibrary(ctx, lib2)
	require.NoError(t, err)

	// List all.
	libraries, err = store.ListLibraries(ctx)
	require.NoError(t, err)
	assert.Len(t, libraries, 2)
}

func TestGetDefaultLibrary(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// No libraries exist.
	_, err := store.GetDefaultLibrary(ctx)
	assert.ErrorIs(t, err, ErrLibraryNotFound)

	// Create a library.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Default Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Get default (returns first library).
	defaultLib, err := store.GetDefaultLibrary(ctx)
	require.NoError(t, err)
	assert.Equal(t, lib.ID, defaultLib.ID)
}

// Test Collection Operations.

func TestCreateCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library first.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Create collection.
	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Verify collection was created.
	retrieved, err := store.GetCollection(ctx, coll.ID, testUserID)
	require.NoError(t, err)
	assert.Equal(t, coll.ID, retrieved.ID)
	assert.Equal(t, coll.Name, retrieved.Name)
	assert.Equal(t, coll.LibraryID, retrieved.LibraryID)
}

func TestCreateCollection_Duplicate(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Create collection.
	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Try to create again.
	err = store.CreateCollection(ctx, coll)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateCollection)
}

func TestGetCollection_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetCollection(ctx, "nonexistent", testUserID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCollectionNotFound)
}

func TestUpdateCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library and collection.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Update collection.
	coll.Name = "Updated Collection"
	coll.BookIDs = []string{"book-001", "book-002"}
	err = store.UpdateCollection(ctx, coll, testUserID)
	require.NoError(t, err)

	// Verify update.
	retrieved, err := store.GetCollection(ctx, coll.ID, testUserID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Collection", retrieved.Name)
	assert.Len(t, retrieved.BookIDs, 2)
}

func TestUpdateCollection_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	coll := &domain.Collection{
		ID:        "nonexistent",
		LibraryID: "lib-001",
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.UpdateCollection(ctx, coll, testUserID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrCollectionNotFound)
}

func TestDeleteCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library and collection.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Delete collection.
	err = store.DeleteCollection(ctx, coll.ID, testUserID)
	require.NoError(t, err)

	// Verify collection is gone.
	_, err = store.GetCollection(ctx, coll.ID, testUserID)
	assert.ErrorIs(t, err, ErrCollectionNotFound)
}

func TestDeleteCollection_SystemCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Create system collection (inbox).
	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Inbox",
		IsInbox:   true, // Mark as system collection
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Try to delete system collection.
	err = store.DeleteCollection(ctx, coll.ID, testUserID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete system collection")
}

func TestListCollectionsByLibrary(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Empty list.
	collections, err := store.ListCollectionsByLibrary(ctx, lib.ID, testUserID)
	require.NoError(t, err)
	assert.Empty(t, collections)

	// Create collections.
	coll1 := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Collection 1",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	coll2 := &domain.Collection{
		ID:        "coll-002",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Collection 2",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.CreateCollection(ctx, coll1)
	require.NoError(t, err)
	err = store.CreateCollection(ctx, coll2)
	require.NoError(t, err)

	// List collections.
	collections, err = store.ListCollectionsByLibrary(ctx, lib.ID, testUserID)
	require.NoError(t, err)
	assert.Len(t, collections, 2)
}

// Test Book-Collection Operations.

func TestAddBookToCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library and collection.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Add book to collection.
	err = store.AddBookToCollection(ctx, "book-001", coll.ID, testUserID)
	require.NoError(t, err)

	// Verify book was added.
	retrieved, err := store.GetCollection(ctx, coll.ID, testUserID)
	require.NoError(t, err)
	assert.Len(t, retrieved.BookIDs, 1)
	assert.Contains(t, retrieved.BookIDs, "book-001")
}

func TestAddBookToCollection_Duplicate(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library and collection.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Add book twice.
	err = store.AddBookToCollection(ctx, "book-001", coll.ID, testUserID)
	require.NoError(t, err)
	err = store.AddBookToCollection(ctx, "book-001", coll.ID, testUserID)
	require.NoError(t, err) // Should not error

	// Verify book appears only once.
	retrieved, err := store.GetCollection(ctx, coll.ID, testUserID)
	require.NoError(t, err)
	assert.Len(t, retrieved.BookIDs, 1)
}

func TestRemoveBookFromCollection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create library and collection.
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{"book-001", "book-002", "book-003"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Remove book from collection.
	err = store.RemoveBookFromCollection(ctx, "book-002", coll.ID, testUserID)
	require.NoError(t, err)

	// Verify book was removed.
	retrieved, err := store.GetCollection(ctx, coll.ID, testUserID)
	require.NoError(t, err)
	assert.Len(t, retrieved.BookIDs, 2)
	assert.Contains(t, retrieved.BookIDs, "book-001")
	assert.Contains(t, retrieved.BookIDs, "book-003")
	assert.NotContains(t, retrieved.BookIDs, "book-002")
}

func TestGetCollectionsForBook(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create libraries.
	lib1 := &domain.Library{
		ID:        "lib-001",
		Name:      "Library 1",
		ScanPaths: []string{"/path/1"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	lib2 := &domain.Library{
		ID:        "lib-002",
		Name:      "Library 2",
		ScanPaths: []string{"/path/2"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib1)
	require.NoError(t, err)
	err = store.CreateLibrary(ctx, lib2)
	require.NoError(t, err)

	// Create collections with books.
	coll1 := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib1.ID,
		OwnerID:   testUserID,
		Name:      "Collection 1",
		BookIDs:   []string{"book-001", "book-002"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	coll2 := &domain.Collection{
		ID:        "coll-002",
		LibraryID: lib1.ID,
		OwnerID:   testUserID,
		Name:      "Collection 2",
		BookIDs:   []string{"book-002", "book-003"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	coll3 := &domain.Collection{
		ID:        "coll-003",
		LibraryID: lib2.ID,
		OwnerID:   testUserID,
		Name:      "Collection 3",
		BookIDs:   []string{"book-004"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.CreateCollection(ctx, coll1)
	require.NoError(t, err)
	err = store.CreateCollection(ctx, coll2)
	require.NoError(t, err)
	err = store.CreateCollection(ctx, coll3)
	require.NoError(t, err)

	// Get collections for book-002.
	collections, err := store.GetCollectionsForBook(ctx, "book-002")
	require.NoError(t, err)
	assert.Len(t, collections, 2)

	collIDs := []string{collections[0].ID, collections[1].ID}
	assert.Contains(t, collIDs, "coll-001")
	assert.Contains(t, collIDs, "coll-002")

	// Get collections for book-004.
	collections, err = store.GetCollectionsForBook(ctx, "book-004")
	require.NoError(t, err)
	assert.Len(t, collections, 1)
	assert.Equal(t, "coll-003", collections[0].ID)

	// Get collections for non-existent book.
	collections, err = store.GetCollectionsForBook(ctx, "book-999")
	require.NoError(t, err)
	assert.Empty(t, collections)
}

// Test Edge Cases.

func TestLibrary_Persistence(t *testing.T) {
	// Test that data persists across store reopens.
	tmpDir := t.TempDir()

	ctx := context.Background()

	// Create and populate store.
	store1, err := New(tmpDir+"/test.db", nil, NewNoopEmitter())
	require.NoError(t, err)

	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/path/to/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store1.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		OwnerID:   testUserID,
		Name:      "Test Collection",
		BookIDs:   []string{"book-001", "book-002"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store1.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Close store.
	err = store1.Close()
	require.NoError(t, err)

	// Reopen store.
	store2, err := New(tmpDir+"/test.db", nil, NewNoopEmitter())
	require.NoError(t, err)
	defer store2.Close() //nolint:errcheck // Test cleanup

	// Verify data persisted.
	retrievedLib, err := store2.GetLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, lib.ID, retrievedLib.ID)
	assert.Equal(t, lib.Name, retrievedLib.Name)

	retrievedColl, err := store2.GetCollection(ctx, coll.ID, testUserID)
	require.NoError(t, err)
	assert.Equal(t, coll.ID, retrievedColl.ID)
	assert.Len(t, retrievedColl.BookIDs, 2)
}

// Test EnsureLibrary (Bootstrap).

func TestEnsureLibrary_NewLibrary(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	result, err := store.EnsureLibrary(ctx, "/path/to/audiobooks", testUserID)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result structure.
	assert.True(t, result.IsNewLibrary)
	assert.NotNil(t, result.Library)
	assert.NotNil(t, result.InboxCollection)
	assert.NotNil(t, result.InboxCollection)

	// Verify library details.
	assert.NotEmpty(t, result.Library.ID)
	assert.Contains(t, result.Library.ID, "lib-")
	assert.Len(t, result.Library.ID, len("lib-")+21, "Should have NanoID format: prefix + 21 chars")
	assert.Equal(t, "My Library", result.Library.Name)
	assert.Len(t, result.Library.ScanPaths, 1)
	assert.Equal(t, "/path/to/audiobooks", result.Library.ScanPaths[0])
	assert.False(t, result.Library.CreatedAt.IsZero())
	assert.False(t, result.Library.UpdatedAt.IsZero())

	// Verify inbox collection.
	assert.NotEmpty(t, result.InboxCollection.ID)
	assert.Contains(t, result.InboxCollection.ID, "coll-")
	assert.Len(t, result.InboxCollection.ID, len("coll-")+21, "Should have NanoID format: prefix + 21 chars")
	assert.Equal(t, result.Library.ID, result.InboxCollection.LibraryID)
	assert.Equal(t, "Inbox", result.InboxCollection.Name)
	assert.True(t, result.InboxCollection.IsInbox, "Inbox collection should have IsInbox = true")
	assert.Empty(t, result.InboxCollection.BookIDs)
	assert.False(t, result.InboxCollection.CreatedAt.IsZero())
	assert.False(t, result.InboxCollection.UpdatedAt.IsZero())
}

func TestEnsureLibrary_ExistingLibrary(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// First call - creates library.
	result1, err := store.EnsureLibrary(ctx, "/path/to/audiobooks", testUserID)
	require.NoError(t, err)
	assert.True(t, result1.IsNewLibrary)

	originalID := result1.Library.ID
	originalDefaultID := result1.InboxCollection.ID
	originalInboxID := result1.InboxCollection.ID

	// Second call - returns existing library.
	result2, err := store.EnsureLibrary(ctx, "/path/to/audiobooks", testUserID)
	require.NoError(t, err)
	assert.False(t, result2.IsNewLibrary)

	// Should be the same library.
	assert.Equal(t, originalID, result2.Library.ID)
	assert.Equal(t, result1.Library.Name, result2.Library.Name)
	assert.Len(t, result2.Library.ScanPaths, 1)
	assert.Equal(t, "/path/to/audiobooks", result2.Library.ScanPaths[0])

	// Should be the same collections.
	assert.Equal(t, originalDefaultID, result2.InboxCollection.ID)
	assert.Equal(t, originalInboxID, result2.InboxCollection.ID)
}

func TestEnsureLibrary_AddsNewScanPath(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// First call.
	result1, err := store.EnsureLibrary(ctx, "/path/one", testUserID)
	require.NoError(t, err)
	assert.True(t, result1.IsNewLibrary)
	assert.Len(t, result1.Library.ScanPaths, 1)
	assert.Contains(t, result1.Library.ScanPaths, "/path/one")

	originalID := result1.Library.ID

	// Second call with new path.
	result2, err := store.EnsureLibrary(ctx, "/path/two", testUserID)
	require.NoError(t, err)
	assert.False(t, result2.IsNewLibrary)
	assert.Equal(t, originalID, result2.Library.ID)
	assert.Len(t, result2.Library.ScanPaths, 2)
	assert.Contains(t, result2.Library.ScanPaths, "/path/one")
	assert.Contains(t, result2.Library.ScanPaths, "/path/two")

	// Third call with another new path.
	result3, err := store.EnsureLibrary(ctx, "/path/three", testUserID)
	require.NoError(t, err)
	assert.False(t, result3.IsNewLibrary)
	assert.Equal(t, originalID, result3.Library.ID)
	assert.Len(t, result3.Library.ScanPaths, 3)
	assert.Contains(t, result3.Library.ScanPaths, "/path/one")
	assert.Contains(t, result3.Library.ScanPaths, "/path/two")
	assert.Contains(t, result3.Library.ScanPaths, "/path/three")
}

func TestEnsureLibrary_DoesNotDuplicatePaths(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	scanPath := "/test/audiobooks"

	// First call.
	result1, err := store.EnsureLibrary(ctx, scanPath, testUserID)
	require.NoError(t, err)
	assert.True(t, result1.IsNewLibrary)
	assert.Len(t, result1.Library.ScanPaths, 1)

	// Second call with same path should not duplicate.
	result2, err := store.EnsureLibrary(ctx, scanPath, testUserID)
	require.NoError(t, err)
	assert.False(t, result2.IsNewLibrary)
	assert.Equal(t, result1.Library.ID, result2.Library.ID)
	assert.Len(t, result2.Library.ScanPaths, 1)
	assert.Contains(t, result2.Library.ScanPaths, scanPath)

	// Third call with same path again.
	result3, err := store.EnsureLibrary(ctx, scanPath, testUserID)
	require.NoError(t, err)
	assert.False(t, result3.IsNewLibrary)
	assert.Equal(t, result1.Library.ID, result3.Library.ID)
	assert.Len(t, result3.Library.ScanPaths, 1)
}

func TestEnsureLibrary_MultiplePathsSequence(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	paths := []string{
		"/mnt/nas/audiobooks",
		"/external/more",
		"/home/user/audible",
	}

	var libraryID string

	// Add each path sequentially.
	for i, path := range paths {
		result, err := store.EnsureLibrary(ctx, path, testUserID)
		require.NoError(t, err)

		if i == 0 {
			// First call creates the library.
			assert.True(t, result.IsNewLibrary)
			libraryID = result.Library.ID
		} else {
			// Subsequent calls should use same library.
			assert.False(t, result.IsNewLibrary)
			assert.Equal(t, libraryID, result.Library.ID)
		}

		// Verify correct number of paths.
		assert.Len(t, result.Library.ScanPaths, i+1)

		// Verify all previous paths are present.
		for j := 0; j <= i; j++ {
			assert.Contains(t, result.Library.ScanPaths, paths[j])
		}
	}
}

func TestEnsureLibrary_Idempotent(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	scanPath := "/test/audiobooks"

	// Call multiple times.
	var results []*BootstrapResult

	for i := 0; i < 5; i++ {
		result, err := store.EnsureLibrary(ctx, scanPath, testUserID)
		require.NoError(t, err)
		results = append(results, result)
	}

	// First call should create, rest should return existing.
	assert.True(t, results[0].IsNewLibrary)
	for i := 1; i < len(results); i++ {
		assert.False(t, results[i].IsNewLibrary)
	}

	// All calls should return the same library and collections.
	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0].Library.ID, results[i].Library.ID)
		assert.Equal(t, results[0].InboxCollection.ID, results[i].InboxCollection.ID)
		assert.Equal(t, results[0].InboxCollection.ID, results[i].InboxCollection.ID)
	}

	// Should still have only one scan path.
	assert.Len(t, results[4].Library.ScanPaths, 1)
}
