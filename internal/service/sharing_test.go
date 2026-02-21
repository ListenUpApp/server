package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/listenupapp/listenup-server/internal/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSharingTest creates a sharing service with temporary storage for testing.
func setupSharingTest(t *testing.T) (*SharingService, store.Store, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "listenup-sharing-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store
	s, err := sqlite.Open(dbPath, nil)
	require.NoError(t, err)

	// Create logger (discard output in tests)
	logger := slog.New(slog.DiscardHandler)

	// Create sharing service
	sharingService := NewSharingService(s, logger)

	// Cleanup function
	cleanup := func() {
		_ = s.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return sharingService, s, cleanup
}

// createTestUserWithPermissions creates a test user with specific permissions.
func createTestUserWithPermissions(t *testing.T, s store.Store, email string, canShare bool) *domain.User {
	t.Helper()

	userID, err := id.Generate("user")
	require.NoError(t, err)

	user := &domain.User{
		Syncable: domain.Syncable{
			ID:        userID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Email:       email,
		FirstName:   "Test",
		LastName:    "User",
		DisplayName: "Test User",
		IsRoot:      false,
		Permissions: domain.UserPermissions{
			CanShare: canShare,
			CanEdit:  true,
		},
	}

	err = s.CreateUser(context.Background(), user)
	require.NoError(t, err)

	return user
}

// createTestLibrary creates a test library.
func createTestLibrary(t *testing.T, s store.Store, ownerID string) *domain.Library {
	t.Helper()

	libID, err := id.Generate("lib")
	require.NoError(t, err)

	library := &domain.Library{
		ID:        libID,
		OwnerID:   ownerID,
		Name:      "Test Library",
		ScanPaths: []string{"/test/audiobooks"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = s.CreateLibrary(context.Background(), library)
	require.NoError(t, err)

	return library
}

// createTestCollection creates a test collection owned by a user.
func createTestCollection(t *testing.T, s store.Store, ownerID, libraryID, name string) *domain.Collection {
	t.Helper()

	collID, err := id.Generate("coll")
	require.NoError(t, err)

	collection := &domain.Collection{
		ID:        collID,
		LibraryID: libraryID,
		OwnerID:   ownerID,
		Name:      name,
		BookIDs:   []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = s.CreateCollection(context.Background(), collection)
	require.NoError(t, err)

	return collection
}

// === ShareCollection Tests ===

func TestShareCollection_OwnerCanShare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create owner user with share permission
	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)

	// Create recipient user
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)

	// Create library and collection
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Owner should be able to share
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)
	assert.NotNil(t, share)
	assert.Equal(t, collection.ID, share.CollectionID)
	assert.Equal(t, recipient.ID, share.SharedWithUserID)
	assert.Equal(t, owner.ID, share.SharedByUserID)
	assert.Equal(t, domain.PermissionRead, share.Permission)
}

func TestShareCollection_OwnerCanShareWithWritePermission(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Share with Write permission
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionWrite)
	require.NoError(t, err)
	assert.Equal(t, domain.PermissionWrite, share.Permission)
	assert.True(t, share.Permission.CanWrite())
	assert.True(t, share.Permission.CanRead())
}

func TestShareCollection_NonOwnerCannotShare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create owner, non-owner, and recipient
	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	nonOwner := createTestUserWithPermissions(t, s, "nonowner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)

	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "Owner's Collection")

	// Non-owner tries to share owner's collection - should fail
	// Note: System returns generic error to avoid leaking ownership info (good security practice)
	_, err := sharingService.ShareCollection(ctx, nonOwner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	assert.Error(t, err, "non-owner should not be able to share collection")
}

func TestShareCollection_UserWithoutSharePermissionCannotShare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create owner WITHOUT share permission
	owner := createTestUserWithPermissions(t, s, "owner@test.com", false) // canShare = false
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)

	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Owner without CanShare permission tries to share - should fail
	_, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not have permission to share")
}

func TestShareCollection_CannotShareWithSelf(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Owner tries to share with themselves - should fail
	_, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, owner.ID, domain.PermissionRead)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot share collection with yourself")
}

func TestShareCollection_CannotShareWithNonexistentUser(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Owner tries to share with non-existent user - should fail
	_, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, "user-does-not-exist", domain.PermissionRead)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get user to share with")
}

