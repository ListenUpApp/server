package store

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
)

// Store wraps a Badger database instance
type Store struct {
	db     *badger.DB
	logger *slog.Logger
}

// New creates a new Store instance with the given database path
func New(path string, logger *slog.Logger) (*Store, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil // Disable Badger's internal logging

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	store := &Store{
		db:     db,
		logger: logger,
	}

	if logger != nil {
		logger.Info("Badger database opened successfully", "path", path)
	}

	return store, nil
}

// Close gracefully closes the database connection
func (s *Store) Close() error {
	if s.logger != nil {
		s.logger.Info("Closing database connection")
	}
	return s.db.Close()
}

// Helper methods for database operations

// get retrieves a value by key
func (s *Store) get(key []byte, dest interface{}) error {
	return s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, dest)
		})
	})
}

// set stores a value by key
func (s *Store) set(key []byte, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// delete removes a key from the database
func (s *Store) delete(key []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

// exists checks if a key exists
func (s *Store) exists(key []byte) (bool, error) {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})

	if err == badger.ErrKeyNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
