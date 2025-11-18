package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/config"
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

	// Create test config.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name:      "Test Server",
			LocalURL:  "http://localhost:8080",
			RemoteURL: "",
		},
	}

	// Create service.
	service := NewInstanceService(s, nil, cfg)

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
	assert.True(t, instance.IsSetupRequired())
	assert.Empty(t, instance.RootUserID)
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
	assert.True(t, instance.IsSetupRequired())
	assert.Empty(t, instance.RootUserID)
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

	// Initialize instance (setup required).
	_, err := service.InitializeInstance(ctx)
	require.NoError(t, err)

	// Check setup status.
	isSetup, err := service.IsInstanceSetup(ctx)
	require.NoError(t, err)
	assert.False(t, isSetup)

	// Set root user to complete setup.
	err = service.SetRootUser(ctx, "user_test_root")
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

func TestInstanceService_SetRootUser(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize instance.
	instance, err := service.InitializeInstance(ctx)
	require.NoError(t, err)
	assert.True(t, instance.IsSetupRequired())

	// Wait a moment to ensure UpdatedAt will be different.
	time.Sleep(10 * time.Millisecond)

	// Set root user.
	err = service.SetRootUser(ctx, "user_test_123")
	require.NoError(t, err)

	// Verify root user is set.
	updated, err := service.GetInstance(ctx)
	require.NoError(t, err)
	assert.Equal(t, "user_test_123", updated.RootUserID)
	assert.False(t, updated.IsSetupRequired())
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt))
}

func TestInstanceService_SetRootUser_AlreadySet(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize and set root user.
	_, err := service.InitializeInstance(ctx)
	require.NoError(t, err)

	err = service.SetRootUser(ctx, "user_first")
	require.NoError(t, err)

	// Try to set root user again - should fail.
	err = service.SetRootUser(ctx, "user_second")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "root user already configured")
}

func TestInstanceService_SetRootUser_NotFound(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()

	// Try to set root user before instance exists.
	err := service.SetRootUser(ctx, "user_test")
	assert.Error(t, err)
}
