// Package service provides business logic layer for managing audiobooks, libraries, and synchronization.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// SharingService orchestrates collection sharing operations with ACL enforcement.
type SharingService struct {
	store  store.Store
	logger *slog.Logger
}

// NewSharingService creates a new sharing service.
func NewSharingService(store store.Store, logger *slog.Logger) *SharingService {
	return &SharingService{
		store:  store,
		logger: logger,
	}
}

// ShareCollection shares a collection with another user.
// The requesting user must own the collection AND have share permission.
func (s *SharingService) ShareCollection(ctx context.Context, ownerUserID, collectionID, sharedWithUserID string, permission domain.SharePermission) (*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get the requesting user to check permissions
	requestingUser, err := s.store.GetUser(ctx, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("get requesting user: %w", err)
	}

	// Check if user has share permission
	if !requestingUser.CanShare() {
		return nil, errors.New("user does not have permission to share collections")
	}

	// Verify the requester owns the collection
	canAccess, _, isOwner, err := s.store.CanUserAccessCollection(ctx, ownerUserID, collectionID)
	if err != nil {
		return nil, fmt.Errorf("check collection access: %w", err)
	}
	if !canAccess || !isOwner {
		return nil, fmt.Errorf("only collection owner can share: user %s is not owner of collection %s", ownerUserID, collectionID)
	}

	// Verify the user we're sharing with exists
	if _, err := s.store.GetUser(ctx, sharedWithUserID); err != nil {
		return nil, fmt.Errorf("get user to share with: %w", err)
	}

	// Prevent sharing with self
	if ownerUserID == sharedWithUserID {
		return nil, errors.New("cannot share collection with yourself")
	}

	// Create the share
	share := &domain.CollectionShare{
		CollectionID:     collectionID,
		SharedWithUserID: sharedWithUserID,
		SharedByUserID:   ownerUserID,
		Permission:       permission,
	}

	if err := s.store.CreateShare(ctx, share); err != nil {
		return nil, fmt.Errorf("create share: %w", err)
	}

	s.logger.Info("collection shared",
		"collection_id", collectionID,
		"shared_by", ownerUserID,
		"shared_with", sharedWithUserID,
		"permission", permission.String(),
	)

	return share, nil
}

// UnshareCollection removes a share.
// Only the collection owner or the person who created the share can unshare.
func (s *SharingService) UnshareCollection(ctx context.Context, requestingUserID, shareID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get the share
	share, err := s.store.GetShare(ctx, shareID)
	if err != nil {
		return fmt.Errorf("get share: %w", err)
	}

	// Get the collection to verify ownership
	_, _, isOwner, err := s.store.CanUserAccessCollection(ctx, requestingUserID, share.CollectionID)
	if err != nil {
		return fmt.Errorf("check collection access: %w", err)
	}

	// Only owner or the user who created the share can unshare
	if !isOwner && share.SharedByUserID != requestingUserID {
		return errors.New("only collection owner or share creator can unshare")
	}

	if err := s.store.DeleteShare(ctx, shareID); err != nil {
		return fmt.Errorf("delete share: %w", err)
	}

	s.logger.Info("collection unshared",
		"share_id", shareID,
		"collection_id", share.CollectionID,
		"requesting_user", requestingUserID,
	)

	return nil
}

// UpdateSharePermission updates the permission level of a share.
// Only the collection owner can update permissions.
func (s *SharingService) UpdateSharePermission(ctx context.Context, ownerUserID, shareID string, newPermission domain.SharePermission) (*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get the share
	share, err := s.store.GetShare(ctx, shareID)
	if err != nil {
		return nil, fmt.Errorf("get share: %w", err)
	}

	// Verify the requester owns the collection
	_, _, isOwner, err := s.store.CanUserAccessCollection(ctx, ownerUserID, share.CollectionID)
	if err != nil {
		return nil, fmt.Errorf("check collection access: %w", err)
	}
	if !isOwner {
		return nil, errors.New("only collection owner can update share permissions")
	}

	// Update permission
	share.Permission = newPermission

	if err := s.store.UpdateShare(ctx, share); err != nil {
		return nil, fmt.Errorf("update share: %w", err)
	}

	s.logger.Info("share permission updated",
		"share_id", shareID,
		"collection_id", share.CollectionID,
		"new_permission", newPermission.String(),
	)

	return share, nil
}

// ListCollectionShares returns all shares for a collection.
// Only the collection owner can list shares.
func (s *SharingService) ListCollectionShares(ctx context.Context, ownerUserID, collectionID string) ([]*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Verify the requester owns the collection
	canAccess, _, isOwner, err := s.store.CanUserAccessCollection(ctx, ownerUserID, collectionID)
	if err != nil {
		return nil, fmt.Errorf("check collection access: %w", err)
	}
	if !canAccess || !isOwner {
		return nil, errors.New("only collection owner can list shares")
	}

	shares, err := s.store.GetSharesForCollection(ctx, collectionID)
	if err != nil {
		return nil, fmt.Errorf("get shares for collection: %w", err)
	}

	return shares, nil
}

// ListSharedWithMe returns all collections shared with the user.
// Returns the shares (not the collections themselves).
func (s *SharingService) ListSharedWithMe(ctx context.Context, userID string) ([]*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	shares, err := s.store.GetSharesForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get shares for user: %w", err)
	}

	return shares, nil
}

// GetShare retrieves a share by ID.
// User must be involved in the share (owner, sharer, or sharee).
func (s *SharingService) GetShare(ctx context.Context, requestingUserID, shareID string) (*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	share, err := s.store.GetShare(ctx, shareID)
	if err != nil {
		return nil, err
	}

	// Verify requester is involved in this share
	_, _, isOwner, err := s.store.CanUserAccessCollection(ctx, requestingUserID, share.CollectionID)
	if err != nil {
		return nil, fmt.Errorf("check collection access: %w", err)
	}

	// Requester must be: owner, the person shared with, or the person who shared
	if !isOwner && share.SharedWithUserID != requestingUserID && share.SharedByUserID != requestingUserID {
		return nil, errors.New("access denied: user not involved in this share")
	}

	return share, nil
}
