package service

import (
	"context"
	"fmt"
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

// setupTestSync creates a test sync service with temp database
func setupTestSync(t *testing.T) (*SyncService, *store.Store, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "listenup-sync-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store
	testStore, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)
	require.NotNil(t, testStore)

	// Create sync service
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	syncService := NewSyncService(testStore, logger)

	// Return cleanup function
	cleanup := func() {
		testStore.Close()
		os.RemoveAll(tmpDir)
	}

	return syncService, testStore, cleanup
}

// createTestBook creates a test book with the given ID and updatedAt time
func createTestBook(id string, updatedAt time.Time) *domain.Book {
	return &domain.Book{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: updatedAt.Add(-1 * time.Hour),
			UpdatedAt: updatedAt,
		},
		Title:       "Test Book " + id,
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
				ModTime:  updatedAt.Unix(),
			},
		},
		TotalDuration: 300000,
		TotalSize:     1024000,
		ScannedAt:     updatedAt,
	}
}

// TestGetManifest_WithMultipleBooks tests GetManifest with multiple books
func TestGetManifest_WithMultipleBooks(t *testing.T) {
	syncService, testStore, cleanup := setupTestSync(t)
	defer cleanup()

	ctx := context.Background()

	// Create books with different UpdatedAt times
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	book1 := createTestBook("book-001", baseTime)
	book2 := createTestBook("book-002", baseTime.Add(1*time.Hour))
	book3 := createTestBook("book-003", baseTime.Add(2*time.Hour)) // Latest

	require.NoError(t, testStore.CreateBook(ctx, book1))
	require.NoError(t, testStore.CreateBook(ctx, book2))
	require.NoError(t, testStore.CreateBook(ctx, book3))

	// Get manifest
	manifest, err := syncService.GetManifest(ctx)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	// Verify manifest contents
	assert.Equal(t, 3, manifest.Counts.Books)
	assert.Len(t, manifest.BookIDs, 3)
	assert.Contains(t, manifest.BookIDs, "book-001")
	assert.Contains(t, manifest.BookIDs, "book-002")
	assert.Contains(t, manifest.BookIDs, "book-003")

	// Verify checkpoint is the latest book's UpdatedAt
	checkpoint, err := time.Parse(time.RFC3339, manifest.Checkpoint)
	require.NoError(t, err)
	assert.WithinDuration(t, book3.UpdatedAt, checkpoint, time.Second)

	// Verify LibraryVersion matches Checkpoint
	assert.Equal(t, manifest.LibraryVersion, manifest.Checkpoint)

	// Verify future counts are zero
	assert.Equal(t, 0, manifest.Counts.Authors)
	assert.Equal(t, 0, manifest.Counts.Series)
}

// TestGetManifest_EmptyLibrary tests GetManifest with no books
func TestGetManifest_EmptyLibrary(t *testing.T) {
	syncService, _, cleanup := setupTestSync(t)
	defer cleanup()

	ctx := context.Background()

	manifest, err := syncService.GetManifest(ctx)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	// Verify empty library returns current time as checkpoint
	assert.Equal(t, 0, manifest.Counts.Books)
	assert.Empty(t, manifest.BookIDs)

	// Verify checkpoint is current time (within reasonable range)
	checkpoint, err := time.Parse(time.RFC3339, manifest.Checkpoint)
	require.NoError(t, err)
	// Should be within a few seconds of now
	assert.WithinDuration(t, time.Now(), checkpoint, 5*time.Second)
}

// TestGetManifest_SingleBook tests GetManifest with a single book
func TestGetManifest_SingleBook(t *testing.T) {
	syncService, testStore, cleanup := setupTestSync(t)
	defer cleanup()

	ctx := context.Background()

	// Create a single book
	bookTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	book := createTestBook("book-solo", bookTime)
	require.NoError(t, testStore.CreateBook(ctx, book))

	manifest, err := syncService.GetManifest(ctx)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	// Verify single book manifest
	assert.Equal(t, 1, manifest.Counts.Books)
	assert.Len(t, manifest.BookIDs, 1)
	assert.Equal(t, "book-solo", manifest.BookIDs[0])

	// Verify checkpoint matches book's UpdatedAt
	checkpoint, err := time.Parse(time.RFC3339, manifest.Checkpoint)
	require.NoError(t, err)
	assert.WithinDuration(t, book.UpdatedAt, checkpoint, time.Second)
}

// TestGetManifest_CheckpointOrdering tests that checkpoint uses most recent book
func TestGetManifest_CheckpointOrdering(t *testing.T) {
	syncService, testStore, cleanup := setupTestSync(t)
	defer cleanup()

	ctx := context.Background()

	// Create books in non-chronological order
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	oldBook := createTestBook("old-book", baseTime)
	newestBook := createTestBook("newest-book", baseTime.Add(10*time.Hour))
	middleBook := createTestBook("middle-book", baseTime.Add(5*time.Hour))

	// Insert in random order
	require.NoError(t, testStore.CreateBook(ctx, middleBook))
	require.NoError(t, testStore.CreateBook(ctx, newestBook))
	require.NoError(t, testStore.CreateBook(ctx, oldBook))

	manifest, err := syncService.GetManifest(ctx)
	require.NoError(t, err)

	// Verify checkpoint is from the newest book, not insertion order
	checkpoint, err := time.Parse(time.RFC3339, manifest.Checkpoint)
	require.NoError(t, err)
	assert.WithinDuration(t, newestBook.UpdatedAt, checkpoint, time.Second)
}

