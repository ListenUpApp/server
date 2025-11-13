package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test book
func createTestBook(id string) *domain.Book {
	now := time.Now()
	return &domain.Book{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:       "Test Book",
		Subtitle:    "A Test Subtitle",
		Authors:     []string{"Test Author"},
		Narrators:   []string{"Test Narrator"},
		Description: "A test book description",
		Path:        "/test/path/" + id,
		AudioFiles: []domain.AudioFileInfo{
			{
				ID:       "af-1",
				Path:     "/test/path/" + id + "/file1.mp3",
				Filename: "file1.mp3",
				Size:     1024000,
				Duration: 300000,
				Format:   "mp3",
				Inode:    1001,
				ModTime:  time.Now().Unix(),
			},
			{
				ID:       "af-2",
				Path:     "/test/path/" + id + "/file2.mp3",
				Filename: "file2.mp3",
				Size:     2048000,
				Duration: 600000,
				Format:   "mp3",
				Inode:    1002,
				ModTime:  time.Now().Unix(),
			},
		},
		TotalDuration: 900000,
		TotalSize:     3072000,
		ScannedAt:     now,
	}
}

// TestCreateBook tests creating a new book
func TestCreateBook(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")

	err := store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Verify book was created
	retrieved, err := store.GetBook(ctx, book.ID)
	require.NoError(t, err)
	assert.Equal(t, book.ID, retrieved.ID)
	assert.Equal(t, book.Title, retrieved.Title)
	assert.Equal(t, book.Path, retrieved.Path)
	assert.Len(t, retrieved.AudioFiles, 2)
}

// TestCreateBook_Duplicate tests that creating a duplicate book returns an error
func TestCreateBook_Duplicate(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")

	// Create first time
	err := store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Try to create again - should fail
	err = store.CreateBook(ctx, book)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrBookExists)
}

// TestGetBook tests retrieving a book by ID
func TestGetBook(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")

	err := store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Get the book
	retrieved, err := store.GetBook(ctx, "book-001")
	require.NoError(t, err)
	assert.Equal(t, book.ID, retrieved.ID)
	assert.Equal(t, book.Title, retrieved.Title)
}

// TestGetBook_NotFound tests getting a nonexistent book
func TestGetBook_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetBook(ctx, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrBookNotFound)
}

// TestGetBookByInode tests retrieving a book by audio file inode
func TestGetBookByInode(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")

	err := store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Get book by inode of first audio file
	retrieved, err := store.GetBookByInode(ctx, 1001)
	require.NoError(t, err)
	assert.Equal(t, book.ID, retrieved.ID)
	assert.Equal(t, book.Title, retrieved.Title)

	// Get book by inode of second audio file
	retrieved, err = store.GetBookByInode(ctx, 1002)
	require.NoError(t, err)
	assert.Equal(t, book.ID, retrieved.ID)
}

// TestGetBookByInode_NotFound tests getting a book by nonexistent inode
func TestGetBookByInode_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetBookByInode(ctx, 9999)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrBookNotFound)
}

// TestUpdateBook tests updating an existing book
func TestUpdateBook(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")

	err := store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Update the book
	book.Title = "Updated Title"
	book.Description = "Updated Description"
	err = store.UpdateBook(ctx, book)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetBook(ctx, book.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", retrieved.Title)
	assert.Equal(t, "Updated Description", retrieved.Description)
}

// TestUpdateBook_PathChange tests updating a book's path and verifying index updates
func TestUpdateBook_PathChange(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")
	originalPath := book.Path

	err := store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Update the path
	book.Path = "/new/test/path/book-001"
	err = store.UpdateBook(ctx, book)
	require.NoError(t, err)

	// Verify path was updated
	retrieved, err := store.GetBook(ctx, book.ID)
	require.NoError(t, err)
	assert.Equal(t, "/new/test/path/book-001", retrieved.Path)
	assert.NotEqual(t, originalPath, retrieved.Path)
}

// TestUpdateBook_InodeChanges tests updating book with changed audio files
func TestUpdateBook_InodeChanges(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")

	err := store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Verify we can find by original inode
	_, err = store.GetBookByInode(ctx, 1001)
	require.NoError(t, err)

	// Update book - remove first audio file, add new one
	book.AudioFiles = []domain.AudioFileInfo{
		book.AudioFiles[1], // Keep second file
		{
			ID:       "af-3",
			Path:     "/test/path/book-001/file3.mp3",
			Filename: "file3.mp3",
			Size:     1500000,
			Duration: 450000,
			Format:   "mp3",
			Inode:    1003,
			ModTime:  time.Now().Unix(),
		},
	}

	err = store.UpdateBook(ctx, book)
	require.NoError(t, err)

	// Old inode (1001) should no longer find the book
	_, err = store.GetBookByInode(ctx, 1001)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrBookNotFound)

	// Kept inode (1002) should still work
	retrieved, err := store.GetBookByInode(ctx, 1002)
	require.NoError(t, err)
	assert.Equal(t, book.ID, retrieved.ID)

	// New inode (1003) should work
	retrieved, err = store.GetBookByInode(ctx, 1003)
	require.NoError(t, err)
	assert.Equal(t, book.ID, retrieved.ID)
}

