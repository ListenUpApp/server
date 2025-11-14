package store

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
)

// EventEmitter is the interface for emitting SSE events.
// Store uses this to broadcast changes without depending on SSE implementation details.
type EventEmitter interface {
	Emit(event any)
}

// NoopEmitter is a no-op implementation of EventEmitter for testing.
type NoopEmitter struct{}

// Emit implements EventEmitter.Emit as a no-op.
func (NoopEmitter) Emit(_ any) {}

// NewNoopEmitter creates a new no-op emitter for testing.
func NewNoopEmitter() EventEmitter {
	return NoopEmitter{}
}

// Store wraps a Badger database instance.
type Store struct {
	db     *badger.DB
	logger *slog.Logger

	// SSE event emitter for broadcasting changes.
	eventEmitter EventEmitter
}

// New creates a new Store instance with the given database path and event emitter.
// The emitter is required and used to broadcast store changes via SSE.
func New(path string, logger *slog.Logger, emitter EventEmitter) (*Store, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil // Disable Badger's internal logging

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	store := &Store{
		db:           db,
		logger:       logger,
		eventEmitter: emitter,
	}

	if logger != nil {
		logger.Info("Badger database opened successfully", "path", path)
	}

	return store, nil
}

// Close gracefully closes the database connection.
func (s *Store) Close() error {
	if s.logger != nil {
		s.logger.Info("Closing database connection")
	}
	return s.db.Close()
}

// Helper methods for database operations.

// get retrieves a value by key.
func (s *Store) get(key []byte, dest any) error {
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

// set stores a value by key.
func (s *Store) set(key []byte, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// delete removes a key from the database.
func (s *Store) delete(key []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

// exists checks if a key exists.
func (s *Store) exists(key []byte) (bool, error) {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})

	if errors.Is(err, badger.ErrKeyNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
