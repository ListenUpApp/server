package store

import (
	"context"
	"testing"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// === CreateShare Tests ===

func TestCreateShare(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	share := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}

	err := s.CreateShare(ctx, share)
	require.NoError(t, err)
	assert.NotEmpty(t, share.ID, "share ID should be generated")

	// Verify it was created
	fetchedShare, err := s.GetShare(ctx, share.ID)
	require.NoError(t, err)
	assert.Equal(t, share.ID, fetchedShare.ID)
	assert.Equal(t, share.CollectionID, fetchedShare.CollectionID)
	assert.Equal(t, share.SharedWithUserID, fetchedShare.SharedWithUserID)
	assert.Equal(t, share.SharedByUserID, fetchedShare.SharedByUserID)
	assert.Equal(t, domain.PermissionRead, fetchedShare.Permission)
}

func TestCreateShare_WithExistingID(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	share := &domain.CollectionShare{
		Syncable:         domain.Syncable{ID: "share-custom-id"},
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionWrite,
	}

	err := s.CreateShare(ctx, share)
	require.NoError(t, err)
	assert.Equal(t, "share-custom-id", share.ID)

	// Verify it was created with the custom ID
	fetchedShare, err := s.GetShare(ctx, "share-custom-id")
	require.NoError(t, err)
	assert.Equal(t, "share-custom-id", fetchedShare.ID)
}

func TestCreateShare_DuplicateUserAndCollection(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create first share
	share1 := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share1)
	require.NoError(t, err)

	// Try to create duplicate share (same user, same collection)
	share2 := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionWrite, // Different permission
	}
	err = s.CreateShare(ctx, share2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrShareAlreadyExists)
}

func TestCreateShare_MultipleSharesSameCollection(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create share with user1
	share1 := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-1",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share1)
	require.NoError(t, err)

	// Create share with user2 (same collection)
	share2 := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-2",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionWrite,
	}
	err = s.CreateShare(ctx, share2)
	require.NoError(t, err, "should allow sharing same collection with different users")

	// Create share with user3 (same collection)
	share3 := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-3",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err = s.CreateShare(ctx, share3)
	require.NoError(t, err, "should allow sharing same collection with multiple users")

	// Verify all shares exist
	shares, err := s.GetSharesForCollection(ctx, "coll-123")
	require.NoError(t, err)
	assert.Len(t, shares, 3)
}

func TestCreateShare_MultipleSharesSameUser(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create share for collection1
	share1 := &domain.CollectionShare{
		CollectionID:     "coll-1",
		SharedWithUserID: "user-recipient",
		SharedByUserID:   "user-owner-1",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share1)
	require.NoError(t, err)

	// Create share for collection2 (same recipient)
	share2 := &domain.CollectionShare{
		CollectionID:     "coll-2",
		SharedWithUserID: "user-recipient",
		SharedByUserID:   "user-owner-2",
		Permission:       domain.PermissionWrite,
	}
	err = s.CreateShare(ctx, share2)
	require.NoError(t, err, "should allow user to receive shares for multiple collections")

	// Verify all shares exist
	shares, err := s.GetSharesForUser(ctx, "user-recipient")
	require.NoError(t, err)
	assert.Len(t, shares, 2)
}

// === GetShare Tests ===

func TestGetShare(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	share := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionWrite,
	}
	err := s.CreateShare(ctx, share)
	require.NoError(t, err)

	fetchedShare, err := s.GetShare(ctx, share.ID)
	require.NoError(t, err)
	assert.Equal(t, share.ID, fetchedShare.ID)
	assert.Equal(t, domain.PermissionWrite, fetchedShare.Permission)
}

func TestGetShare_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := s.GetShare(ctx, "share-nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrShareNotFound)
}

// === GetShareForUserAndCollection Tests ===

func TestGetShareForUserAndCollection(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	share := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share)
	require.NoError(t, err)

	// Find the share by user and collection
	found, err := s.GetShareForUserAndCollection(ctx, "user-456", "coll-123")
	require.NoError(t, err)
	assert.Equal(t, share.ID, found.ID)
}

func TestGetShareForUserAndCollection_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create a share for different user
	share := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share)
	require.NoError(t, err)

	// Search for different user/collection combinations
	_, err = s.GetShareForUserAndCollection(ctx, "user-999", "coll-123")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrShareNotFound)

	_, err = s.GetShareForUserAndCollection(ctx, "user-456", "coll-999")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrShareNotFound)
}

// === GetSharesForUser Tests ===