// TestUpdateBook_NotFound tests updating a nonexistent book
func TestUpdateBook_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("nonexistent")

	err := store.UpdateBook(ctx, book)
	assert.Error(t, err)
}

// TestDeleteBook tests deleting a book
func TestDeleteBook(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")

	err := store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Delete the book
	err = store.DeleteBook(ctx, book.ID)
	require.NoError(t, err)

	// Verify book is gone
	_, err = store.GetBook(ctx, book.ID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrBookNotFound)

	// Verify inode index is gone
	_, err = store.GetBookByInode(ctx, 1001)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrBookNotFound)
}

// TestDeleteBook_NotFound tests deleting a nonexistent book
func TestDeleteBook_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	err := store.DeleteBook(ctx, "nonexistent")
	assert.Error(t, err)
}

// TestBookExists tests checking if a book exists
func TestBookExists(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	book := createTestBook("book-001")

	// Should not exist initially
	exists, err := store.BookExists(ctx, book.ID)
	require.NoError(t, err)
	assert.False(t, exists)

	// Create book
	err = store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Should now exist
	exists, err = store.BookExists(ctx, book.ID)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete book
	err = store.DeleteBook(ctx, book.ID)
	require.NoError(t, err)

	// Should no longer exist
	exists, err = store.BookExists(ctx, book.ID)
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestListBooks tests paginated book listing
func TestListBooks(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple books
	for i := 1; i <= 5; i++ {
		book := createTestBook(domain.GenerateAudioFileID(uint64(i)))
		err := store.CreateBook(ctx, book)
		require.NoError(t, err)
	}

	// List all books (first page)
	params := PaginationParams{
		Limit:  10,
		Cursor: "",
	}
	result, err := store.ListBooks(ctx, params)
	require.NoError(t, err)
	assert.Len(t, result.Items, 5)
	assert.False(t, result.HasMore)
	assert.Empty(t, result.NextCursor)
}

// TestListBooks_Pagination tests pagination with multiple pages
func TestListBooks_Pagination(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create 10 books
	for i := 1; i <= 10; i++ {
		book := createTestBook(domain.GenerateAudioFileID(uint64(i)))
		err := store.CreateBook(ctx, book)
		require.NoError(t, err)
	}

	// Get first page (limit 3)
	params := PaginationParams{
		Limit:  3,
		Cursor: "",
	}
	result, err := store.ListBooks(ctx, params)
	require.NoError(t, err)
	assert.Len(t, result.Items, 3)
	assert.True(t, result.HasMore)
	assert.NotEmpty(t, result.NextCursor)

	// Get second page
	params.Cursor = result.NextCursor
	result, err = store.ListBooks(ctx, params)
	require.NoError(t, err)
	assert.Len(t, result.Items, 3)
	assert.True(t, result.HasMore)

	// Get third page
	params.Cursor = result.NextCursor
	result, err = store.ListBooks(ctx, params)
	require.NoError(t, err)
	assert.Len(t, result.Items, 3)
	assert.True(t, result.HasMore)

	// Get fourth page (last page)
	params.Cursor = result.NextCursor
	result, err = store.ListBooks(ctx, params)
	require.NoError(t, err)
	assert.Len(t, result.Items, 1)
	assert.False(t, result.HasMore)
	assert.Empty(t, result.NextCursor)
}

// TestListBooks_Empty tests listing when no books exist
func TestListBooks_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	params := PaginationParams{
		Limit:  10,
		Cursor: "",
	}
	result, err := store.ListBooks(ctx, params)
	require.NoError(t, err)
	assert.Empty(t, result.Items)
	assert.False(t, result.HasMore)
}

