package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
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

// SearchIndexer is the interface for updating the search index.
// Store uses this to keep search in sync without depending on search implementation.
// Index updates are performed asynchronously to not block store operations.
type SearchIndexer interface {
	IndexBook(ctx context.Context, book *domain.Book) error
	DeleteBook(ctx context.Context, bookID string) error
	IndexContributor(ctx context.Context, c *domain.Contributor) error
	DeleteContributor(ctx context.Context, contributorID string) error
	IndexSeries(ctx context.Context, s *domain.Series) error
	DeleteSeries(ctx context.Context, seriesID string) error
}

// NoopSearchIndexer is a no-op implementation for testing.
type NoopSearchIndexer struct{}

// IndexBook is a no-op.
func (NoopSearchIndexer) IndexBook(context.Context, *domain.Book) error { return nil }

// DeleteBook is a no-op.
func (NoopSearchIndexer) DeleteBook(context.Context, string) error { return nil }

// IndexContributor is a no-op.
func (NoopSearchIndexer) IndexContributor(context.Context, *domain.Contributor) error { return nil }

// DeleteContributor is a no-op.
func (NoopSearchIndexer) DeleteContributor(context.Context, string) error { return nil }

// IndexSeries is a no-op.
func (NoopSearchIndexer) IndexSeries(context.Context, *domain.Series) error { return nil }

// DeleteSeries is a no-op.
func (NoopSearchIndexer) DeleteSeries(context.Context, string) error { return nil }

// NewNoopSearchIndexer creates a new no-op search indexer for testing.
func NewNoopSearchIndexer() SearchIndexer {
	return NoopSearchIndexer{}
}

// Store wraps a Badger database instance.
type Store struct {
	db     *badger.DB
	logger *slog.Logger

	// SSE event emitter for broadcasting changes.
	eventEmitter EventEmitter

	// Search indexer for keeping search in sync with store changes.
	// Set via SetSearchIndexer after store creation to avoid circular dependencies.
	searchIndexer SearchIndexer

	// Enricher for denormalizing domain models before sending to clients.
	// Used to populate display fields (author names, series names) in SSE events.
	enricher *dto.Enricher

	// Generic entities
	Users            *Entity[domain.User]
	CollectionShares *Entity[domain.CollectionShare]
}

// New creates a new Store instance with the given database path and event emitter.
// The emitter is required and used to broadcast store changes via SSE.
func New(path string, logger *slog.Logger, emitter EventEmitter) (*Store, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil            // Disable Badger's internal logging
	opts.SyncWrites = true       // Ensure writes are synced to disk to prevent corruption on crashes
	opts.CompactL0OnClose = true // Compact L0 tables on close for faster startup

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	store := &Store{
		db:           db,
		logger:       logger,
		eventEmitter: emitter,
	}

	// Initialize enricher for SSE event denormalization.
	// Store implements dto.Store interface (GetContributorsByIDs, GetSeries).
	store.enricher = dto.NewEnricher(store)

	// Initialize generic entities
	store.initUsers()
	store.initCollectionShares()

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

// SetSearchIndexer sets the search indexer for keeping search in sync.
// This is set after store creation to avoid circular dependencies
// (store needs to exist before search service can be created).
func (s *Store) SetSearchIndexer(indexer SearchIndexer) {
	s.searchIndexer = indexer
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

// initUsers initializes the Users entity on the store.
// Uses case-insensitive email indexing via normalizeEmail transformation.
func (s *Store) initUsers() {
	s.Users = NewEntity[domain.User](s, "user:").
		WithIndexTransform("email",
			func(u *domain.User) []string {
				return []string{normalizeEmail(u.Email)}
			},
			normalizeEmail, // Transform lookups to be case-insensitive
		)
}

// initCollectionShares initializes the CollectionShares entity on the store.
// Indexes by user (for finding all shares a user has) and collection (for finding all shares of a collection).
func (s *Store) initCollectionShares() {
	s.CollectionShares = NewEntity[domain.CollectionShare](s, "share:").
		WithIndex("user", func(cs *domain.CollectionShare) []string {
			return []string{cs.SharedWithUserID}
		}).
		WithIndex("collection", func(cs *domain.CollectionShare) []string {
			return []string{cs.CollectionID}
		})
}
