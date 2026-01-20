package backup_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/listenupapp/listenup-server/internal/backup"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// testSetup creates a test store and backup/restore services.
func testSetup(t *testing.T) (*store.Store, *backup.BackupService, *backup.RestoreService, string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "backup_test")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	dataDir := filepath.Join(tmpDir, "data")
	backupDir := filepath.Join(tmpDir, "backups")

	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.MkdirAll(backupDir, 0o755))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	testStore, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	backupSvc := backup.NewBackupService(testStore, backupDir, dataDir, "test", logger)
	restoreSvc := backup.NewRestoreService(testStore, dataDir, logger)

	cleanup := func() {
		_ = testStore.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return testStore, backupSvc, restoreSvc, backupDir, cleanup
}

// createTestEntities creates a set of test entities in the store.
func createTestEntities(t *testing.T, s *store.Store) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()

	// Create instance (factory method - no params)
	_, err := s.CreateInstance(ctx)
	require.NoError(t, err)

	// Create root user
	user := &domain.User{
		Syncable: domain.Syncable{
			ID:        "user-root",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Email:        "admin@test.com",
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$test",
		IsRoot:       true,
		Role:         domain.RoleAdmin,
		Status:       domain.UserStatusActive,
		DisplayName:  "Test Admin",
		FirstName:    "Test",
		LastName:     "Admin",
	}
	require.NoError(t, s.Users.Create(ctx, user.ID, user))

	// Create library
	library := &domain.Library{
		ID:        "lib-test",
		OwnerID:   "user-root",
		Name:      "Test Library",
		ScanPaths: []string{"/audiobooks"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, s.CreateLibrary(ctx, library))

	// Create contributor
	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID:        "contrib-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Test Author",
	}
	require.NoError(t, s.CreateContributor(ctx, contributor))

	// Create series
	series := &domain.Series{
		Syncable: domain.Syncable{
			ID:        "series-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Test Series",
	}
	require.NoError(t, s.CreateSeries(ctx, series))

	// Create genre
	genre := &domain.Genre{
		Syncable: domain.Syncable{
			ID:        "genre-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name: "Fiction",
		Slug: "fiction",
		Path: "/fiction",
	}
	require.NoError(t, s.CreateGenre(ctx, genre))

	// Create book
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        "book-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:         "Test Book",
		Path:          "/audiobooks/test-book",
		TotalDuration: 3600000, // 1 hour in ms
		TotalSize:     100000000,
		Contributors: []domain.BookContributor{
			{ContributorID: "contrib-1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
		},
		Series: []domain.BookSeries{
			{SeriesID: "series-1", Sequence: "1"},
		},
		GenreIDs: []string{"genre-1"},
	}
	require.NoError(t, s.CreateBook(ctx, book))

	// Create collection
	collection := &domain.Collection{
		ID:        "coll-1",
		LibraryID: "lib-test",
		OwnerID:   "user-root",
		Name:      "Test Collection",
		CreatedAt: now,
		UpdatedAt: now,
		BookIDs:   []string{"book-1"},
	}
	require.NoError(t, s.CreateCollection(ctx, collection))
}

// TestBackupRestore_RoundTrip tests creating a backup and restoring to a fresh store.
func TestBackupRestore_RoundTrip(t *testing.T) {
	// Setup source store
	sourceStore, backupSvc, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// Create test entities
	createTestEntities(t, sourceStore)

	// Create backup
	result, err := backupSvc.Create(ctx, backup.BackupOptions{
		IncludeImages: false,
		IncludeEvents: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Path)
	require.Greater(t, result.Size, int64(0))
	require.NotEmpty(t, result.Checksum)

	// Verify counts
	assert.Equal(t, 1, result.Counts.Users)
	assert.Equal(t, 1, result.Counts.Libraries)
	assert.Equal(t, 1, result.Counts.Books)
	assert.Equal(t, 1, result.Counts.Contributors)
	assert.Equal(t, 1, result.Counts.Series)
	assert.Equal(t, 1, result.Counts.Genres)
	assert.Equal(t, 1, result.Counts.Collections)

	// Create destination store
	destDir, err := os.MkdirTemp("", "backup_dest")
	require.NoError(t, err)
	defer os.RemoveAll(destDir)

	destDbPath := filepath.Join(destDir, "dest.db")
	destDataDir := filepath.Join(destDir, "data")
	require.NoError(t, os.MkdirAll(destDataDir, 0o755))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	destStore, err := store.New(destDbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer destStore.Close()

	destRestoreSvc := backup.NewRestoreService(destStore, destDataDir, logger)

	// Restore to destination
	restoreResult, err := destRestoreSvc.Restore(ctx, result.Path, backup.RestoreOptions{
		Mode: backup.RestoreModeFull,
	})
	require.NoError(t, err)
	assert.Empty(t, restoreResult.Errors)

	// Verify restored entities
	user, err := destStore.Users.Get(ctx, "user-root")
	require.NoError(t, err)
	assert.Equal(t, "admin@test.com", user.Email)
	assert.Equal(t, "Test Admin", user.DisplayName)
	assert.True(t, user.IsRoot)

	book, err := destStore.GetBookNoAccessCheck(ctx, "book-1")
	require.NoError(t, err)
	assert.Equal(t, "Test Book", book.Title)
	assert.Equal(t, int64(3600000), book.TotalDuration)

	contrib, err := destStore.GetContributor(ctx, "contrib-1")
	require.NoError(t, err)
	assert.Equal(t, "Test Author", contrib.Name)

	series, err := destStore.GetSeries(ctx, "series-1")
	require.NoError(t, err)
	assert.Equal(t, "Test Series", series.Name)

	genre, err := destStore.GetGenre(ctx, "genre-1")
	require.NoError(t, err)
	assert.Equal(t, "Fiction", genre.Name)

	// List backups
	backups, err := backupSvc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, backups, 1)
	assert.Equal(t, result.Path, backups[0].Path)
}

// TestBackupValidate validates backup integrity checking.
func TestBackupValidate(t *testing.T) {
	sourceStore, backupSvc, restoreSvc, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	createTestEntities(t, sourceStore)

	// Create backup
	result, err := backupSvc.Create(ctx, backup.BackupOptions{IncludeEvents: true})
	require.NoError(t, err)

	// Validate backup
	validation, err := restoreSvc.Validate(ctx, result.Path)
	require.NoError(t, err)
	assert.True(t, validation.Valid)
	assert.Empty(t, validation.Errors)
	assert.NotNil(t, validation.Manifest)
	assert.Equal(t, "1.0", validation.Manifest.Version)
	assert.Equal(t, 1, validation.ExpectedCounts.Books)
}

// TestBackupValidate_InvalidPath tests validation with nonexistent file.
func TestBackupValidate_InvalidPath(t *testing.T) {
	_, _, restoreSvc, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	validation, err := restoreSvc.Validate(ctx, "/nonexistent/backup.zip")
	require.NoError(t, err) // Returns result with errors, not error
	assert.False(t, validation.Valid)
	assert.NotEmpty(t, validation.Errors)
}

// TestBackupDelete tests backup deletion.
func TestBackupDelete(t *testing.T) {
	sourceStore, backupSvc, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	createTestEntities(t, sourceStore)

	// Create backup
	result, err := backupSvc.Create(ctx, backup.BackupOptions{})
	require.NoError(t, err)

	// Get backup
	info, err := backupSvc.Get(ctx, extractID(result.Path))
	require.NoError(t, err)
	assert.Equal(t, result.Path, info.Path)

	// Delete backup
	err = backupSvc.Delete(ctx, extractID(result.Path))
	require.NoError(t, err)

	// Verify deleted
	_, err = backupSvc.Get(ctx, extractID(result.Path))
	assert.Equal(t, backup.ErrBackupNotFound, err)
}

// extractID extracts backup ID from path.
func extractID(path string) string {
	base := filepath.Base(path)
	// Remove .listenup.zip suffix
	if len(base) > 13 {
		return base[:len(base)-13]
	}
	return base
}

// TestRebuildProgress tests that playback progress can be rebuilt from listening events.
func TestRebuildProgress(t *testing.T) {
	sourceStore, backupSvc, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create instance
	_, err := sourceStore.CreateInstance(ctx)
	require.NoError(t, err)

	// Create user
	user := &domain.User{
		Syncable:    domain.Syncable{ID: "user-1", CreatedAt: now, UpdatedAt: now},
		Email:       "test@test.com",
		DisplayName: "Test User",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, sourceStore.Users.Create(ctx, user.ID, user))

	// Create library
	library := &domain.Library{
		ID:        "lib-1",
		OwnerID:   "user-1",
		Name:      "Test Library",
		ScanPaths: []string{"/audiobooks"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, sourceStore.CreateLibrary(ctx, library))

	// Create book (1 hour duration)
	book := &domain.Book{
		Syncable:      domain.Syncable{ID: "book-1", CreatedAt: now, UpdatedAt: now},
		Title:         "Test Book",
		Path:          "/audiobooks/test",
		TotalDuration: 3600000, // 1 hour in ms
		TotalSize:     100000000,
	}
	require.NoError(t, sourceStore.CreateBook(ctx, book))

	// Create listening events (simulating listening to half the book)
	event1 := domain.NewListeningEvent(
		"event-1", "user-1", "book-1",
		0, 900000, // 0 to 15 minutes
		now.Add(-2*time.Hour), now.Add(-105*time.Minute),
		1.0, "device-1", "Test Device",
	)
	require.NoError(t, sourceStore.CreateListeningEvent(ctx, event1))

	event2 := domain.NewListeningEvent(
		"event-2", "user-1", "book-1",
		900000, 1800000, // 15 to 30 minutes
		now.Add(-1*time.Hour), now.Add(-45*time.Minute),
		1.0, "device-1", "Test Device",
	)
	require.NoError(t, sourceStore.CreateListeningEvent(ctx, event2))

	// Create backup with events
	result, err := backupSvc.Create(ctx, backup.BackupOptions{
		IncludeEvents: true,
	})
	require.NoError(t, err)

	// Create destination store
	destDir, err := os.MkdirTemp("", "rebuild_test")
	require.NoError(t, err)
	defer os.RemoveAll(destDir)

	destStore, err := store.New(filepath.Join(destDir, "dest.db"), nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer destStore.Close()

	// Restore with full mode
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	restoreSvc := backup.NewRestoreService(destStore, destDir, logger)

	restoreResult, err := restoreSvc.Restore(ctx, result.Path, backup.RestoreOptions{
		Mode: backup.RestoreModeFull,
	})
	require.NoError(t, err)

	// Verify events were imported
	assert.Equal(t, 2, restoreResult.Imported["listening_events"])

	// Verify state was rebuilt
	progress, err := destStore.GetState(ctx, "user-1", "book-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1800000), progress.CurrentPositionMs)     // 30 minutes
	assert.Equal(t, int64(1800000), progress.TotalListenTimeMs)     // 30 minutes total
	assert.InDelta(t, 0.5, progress.ComputeProgress(3600000), 0.01) // 50% progress (30min / 60min)
	assert.False(t, progress.IsFinished)                            // Not finished yet

	// Now test RebuildProgress directly by clearing state and rebuilding
	// First, let's verify we can call RebuildProgress
	err = restoreSvc.RebuildProgress(ctx)
	require.NoError(t, err)

	// State should still be correct after rebuild
	progress, err = destStore.GetState(ctx, "user-1", "book-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1800000), progress.CurrentPositionMs)
}
