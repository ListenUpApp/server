package store

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUser(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test123",
		},
		Email:        "test@example.com",
		PasswordHash: "hashed_password",
		DisplayName:  "Test User",
		IsRoot:       false,
	}
	user.InitTimestamps()

	err := store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Verify user can be retrieved
	retrieved, err := store.GetUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
	assert.Equal(t, user.Email, retrieved.Email)
	assert.Equal(t, user.DisplayName, retrieved.DisplayName)
}

func TestCreateUser_DuplicateID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test123",
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	// First creation succeeds
	err := store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Second creation with same ID fails
	user2 := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test123",
		},
		Email:       "different@example.com",
		DisplayName: "Different User",
	}
	user2.InitTimestamps()

	err = store.CreateUser(ctx, user2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUserExists)
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user1 := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test1",
		},
		Email:       "test@example.com",
		DisplayName: "User 1",
	}
	user1.InitTimestamps()

	err := store.CreateUser(ctx, user1)
	require.NoError(t, err)

	// Second user with same email fails
	user2 := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test2",
		},
		Email:       "test@example.com", // Same email
		DisplayName: "User 2",
	}
	user2.InitTimestamps()

	err = store.CreateUser(ctx, user2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrEmailExists)
}

func TestCreateUser_EmailCaseInsensitive(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user1 := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test1",
		},
		Email:       "Test@Example.COM",
		DisplayName: "User 1",
	}
	user1.InitTimestamps()

	err := store.CreateUser(ctx, user1)
	require.NoError(t, err)

	// Second user with different case email should fail
	user2 := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test2",
		},
		Email:       "test@example.com", // Different case
		DisplayName: "User 2",
	}
	user2.InitTimestamps()

	err = store.CreateUser(ctx, user2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrEmailExists)
}

func TestGetUser_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetUser(ctx, "user_nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestGetUser_SoftDeleted(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test123",
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err := store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Soft delete user
	user.MarkDeleted()
	err = store.UpdateUser(ctx, user)
	require.NoError(t, err)

	// GetUser should return not found
	_, err = store.GetUser(ctx, user.ID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestGetUserByEmail_Success(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test123",
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err := store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Retrieve by email
	retrieved, err := store.GetUserByEmail(ctx, "test@example.com")
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
	assert.Equal(t, user.Email, retrieved.Email)
}

func TestGetUserByEmail_CaseInsensitive(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test123",
		},
		Email:       "Test@Example.COM",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err := store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Retrieve with different case
	retrieved, err := store.GetUserByEmail(ctx, "test@example.com")
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetUserByEmail(ctx, "nonexistent@example.com")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUpdateUser_Success(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test123",
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err := store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Wait a moment to ensure UpdatedAt will be different
	time.Sleep(10 * time.Millisecond)

	// Update user
	user.DisplayName = "Updated User"
	user.FirstName = "Test"
	user.LastName = "User"
	err = store.UpdateUser(ctx, user)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated User", updated.DisplayName)
	assert.Equal(t, "Test", updated.FirstName)
	assert.Equal(t, "User", updated.LastName)
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt))
}

func TestUpdateUser_ChangeEmail(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test123",
		},
		Email:       "old@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err := store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Change email
	user.Email = "new@example.com"
	err = store.UpdateUser(ctx, user)
	require.NoError(t, err)

	// Old email should not work
	_, err = store.GetUserByEmail(ctx, "old@example.com")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)

	// New email should work
	retrieved, err := store.GetUserByEmail(ctx, "new@example.com")
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
}

func TestUpdateUser_ChangeEmailConflict(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create first user
	user1 := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test1",
		},
		Email:       "user1@example.com",
		DisplayName: "User 1",
	}
	user1.InitTimestamps()

	err := store.CreateUser(ctx, user1)
	require.NoError(t, err)

	// Create second user
	user2 := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_test2",
		},
		Email:       "user2@example.com",
		DisplayName: "User 2",
	}
	user2.InitTimestamps()

	err = store.CreateUser(ctx, user2)
	require.NoError(t, err)

	// Try to change user2's email to user1's email
	user2.Email = "user1@example.com"
	err = store.UpdateUser(ctx, user2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrEmailExists)
}

func TestUpdateUser_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: "user_nonexistent",
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err := store.UpdateUser(ctx, user)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUserNotFound)
}