func TestShareCollection_DuplicateShareRejected(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// First share succeeds
	_, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Duplicate share should fail
	_, err = sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestShareCollection_NonexistentCollection(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)

	// Try to share non-existent collection
	_, err := sharingService.ShareCollection(ctx, owner.ID, "coll-does-not-exist", recipient.ID, domain.PermissionRead)
	assert.Error(t, err)
}

// === UnshareCollection Tests ===

func TestUnshareCollection_OwnerCanUnshare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Create share
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Owner unshares
	err = sharingService.UnshareCollection(ctx, owner.ID, share.ID)
	assert.NoError(t, err)

	// Verify share is deleted
	_, err = s.GetShare(ctx, share.ID)
	assert.Error(t, err)
}

func TestUnshareCollection_ShareCreatorCanUnshare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Owner creates share
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Share creator (owner in this case) can unshare
	err = sharingService.UnshareCollection(ctx, owner.ID, share.ID)
	assert.NoError(t, err)
}

func TestUnshareCollection_RandomUserCannotUnshare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	randomUser := createTestUserWithPermissions(t, s, "random@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Create share
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Random user tries to unshare - should fail
	// Note: System returns generic error to avoid leaking share existence info
	err = sharingService.UnshareCollection(ctx, randomUser.ID, share.ID)
	assert.Error(t, err, "random user should not be able to unshare")
}

func TestUnshareCollection_RecipientCannotUnshare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Create share
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Recipient tries to unshare - should fail (they're not owner or creator)
	err = sharingService.UnshareCollection(ctx, recipient.ID, share.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only collection owner or share creator can unshare")
}

func TestUnshareCollection_NonexistentShare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)

	// Try to unshare non-existent share
	err := sharingService.UnshareCollection(ctx, owner.ID, "share-does-not-exist")
	assert.Error(t, err)
}

// === UpdateSharePermission Tests ===

func TestUpdateSharePermission_OwnerCanUpdate(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Create share with Read permission
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)
	assert.Equal(t, domain.PermissionRead, share.Permission)

	// Owner updates to Write permission
	updatedShare, err := sharingService.UpdateSharePermission(ctx, owner.ID, share.ID, domain.PermissionWrite)
	require.NoError(t, err)
	assert.Equal(t, domain.PermissionWrite, updatedShare.Permission)

	// Verify persistence
	fetchedShare, err := s.GetShare(ctx, share.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PermissionWrite, fetchedShare.Permission)
}

func TestUpdateSharePermission_OwnerCanDowngrade(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Create share with Write permission
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionWrite)
	require.NoError(t, err)

	// Owner downgrades to Read permission
	updatedShare, err := sharingService.UpdateSharePermission(ctx, owner.ID, share.ID, domain.PermissionRead)
	require.NoError(t, err)
	assert.Equal(t, domain.PermissionRead, updatedShare.Permission)
}

func TestUpdateSharePermission_NonOwnerCannotUpdate(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	randomUser := createTestUserWithPermissions(t, s, "random@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Create share
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Random user tries to update - should fail
	// Note: System returns generic error to avoid leaking share existence info
	_, err = sharingService.UpdateSharePermission(ctx, randomUser.ID, share.ID, domain.PermissionWrite)
	assert.Error(t, err, "random user should not be able to update share permissions")
}

func TestUpdateSharePermission_RecipientCannotUpdate(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Create share with Read permission
	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Recipient tries to escalate to Write permission - should fail
	_, err = sharingService.UpdateSharePermission(ctx, recipient.ID, share.ID, domain.PermissionWrite)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only collection owner can update share permissions")
}

// === ListCollectionShares Tests ===

func TestListCollectionShares_OwnerCanList(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner-list@test.com", true)
	recipient1 := createTestUserWithPermissions(t, s, "recipient1-list@test.com", true)
	recipient2 := createTestUserWithPermissions(t, s, "recipient2-list@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "List Test Collection")

	// Create two shares with different recipients
	share1, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient1.ID, domain.PermissionRead)
	require.NoError(t, err)
	assert.NotNil(t, share1)

	share2, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient2.ID, domain.PermissionWrite)
	require.NoError(t, err)
	assert.NotNil(t, share2)

	// Owner lists shares
	shares, err := sharingService.ListCollectionShares(ctx, owner.ID, collection.ID)
	require.NoError(t, err)
	assert.Len(t, shares, 2)
}

