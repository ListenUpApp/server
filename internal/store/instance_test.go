package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestStore creates a temporary store for testing.
func setupTestStore(t *testing.T) (*Store, func()) { //nolint:gocritic // Test helper return values are clear from context
	t.Helper()

	// Create temp directory for test database.
	tmpDir, err := os.MkdirTemp("", "listenup-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store with noop emitter for testing.
	store, err := New(dbPath, nil, NewNoopEmitter())
	require.NoError(t, err)
	require.NotNil(t, store)

	// Return cleanup function.
	cleanup := func() {
		_ = store.Close()        //nolint:errcheck // Test cleanup
		_ = os.RemoveAll(tmpDir) //nolint:errcheck // Test cleanup
	}

	return store, cleanup
}

func TestCreateInstance(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Test creating instance.
	instance, err := store.CreateInstance(ctx)
	require.NoError(t, err)
	assert.NotNil(t, instance)
	assert.NotEmpty(t, instance.ID)
	assert.Contains(t, instance.ID, "lib-")
	assert.Empty(t, instance.RootUserID)
	assert.True(t, instance.IsSetupRequired())
	assert.False(t, instance.CreatedAt.IsZero())
	assert.False(t, instance.UpdatedAt.IsZero())
}

func TestCreateInstance_AlreadyExists(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create first instance.
	_, err := store.CreateInstance(ctx)
	require.NoError(t, err)

	// Try to create second instance - should fail.
	_, err = store.CreateInstance(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrServerAlreadyExists)
}

func TestGetInstance(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create instance first.
	created, err := store.CreateInstance(ctx)
	require.NoError(t, err)

	// Get instance.
	instance, err := store.GetInstance(ctx)
	require.NoError(t, err)
	assert.NotNil(t, instance)
	assert.Equal(t, created.ID, instance.ID)
	assert.Equal(t, created.RootUserID, instance.RootUserID)
}

func TestGetInstance_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get instance that doesn't exist.
	_, err := store.GetInstance(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrServerNotFound)
}

func TestUpdateInstance(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create instance.
	instance, err := store.CreateInstance(ctx)
	require.NoError(t, err)

	// Wait a moment to ensure UpdatedAt will be different.
	time.Sleep(10 * time.Millisecond)

	// Update instance with root user.
	instance.SetRootUser("user_test123")
	err = store.UpdateInstance(ctx, instance)
	require.NoError(t, err)

	// Verify update.
	updated, err := store.GetInstance(ctx)
	require.NoError(t, err)
	assert.Equal(t, "user_test123", updated.RootUserID)
	assert.False(t, updated.IsSetupRequired())
	assert.True(t, updated.UpdatedAt.After(instance.CreatedAt))
}

func TestUpdateInstance_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Try to update instance that doesn't exist.
	instance := &domain.Instance{
		ID:         "server-001",
		RootUserID: "user_test123",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := store.UpdateInstance(ctx, instance)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrServerNotFound)
}

func TestInitializeInstance_Creates(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize should create instance.
	instance, err := store.InitializeInstance(ctx)
	require.NoError(t, err)
	assert.NotNil(t, instance)
	assert.NotEmpty(t, instance.ID)
	assert.Contains(t, instance.ID, "lib-")
	assert.True(t, instance.IsSetupRequired())
	assert.Empty(t, instance.RootUserID)
}

func TestInitializeInstance_ReturnsExisting(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create instance.
	created, err := store.CreateInstance(ctx)
	require.NoError(t, err)

	// Update it with root user.
	created.SetRootUser("user_existing123")
	err = store.UpdateInstance(ctx, created)
	require.NoError(t, err)

	// Initialize should return existing instance.
	instance, err := store.InitializeInstance(ctx)
	require.NoError(t, err)
	assert.NotNil(t, instance)
	assert.False(t, instance.IsSetupRequired())
	assert.Equal(t, "user_existing123", instance.RootUserID)
	assert.Equal(t, created.ID, instance.ID)
}

func TestStore_Persistence(t *testing.T) {
	// Create temp directory for test database.
	tmpDir, err := os.MkdirTemp("", "listenup-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir) //nolint:errcheck // Test cleanup

	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	// Create store and instance.
	store1, err := New(dbPath, nil, NewNoopEmitter())
	require.NoError(t, err)

	instance, err := store1.CreateInstance(ctx)
	require.NoError(t, err)
	instance.SetRootUser("user_persist123")
	err = store1.UpdateInstance(ctx, instance)
	require.NoError(t, err)

	// Close store.
	err = store1.Close()
	require.NoError(t, err)

	// Reopen store.
	store2, err := New(dbPath, nil, NewNoopEmitter())
	require.NoError(t, err)
	defer store2.Close() //nolint:errcheck // Test cleanup

	// Verify data persisted.
	loaded, err := store2.GetInstance(ctx)
	require.NoError(t, err)
	assert.Equal(t, instance.ID, loaded.ID)
	assert.Equal(t, "user_persist123", loaded.RootUserID)
	assert.False(t, loaded.IsSetupRequired())
}
