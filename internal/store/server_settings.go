package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const keyServerSettings = "settings:server"

// GetServerSettings retrieves server-wide settings.
// Returns default settings if none exist.
func (s *Store) GetServerSettings(ctx context.Context) (*domain.ServerSettings, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var settings domain.ServerSettings

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(keyServerSettings))
		if errors.Is(err, badger.ErrKeyNotFound) {
			// Return defaults if not set
			settings = *domain.NewServerSettings()
			return nil
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

// UpdateServerSettings updates server-wide settings.
func (s *Store) UpdateServerSettings(ctx context.Context, settings *domain.ServerSettings) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal server settings: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(keyServerSettings), data)
	})
}