func TestListCollectionShares_NonOwnerCannotList(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner-nonlist@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient-nonlist@test.com", true)
	randomUser := createTestUserWithPermissions(t, s, "random-nonlist@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "NonOwner List Collection")

	// Create share
	_, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Random user tries to list - should fail
	// Note: System returns generic error to avoid leaking collection info
	_, err = sharingService.ListCollectionShares(ctx, randomUser.ID, collection.ID)
	assert.Error(t, err, "non-owner should not be able to list shares")
}

func TestListCollectionShares_RecipientCannotList(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Create share
	_, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Recipient tries to list shares on collection - should fail
	_, err = sharingService.ListCollectionShares(ctx, recipient.ID, collection.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only collection owner can list shares")
}

// === ListSharedWithMe Tests ===

func TestListSharedWithMe_ReturnsUserShares(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner1 := createTestUserWithPermissions(t, s, "owner1-sharedwithme@test.com", true)
	owner2 := createTestUserWithPermissions(t, s, "owner2-sharedwithme@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient-sharedwithme@test.com", true)

	library := createTestLibrary(t, s, owner1.ID)
	collection1 := createTestCollection(t, s, owner1.ID, library.ID, "SharedWithMe Collection 1")
	collection2 := createTestCollection(t, s, owner2.ID, library.ID, "SharedWithMe Collection 2")

	// Two different owners share with same recipient
	share1, err := sharingService.ShareCollection(ctx, owner1.ID, collection1.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)
	assert.NotNil(t, share1)

	share2, err := sharingService.ShareCollection(ctx, owner2.ID, collection2.ID, recipient.ID, domain.PermissionWrite)
	require.NoError(t, err)
	assert.NotNil(t, share2)

	// Recipient lists shares shared with them
	shares, err := sharingService.ListSharedWithMe(ctx, recipient.ID)
	require.NoError(t, err)
	assert.Len(t, shares, 2)
}

func TestListSharedWithMe_ReturnsEmptyForNoShares(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	user := createTestUserWithPermissions(t, s, "user@test.com", true)

	// User with no shares
	shares, err := sharingService.ListSharedWithMe(ctx, user.ID)
	require.NoError(t, err)
	assert.Empty(t, shares)
}

// === GetShare Tests ===

func TestGetShare_OwnerCanView(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Owner can view share
	fetchedShare, err := sharingService.GetShare(ctx, owner.ID, share.ID)
	require.NoError(t, err)
	assert.Equal(t, share.ID, fetchedShare.ID)
}

func TestGetShare_RecipientCanView(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Recipient can view share
	fetchedShare, err := sharingService.GetShare(ctx, recipient.ID, share.ID)
	require.NoError(t, err)
	assert.Equal(t, share.ID, fetchedShare.ID)
}

func TestGetShare_UninvolvedUserCannotView(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner-getshare@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient-getshare@test.com", true)
	randomUser := createTestUserWithPermissions(t, s, "random-getshare@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "GetShare Collection")

	share, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	require.NoError(t, err)

	// Random user cannot view share
	// Note: System returns generic error to avoid leaking share existence info
	_, err = sharingService.GetShare(ctx, randomUser.ID, share.ID)
	assert.Error(t, err, "uninvolved user should not be able to view share")
}

// === Edge Case: User with Write Permission on Shared Collection ===

func TestSharedUserWithWritePermission_CannotShare(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	ctx := context.Background()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	thirdUser := createTestUserWithPermissions(t, s, "third@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	// Owner shares with Write permission
	_, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionWrite)
	require.NoError(t, err)

	// Recipient with Write permission tries to share with third user - should fail
	// (Write permission allows modifying collection content, not sharing it)
	_, err = sharingService.ShareCollection(ctx, recipient.ID, collection.ID, thirdUser.ID, domain.PermissionRead)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only collection owner can share")
}

// === Context Cancellation Tests ===

func TestShareCollection_RespectsContextCancellation(t *testing.T) {
	sharingService, s, cleanup := setupSharingTest(t)
	defer cleanup()

	owner := createTestUserWithPermissions(t, s, "owner@test.com", true)
	recipient := createTestUserWithPermissions(t, s, "recipient@test.com", true)
	library := createTestLibrary(t, s, owner.ID)
	collection := createTestCollection(t, s, owner.ID, library.ID, "My Collection")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := sharingService.ShareCollection(ctx, owner.ID, collection.ID, recipient.ID, domain.PermissionRead)
	assert.Error(t, err)
}
