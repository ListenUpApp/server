package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsersEntity_Create(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "users-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	userID, err := id.Generate("user")
	require.NoError(t, err)

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: userID,
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err = s.Users.Create(ctx, user.ID, user)
	assert.NoError(t, err)
}

func TestUsersEntity_GetByEmail(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "users-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	userID, err := id.Generate("user")
	require.NoError(t, err)

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: userID,
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err = s.Users.Create(ctx, user.ID, user)
	require.NoError(t, err)

	// Get by email index
	retrieved, err := s.Users.GetByIndex(ctx, "email", "test@example.com")
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
	assert.Equal(t, user.Email, retrieved.Email)
}

func TestUsersEntity_EmailConflict(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "users-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()

	user1ID, err := id.Generate("user")
	require.NoError(t, err)

	user1 := &domain.User{
		Syncable: domain.Syncable{
			ID: user1ID,
		},
		Email:       "same@example.com",
		DisplayName: "User 1",
	}
	user1.InitTimestamps()

	err = s.Users.Create(ctx, user1.ID, user1)
	require.NoError(t, err)

	// Try to create another user with same email
	user2ID, err := id.Generate("user")
	require.NoError(t, err)

	user2 := &domain.User{
		Syncable: domain.Syncable{
			ID: user2ID,
		},
		Email:       "same@example.com",
		DisplayName: "User 2",
	}
	user2.InitTimestamps()

	err = s.Users.Create(ctx, user2.ID, user2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email")
}

func TestUsersEntity_EmailCaseInsensitive(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "users-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()
	userID, err := id.Generate("user")
	require.NoError(t, err)

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: userID,
		},
		Email:       "test@example.com",
		DisplayName: "Test User",
	}
	user.InitTimestamps()

	err = s.Users.Create(ctx, user.ID, user)
	require.NoError(t, err)

	// Test case-insensitive email lookups
	testCases := []struct {
		name  string
		email string
	}{
		{"exact match", "test@example.com"},
		{"all uppercase", "TEST@EXAMPLE.COM"},
		{"mixed case", "TeSt@ExAmPlE.cOm"},
		{"with whitespace", "  test@example.com  "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			retrieved, err := s.Users.GetByIndex(ctx, "email", tc.email)
			require.NoError(t, err, "should find user with email %q", tc.email)
			assert.Equal(t, user.ID, retrieved.ID)
			assert.Equal(t, user.Email, retrieved.Email)
		})
	}
}
