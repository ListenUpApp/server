package sqlite

import (
	"context"
	"sync"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// sqliteBatchWriter implements store.BatchWriter for SQLite.
// Since SQLite handles transactions internally, this is a thin wrapper
// that delegates to the store's CreateBook method and tracks the count.
type sqliteBatchWriter struct {
	store    *Store
	maxSize  int
	count    int
	mu       sync.Mutex
	canceled bool
}

// NewBatchWriter creates a new batch writer that wraps the store's CreateBook method.
// For SQLite, the maxSize parameter is accepted for interface compatibility
// but does not trigger automatic flushing (each write is immediate).
func (s *Store) NewBatchWriter(maxSize int) store.BatchWriter {
	return &sqliteBatchWriter{
		store:   s,
		maxSize: maxSize,
	}
}

// CreateBook adds a book to the store. For SQLite, this is a direct write.
// Returns an error if the batch has been canceled.
func (bw *sqliteBatchWriter) CreateBook(ctx context.Context, book *domain.Book) error {
	bw.mu.Lock()
	if bw.canceled {
		bw.mu.Unlock()
		return context.Canceled
	}
	bw.mu.Unlock()

	if err := bw.store.CreateBook(ctx, book); err != nil {
		return err
	}

	bw.mu.Lock()
	bw.count++
	bw.mu.Unlock()

	return nil
}

// Flush is a no-op for SQLite since writes are immediate.
// Each CreateBook call writes directly to the database.
func (bw *sqliteBatchWriter) Flush(_ context.Context) error {
	return nil
}

// Cancel marks the batch writer as canceled.
// Subsequent CreateBook calls will return context.Canceled.
func (bw *sqliteBatchWriter) Cancel() {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	bw.canceled = true
}

// Count returns the number of books successfully written so far.
func (bw *sqliteBatchWriter) Count() int {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.count
}
