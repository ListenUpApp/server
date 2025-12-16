package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const (
	invitePrefix         = "invite:"
	inviteByCodePrefix   = "idx:invites:code:"   // For public code lookups
	inviteByCreatorPrefix = "idx:invites:creator:" // For listing by admin
)

var (
	// ErrInviteNotFound is returned when an invite cannot be found.
	ErrInviteNotFound = errors.New("invite not found")
	// ErrInviteCodeExists is returned when an invite code already exists.
	ErrInviteCodeExists = errors.New("invite code already exists")
	// ErrInviteExpired is returned when attempting to use an expired invite.
	ErrInviteExpired = errors.New("invite expired")
	// ErrInviteClaimed is returned when attempting to claim an already-claimed invite.
	ErrInviteClaimed = errors.New("invite already claimed")
)

// CreateInvite creates a new invite.
func (s *Store) CreateInvite(_ context.Context, invite *domain.Invite) error {
	key := []byte(invitePrefix + invite.ID)

	// Check if invite ID already exists
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check invite exists: %w", err)
	}
	if exists {
		return fmt.Errorf("invite ID already exists")
	}

	codeKey := []byte(inviteByCodePrefix + invite.Code)
	creatorKey := []byte(inviteByCreatorPrefix + invite.CreatedBy + ":" + invite.ID)

	return s.db.Update(func(txn *badger.Txn) error {
		// Check if code is already in use
		_, err := txn.Get(codeKey)
		if err == nil {
			return ErrInviteCodeExists
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("check code exists: %w", err)
		}

		// Save invite
		data, err := json.Marshal(invite)
		if err != nil {
			return fmt.Errorf("marshal invite: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create code index
		if err := txn.Set(codeKey, []byte(invite.ID)); err != nil {
			return err
		}

		// Create creator index for listing
		if err := txn.Set(creatorKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// GetInvite retrieves an invite by ID.
func (s *Store) GetInvite(_ context.Context, id string) (*domain.Invite, error) {
	key := []byte(invitePrefix + id)

	var invite domain.Invite
	if err := s.get(key, &invite); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrInviteNotFound
		}
		return nil, fmt.Errorf("get invite: %w", err)
	}

	return &invite, nil
}

// GetInviteByCode retrieves an invite by its public code.
// This is the primary lookup method for the public claim flow.
func (s *Store) GetInviteByCode(ctx context.Context, code string) (*domain.Invite, error) {
	codeKey := []byte(inviteByCodePrefix + code)

	// Look up invite ID from code index
	var inviteID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(codeKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			inviteID = string(val)
			return nil
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrInviteNotFound
		}
		return nil, fmt.Errorf("lookup invite by code: %w", err)
	}

	return s.GetInvite(ctx, inviteID)
}

// UpdateInvite updates an existing invite (primarily for claiming).
func (s *Store) UpdateInvite(_ context.Context, invite *domain.Invite) error {
	key := []byte(invitePrefix + invite.ID)

	// Check invite exists
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check invite exists: %w", err)
	}
	if !exists {
		return ErrInviteNotFound
	}

	invite.Touch()

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(invite)
		if err != nil {
			return fmt.Errorf("marshal invite: %w", err)
		}

		return txn.Set(key, data)
	})
}

// DeleteInvite deletes an invite (for revoking unclaimed invites).
func (s *Store) DeleteInvite(_ context.Context, inviteID string) error {
	key := []byte(invitePrefix + inviteID)

	// Get invite data to clean up indices
	var invite domain.Invite
	if err := s.get(key, &invite); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil // Already gone
		}
		return fmt.Errorf("get invite for deletion: %w", err)
	}

	codeKey := []byte(inviteByCodePrefix + invite.Code)
	creatorKey := []byte(inviteByCreatorPrefix + invite.CreatedBy + ":" + inviteID)

	return s.db.Update(func(txn *badger.Txn) error {
		// Delete invite
		if err := txn.Delete(key); err != nil {
			return err
		}

		// Delete code index
		if err := txn.Delete(codeKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		// Delete creator index
		if err := txn.Delete(creatorKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		return nil
	})
}

// ListInvites returns all invites (for admin view).
func (s *Store) ListInvites(ctx context.Context) ([]*domain.Invite, error) {
	prefix := []byte(invitePrefix)
	var invites []*domain.Invite

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var invite domain.Invite
				if unmarshalErr := json.Unmarshal(val, &invite); unmarshalErr != nil {
					// Skip malformed invites
					return nil
				}

				invites = append(invites, &invite)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}

	return invites, nil
}

// ListInvitesByCreator returns all invites created by a specific admin.
func (s *Store) ListInvitesByCreator(ctx context.Context, creatorID string) ([]*domain.Invite, error) {
	prefix := []byte(inviteByCreatorPrefix + creatorID + ":")
	var invites []*domain.Invite

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false // We only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// Extract invite ID from key
			// Key format: idx:invites:creator:creatorID:inviteID
			key := string(it.Item().Key())
			parts := strings.Split(key, ":")
			if len(parts) < 5 {
				continue
			}
			inviteID := parts[4]

			// Get full invite
			invite, err := s.GetInvite(ctx, inviteID)
			if err != nil {
				if errors.Is(err, ErrInviteNotFound) {
					continue // Skip missing invites
				}
				return err
			}

			invites = append(invites, invite)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list invites by creator: %w", err)
	}

	return invites, nil
}
