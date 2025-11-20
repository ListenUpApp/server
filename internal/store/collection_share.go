package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
)

var (
	// ErrShareNotFound is returned when a collection share is not found.
	ErrShareNotFound = errors.New("share not found")
	// ErrShareAlreadyExists is returned when trying to create a duplicate share.
	ErrShareAlreadyExists = errors.New("share already exists for this user and collection")
)

// CreateShare creates a new collection share.
func (s *Store) CreateShare(ctx context.Context, share *domain.CollectionShare) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Generate ID if not provided
	if share.ID == "" {
		shareID, err := id.Generate("share")
		if err != nil {
			return fmt.Errorf("generate share ID: %w", err)
		}
		share.ID = shareID
	}

	// Check if share already exists for this user and collection
	existing, err := s.GetShareForUserAndCollection(ctx, share.SharedWithUserID, share.CollectionID)
	if err != nil && !errors.Is(err, ErrShareNotFound) {
		return fmt.Errorf("check existing share: %w", err)
	}
	if existing != nil {
		return ErrShareAlreadyExists
	}

	// Create the share
	if err := s.CollectionShares.Create(ctx, share.ID, share); err != nil {
		return fmt.Errorf("create share: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("collection share created",
			"share_id", share.ID,
			"collection_id", share.CollectionID,
			"shared_with", share.SharedWithUserID,
			"permission", share.Permission.String(),
		)
	}

	return nil
}

// GetShare retrieves a share by ID.
func (s *Store) GetShare(ctx context.Context, shareID string) (*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	share, err := s.CollectionShares.Get(ctx, shareID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrShareNotFound
		}
		return nil, fmt.Errorf("get share: %w", err)
	}

	return share, nil
}

// GetShareForUserAndCollection finds a share for a specific user and collection.
func (s *Store) GetShareForUserAndCollection(ctx context.Context, userID, collectionID string) (*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get all shares for this user
	shares, err := s.GetSharesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Find the one for this collection
	for _, share := range shares {
		if share.CollectionID == collectionID {
			return share, nil
		}
	}

	return nil, ErrShareNotFound
}

// GetSharesForUser returns all shares for a given user.
func (s *Store) GetSharesForUser(ctx context.Context, userID string) ([]*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	shares, err := s.CollectionShares.FindByIndex(ctx, "user", userID)
	if err != nil {
		return nil, fmt.Errorf("find shares by user: %w", err)
	}

	return shares, nil
}

// GetSharesForCollection returns all shares for a given collection.
func (s *Store) GetSharesForCollection(ctx context.Context, collectionID string) ([]*domain.CollectionShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	shares, err := s.CollectionShares.FindByIndex(ctx, "collection", collectionID)
	if err != nil {
		return nil, fmt.Errorf("find shares by collection: %w", err)
	}

	return shares, nil
}

// DeleteShare deletes a share.
func (s *Store) DeleteShare(ctx context.Context, shareID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Verify share exists first
	_, err := s.GetShare(ctx, shareID)
	if err != nil {
		return err
	}

	if err := s.CollectionShares.Delete(ctx, shareID); err != nil {
		return fmt.Errorf("delete share: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("collection share deleted", "share_id", shareID)
	}

	return nil
}

// UpdateShare updates an existing share.
func (s *Store) UpdateShare(ctx context.Context, share *domain.CollectionShare) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Verify share exists
	_, err := s.GetShare(ctx, share.ID)
	if err != nil {
		return err
	}

	if err := s.CollectionShares.Update(ctx, share.ID, share); err != nil {
		return fmt.Errorf("update share: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("collection share updated",
			"share_id", share.ID,
			"permission", share.Permission.String(),
		)
	}

	return nil
}

// DeleteSharesForCollection deletes all shares for a collection.
// Used when deleting a collection to clean up associated shares.
func (s *Store) DeleteSharesForCollection(ctx context.Context, collectionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	shares, err := s.GetSharesForCollection(ctx, collectionID)
	if err != nil {
		return fmt.Errorf("get shares for collection: %w", err)
	}

	for _, share := range shares {
		if err := s.DeleteShare(ctx, share.ID); err != nil {
			return fmt.Errorf("delete share %s: %w", share.ID, err)
		}
	}

	if s.logger != nil && len(shares) > 0 {
		s.logger.Info("deleted shares for collection",
			"collection_id", collectionID,
			"count", len(shares),
		)
	}

	return nil
}