// TestListAllBooks tests listing all books without pagination
func TestListAllBooks(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple books
	for i := 1; i <= 5; i++ {
		book := createTestBook(domain.GenerateAudioFileID(uint64(i)))
		err := store.CreateBook(ctx, book)
		require.NoError(t, err)
	}

	// List all books
	books, err := store.ListAllBooks(ctx)
	require.NoError(t, err)
	assert.Len(t, books, 5)
}

// TestListAllBooks_Empty tests listing all books when none exist
func TestListAllBooks_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	books, err := store.ListAllBooks(ctx)
	require.NoError(t, err)
	assert.Empty(t, books)
}

// TestGetBooksByCollectionPaginated tests paginated book listing by collection
func TestGetBooksByCollectionPaginated(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a library first
	lib := &domain.Library{
		ID:        "lib-001",
		Name:      "Test Library",
		ScanPaths: []string{"/test"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.CreateLibrary(ctx, lib)
	require.NoError(t, err)

	// Create a collection
	coll := &domain.Collection{
		ID:        "coll-001",
		LibraryID: lib.ID,
		Name:      "Test Collection",
		Type:      domain.CollectionTypeDefault,
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.CreateCollection(ctx, coll)
	require.NoError(t, err)

	// Create books and add to collection
	for i := 1; i <= 5; i++ {
		book := createTestBook(domain.GenerateAudioFileID(uint64(i)))
		err := store.CreateBook(ctx, book)
		require.NoError(t, err)

		err = store.AddBookToCollection(ctx, book.ID, coll.ID)
		require.NoError(t, err)
	}

	// Get first page
	params := PaginationParams{
		Limit:  3,
		Cursor: "",
	}
	result, err := store.GetBooksByCollectionPaginated(ctx, coll.ID, params)
	require.NoError(t, err)
	assert.Len(t, result.Items, 3)
	assert.True(t, result.HasMore)
	assert.Equal(t, 5, result.Total)

	// Get second page
	params.Cursor = result.NextCursor
	result, err = store.GetBooksByCollectionPaginated(ctx, coll.ID, params)
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
	assert.False(t, result.HasMore)
	assert.Equal(t, 5, result.Total)
}

// TestGetAllBookIDs tests getting all book IDs efficiently
func TestGetAllBookIDs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create test books
	book1 := createTestBook("book-001")
	book2 := createTestBook("book-002")
	book3 := createTestBook("book-003")

	require.NoError(t, store.CreateBook(ctx, book1))
	require.NoError(t, store.CreateBook(ctx, book2))
	require.NoError(t, store.CreateBook(ctx, book3))

	// Get all book IDs
	bookIDs, err := store.GetAllBookIDs(ctx)
	require.NoError(t, err)
	assert.Len(t, bookIDs, 3)

	// Verify IDs are present
	assert.Contains(t, bookIDs, "book-001")
	assert.Contains(t, bookIDs, "book-002")
	assert.Contains(t, bookIDs, "book-003")
}

// TestGetAllBookIDs_Empty tests getting IDs when no books exist
func TestGetAllBookIDs_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	bookIDs, err := store.GetAllBookIDs(ctx)
	require.NoError(t, err)
	assert.Empty(t, bookIDs)
}

// TestGetAllBookIDs_ManyBooks tests getting many book IDs
func TestGetAllBookIDs_ManyBooks(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create many books
	for i := 1; i <= 100; i++ {
		book := createTestBook(fmt.Sprintf("book-%03d", i))
		require.NoError(t, store.CreateBook(ctx, book))
	}

	// Get all book IDs
	bookIDs, err := store.GetAllBookIDs(ctx)
	require.NoError(t, err)
	assert.Len(t, bookIDs, 100)

	// Verify all IDs are unique
	idSet := make(map[string]bool)
	for _, id := range bookIDs {
		assert.False(t, idSet[id], "Duplicate ID found: %s", id)
		idSet[id] = true
	}
}
