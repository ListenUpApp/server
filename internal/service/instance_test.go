package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestService creates a service with a temporary store for testing.
func setupTestService(t *testing.T) (*InstanceService, func()) { //nolint:gocritic // Test helper return values are clear from context
	t.Helper()

	// Create temp directory for test database.
	tmpDir, err := os.MkdirTemp("", "listenup-service-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store.
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	// Create service.
	service := NewInstanceService(s, nil)

	// Return cleanup function.
	cleanup := func() {
		_ = s.Close()            //nolint:errcheck // Test cleanup
		_ = os.RemoveAll(tmpDir) //nolint:errcheck // Test cleanup
	}

	return service, cleanup
}

func TestInstanceService_GetInstance(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize instance first.
	_, err := service.InitializeInstance(ctx)
	require.NoError(t, err)

	// Get instance.
	instance, err := service.GetInstance(ctx)
	require.NoError(t, err)
	assert.NotNil(t, instance)
	assert.Equal(t, "server-001", instance.ID)
	assert.False(t, instance.HasRootUser)
}

func TestInstanceService_GetInstance_NotFound(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get instance before it's created.
	_, err := service.GetInstance(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "instance configuration not found")
}

func TestInstanceService_InitializeInstance_Creates(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize should create instance.
	instance, err := service.InitializeInstance(ctx)
	require.NoError(t, err)
	assert.NotNil(t, instance)
	assert.Equal(t, "server-001", instance.ID)
	assert.False(t, instance.HasRootUser)
}

func TestInstanceService_InitializeInstance_ReturnsExisting(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize first time.
	instance1, err := service.InitializeInstance(ctx)
	require.NoError(t, err)

	// Initialize second time - should return existing.
	instance2, err := service.InitializeInstance(ctx)
	require.NoError(t, err)
	assert.Equal(t, instance1.ID, instance2.ID)
	assert.True(t, instance1.CreatedAt.Equal(instance2.CreatedAt), "CreatedAt timestamps should be equal")
}

func TestInstanceService_IsInstanceSetup(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize instance (HasRootUser=false).
	_, err := service.InitializeInstance(ctx)
	require.NoError(t, err)

	// Check setup status.
	isSetup, err := service.IsInstanceSetup(ctx)
	require.NoError(t, err)
	assert.False(t, isSetup)

	// Mark as setup.
	err = service.MarkInstanceAsSetup(ctx)
	require.NoError(t, err)

	// Check setup status again.
	isSetup, err = service.IsInstanceSetup(ctx)
	require.NoError(t, err)
	assert.True(t, isSetup)
}

func TestInstanceService_IsInstanceSetup_NotFound(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Check setup status before instance exists.
	isSetup, err := service.IsInstanceSetup(ctx)
	require.NoError(t, err)
	assert.False(t, isSetup, "Should return false when instance doesn't exist")
}

func TestInstanceService_MarkInstanceAsSetup(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize instance.
	instance, err := service.InitializeInstance(ctx)
	require.NoError(t, err)
	assert.False(t, instance.HasRootUser)

	// Wait a moment to ensure UpdatedAt will be different.
	time.Sleep(10 * time.Millisecond)

	// Mark as setup.
	err = service.MarkInstanceAsSetup(ctx)
	require.NoError(t, err)

	// Verify it's marked as setup.
	updated, err := service.GetInstance(ctx)
	require.NoError(t, err)
	assert.True(t, updated.HasRootUser)
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt))
}

func TestInstanceService_MarkInstanceAsSetup_AlreadySetup(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize and mark as setup.
	_, err := service.InitializeInstance(ctx)
	require.NoError(t, err)

	err = service.MarkInstanceAsSetup(ctx)
	require.NoError(t, err)

	// Try to mark as setup again - should fail.
	err = service.MarkInstanceAsSetup(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already set up")
}

func TestInstanceService_MarkInstanceAsSetup_NotFound(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Try to mark as setup before instance exists.
	err := service.MarkInstanceAsSetup(ctx)
	assert.Error(t, err)
}
