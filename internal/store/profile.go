package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const profilePrefix = "profile:"

// ErrProfileNotFound is returned when a user profile is not found.
var ErrProfileNotFound = ErrNotFound.WithMessage("profile not found")

// GetUserProfile retrieves a user's profile.
// Returns ErrProfileNotFound if no profile exists.
func (s *Store) GetUserProfile(ctx context.Context, userID string) (*domain.UserProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := profilePrefix + userID
	var profile domain.UserProfile

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrProfileNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &profile)
		})
	})

	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// SaveUserProfile creates or updates a user's profile.
func (s *Store) SaveUserProfile(ctx context.Context, profile *domain.UserProfile) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := profilePrefix + profile.UserID
	data, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

// DeleteUserProfile removes a user's profile.
func (s *Store) DeleteUserProfile(ctx context.Context, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := profilePrefix + userID
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}