func TestGetSharesForUser(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create shares for the target user
	share1 := &domain.CollectionShare{
		CollectionID:     "coll-1",
		SharedWithUserID: "user-target",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share1)
	require.NoError(t, err)

	share2 := &domain.CollectionShare{
		CollectionID:     "coll-2",
		SharedWithUserID: "user-target",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionWrite,
	}
	err = s.CreateShare(ctx, share2)
	require.NoError(t, err)

	// Create share for different user (should not be returned)
	share3 := &domain.CollectionShare{
		CollectionID:     "coll-3",
		SharedWithUserID: "user-other",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err = s.CreateShare(ctx, share3)
	require.NoError(t, err)

	// Get shares for target user
	shares, err := s.GetSharesForUser(ctx, "user-target")
	require.NoError(t, err)
	assert.Len(t, shares, 2)
}

func TestGetSharesForUser_Empty(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	shares, err := s.GetSharesForUser(ctx, "user-no-shares")
	require.NoError(t, err)
	assert.Empty(t, shares)
}

// === GetSharesForCollection Tests ===

func TestGetSharesForCollection(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create shares for the target collection
	share1 := &domain.CollectionShare{
		CollectionID:     "coll-target",
		SharedWithUserID: "user-1",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share1)
	require.NoError(t, err)

	share2 := &domain.CollectionShare{
		CollectionID:     "coll-target",
		SharedWithUserID: "user-2",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionWrite,
	}
	err = s.CreateShare(ctx, share2)
	require.NoError(t, err)

	// Create share for different collection (should not be returned)
	share3 := &domain.CollectionShare{
		CollectionID:     "coll-other",
		SharedWithUserID: "user-3",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err = s.CreateShare(ctx, share3)
	require.NoError(t, err)

	// Get shares for target collection
	shares, err := s.GetSharesForCollection(ctx, "coll-target")
	require.NoError(t, err)
	assert.Len(t, shares, 2)
}

func TestGetSharesForCollection_Empty(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	shares, err := s.GetSharesForCollection(ctx, "coll-no-shares")
	require.NoError(t, err)
	assert.Empty(t, shares)
}

// === UpdateShare Tests ===

func TestUpdateShare(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	share := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share)
	require.NoError(t, err)

	// Update permission
	share.Permission = domain.PermissionWrite
	err = s.UpdateShare(ctx, share)
	require.NoError(t, err)

	// Verify update
	fetchedShare, err := s.GetShare(ctx, share.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PermissionWrite, fetchedShare.Permission)
}

func TestUpdateShare_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	share := &domain.CollectionShare{
		Syncable:         domain.Syncable{ID: "share-nonexistent"},
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}

	err := s.UpdateShare(ctx, share)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrShareNotFound)
}

// === DeleteShare Tests ===

func TestDeleteShare(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	share := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share)
	require.NoError(t, err)

	// Delete the share
	err = s.DeleteShare(ctx, share.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = s.GetShare(ctx, share.ID)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrShareNotFound)
}

func TestDeleteShare_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	err := s.DeleteShare(ctx, "share-nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrShareNotFound)
}

// === DeleteSharesForCollection Tests ===

func TestDeleteSharesForCollection(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple shares for target collection
	share1 := &domain.CollectionShare{
		CollectionID:     "coll-to-delete",
		SharedWithUserID: "user-1",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err := s.CreateShare(ctx, share1)
	require.NoError(t, err)

	share2 := &domain.CollectionShare{
		CollectionID:     "coll-to-delete",
		SharedWithUserID: "user-2",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionWrite,
	}
	err = s.CreateShare(ctx, share2)
	require.NoError(t, err)

	// Create share for different collection (should NOT be deleted)
	share3 := &domain.CollectionShare{
		CollectionID:     "coll-keep",
		SharedWithUserID: "user-3",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}
	err = s.CreateShare(ctx, share3)
	require.NoError(t, err)

	// Delete all shares for target collection
	err = s.DeleteSharesForCollection(ctx, "coll-to-delete")
	require.NoError(t, err)

	// Verify target collection shares are deleted
	shares, err := s.GetSharesForCollection(ctx, "coll-to-delete")
	require.NoError(t, err)
	assert.Empty(t, shares)

	// Verify other collection's share still exists
	otherShares, err := s.GetSharesForCollection(ctx, "coll-keep")
	require.NoError(t, err)
	assert.Len(t, otherShares, 1)
}

func TestDeleteSharesForCollection_NoShares(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Should not error when no shares exist
	err := s.DeleteSharesForCollection(ctx, "coll-empty")
	assert.NoError(t, err)
}

// === Permission Value Tests ===

func TestSharePermission_Values(t *testing.T) {
	assert.Equal(t, domain.SharePermission(0), domain.PermissionRead)
	assert.Equal(t, domain.SharePermission(1), domain.PermissionWrite)
}

func TestSharePermission_CanRead(t *testing.T) {
	assert.True(t, domain.PermissionRead.CanRead())
	assert.True(t, domain.PermissionWrite.CanRead(), "Write implies Read")
}

func TestSharePermission_CanWrite(t *testing.T) {
	assert.False(t, domain.PermissionRead.CanWrite())
	assert.True(t, domain.PermissionWrite.CanWrite())
}

func TestSharePermission_String(t *testing.T) {
	assert.Equal(t, "read", domain.PermissionRead.String())
	assert.Equal(t, "write", domain.PermissionWrite.String())
	assert.Equal(t, "unknown", domain.SharePermission(99).String())
}

func TestParseSharePermission(t *testing.T) {
	tests := []struct {
		input       string
		expected    domain.SharePermission
		shouldParse bool
	}{
		{"read", domain.PermissionRead, true},
		{"write", domain.PermissionWrite, true},
		{"invalid", domain.PermissionRead, false},
		{"", domain.PermissionRead, false},
		{"READ", domain.PermissionRead, false}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			perm, ok := domain.ParseSharePermission(tt.input)
			assert.Equal(t, tt.shouldParse, ok)
			assert.Equal(t, tt.expected, perm)
		})
	}
}

// === Context Cancellation Tests ===

func TestCreateShare_RespectsContextCancellation(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	share := &domain.CollectionShare{
		CollectionID:     "coll-123",
		SharedWithUserID: "user-456",
		SharedByUserID:   "user-owner",
		Permission:       domain.PermissionRead,
	}

	err := s.CreateShare(ctx, share)
	assert.Error(t, err)
}

func TestGetShare_RespectsContextCancellation(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.GetShare(ctx, "share-123")
	assert.Error(t, err)
}
