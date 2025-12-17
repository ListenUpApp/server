package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const userSettingsPrefix = "settings:"

// ErrUserSettingsNotFound is returned when user settings are not found.
var ErrUserSettingsNotFound = ErrNotFound.WithMessage("user settings not found")

// GetUserSettings retrieves settings for a user.
func (s *Store) GetUserSettings(ctx context.Context, userID string) (*domain.UserSettings, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := userSettingsPrefix + userID
	var settings domain.UserSettings

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrUserSettingsNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &settings)
		})
	})

	if err != nil {
		return nil, err
	}
	return &settings, nil
}

// UpsertUserSettings creates or updates user settings.
func (s *Store) UpsertUserSettings(ctx context.Context, settings *domain.UserSettings) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := userSettingsPrefix + settings.UserID
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

// DeleteUserSettings removes user settings.
func (s *Store) DeleteUserSettings(ctx context.Context, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := userSettingsPrefix + userID
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// GetOrCreateUserSettings retrieves settings or creates defaults if not found.
func (s *Store) GetOrCreateUserSettings(ctx context.Context, userID string) (*domain.UserSettings, error) {
	settings, err := s.GetUserSettings(ctx, userID)
	if err == nil {
		return settings, nil
	}

	if !errors.Is(err, ErrUserSettingsNotFound) {
		return nil, err
	}

	// Create defaults
	settings = domain.NewUserSettings(userID)
	if err := s.UpsertUserSettings(ctx, settings); err != nil {
		return nil, err
	}
	return settings, nil
}