// TestNewSyncService tests sync service construction
func TestNewSyncService(t *testing.T) {
	_, testStore, cleanup := setupTestSync(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	service := NewSyncService(testStore, logger)

	assert.NotNil(t, service)
	assert.Equal(t, testStore, service.store)
	assert.Equal(t, logger, service.logger)
}

// TestManifestResponse_Structure tests the manifest response structure
func TestManifestResponse_Structure(t *testing.T) {
	syncService, testStore, cleanup := setupTestSync(t)
	defer cleanup()

	ctx := context.Background()

	// Create a book
	now := time.Now()
	book := createTestBook("book-001", now)
	require.NoError(t, testStore.CreateBook(ctx, book))

	manifest, err := syncService.GetManifest(ctx)
	require.NoError(t, err)

	// Verify all fields are populated
	assert.NotEmpty(t, manifest.LibraryVersion)
	assert.NotEmpty(t, manifest.Checkpoint)
	assert.Equal(t, manifest.LibraryVersion, manifest.Checkpoint)
	assert.NotNil(t, manifest.BookIDs)
	assert.GreaterOrEqual(t, manifest.Counts.Books, 0)
	assert.GreaterOrEqual(t, manifest.Counts.Authors, 0)
	assert.GreaterOrEqual(t, manifest.Counts.Series, 0)

	// Verify timestamps are valid RFC3339
	_, err = time.Parse(time.RFC3339, manifest.LibraryVersion)
	assert.NoError(t, err)
	_, err = time.Parse(time.RFC3339, manifest.Checkpoint)
	assert.NoError(t, err)
}

// TestGetBooksForSync_WithPagination tests paginated book fetch
func TestGetBooksForSync_WithPagination(t *testing.T) {
	syncService, testStore, cleanup := setupTestSync(t)
	defer cleanup()

	ctx := context.Background()

	// Create 5 books
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		book := createTestBook(fmt.Sprintf("book-%03d", i), baseTime.Add(time.Duration(i)*time.Hour))
		require.NoError(t, testStore.CreateBook(ctx, book))
	}

	// Get first page
	response, err := syncService.GetBooksForSync(ctx, store.PaginationParams{
		Limit:  2,
		Cursor: "",
	})
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify first page
	assert.Len(t, response.Books, 2)
	assert.True(t, response.HasMore)
	assert.NotEmpty(t, response.NextCursor)

	// Get second page
	response2, err := syncService.GetBooksForSync(ctx, store.PaginationParams{
		Limit:  2,
		Cursor: response.NextCursor,
	})
	require.NoError(t, err)
	assert.Len(t, response2.Books, 2)
	assert.True(t, response2.HasMore)

	// Get last page
	response3, err := syncService.GetBooksForSync(ctx, store.PaginationParams{
		Limit:  2,
		Cursor: response2.NextCursor,
	})
	require.NoError(t, err)
	assert.Len(t, response3.Books, 1)
	assert.False(t, response3.HasMore)
	assert.Empty(t, response3.NextCursor)
}

// TestGetBooksForSync_Empty tests with no books
func TestGetBooksForSync_Empty(t *testing.T) {
	syncService, _, cleanup := setupTestSync(t)
	defer cleanup()

	ctx := context.Background()

	response, err := syncService.GetBooksForSync(ctx, store.PaginationParams{
		Limit:  50,
		Cursor: "",
	})
	require.NoError(t, err)
	require.NotNil(t, response)

	assert.Empty(t, response.Books)
	assert.False(t, response.HasMore)
	assert.Empty(t, response.NextCursor)
}

// TestGetBooksForSync_SinglePage tests all books fit on one page
func TestGetBooksForSync_SinglePage(t *testing.T) {
	syncService, testStore, cleanup := setupTestSync(t)
	defer cleanup()

	ctx := context.Background()

	// Create 3 books
	baseTime := time.Now()
	for i := 1; i <= 3; i++ {
		book := createTestBook(fmt.Sprintf("book-%d", i), baseTime.Add(time.Duration(i)*time.Minute))
		require.NoError(t, testStore.CreateBook(ctx, book))
	}

	// Request with limit larger than book count
	response, err := syncService.GetBooksForSync(ctx, store.PaginationParams{
		Limit:  50,
		Cursor: "",
	})
	require.NoError(t, err)
	require.NotNil(t, response)

	assert.Len(t, response.Books, 3)
	assert.False(t, response.HasMore)
	assert.Empty(t, response.NextCursor)
}
