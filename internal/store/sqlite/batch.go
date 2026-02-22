package sqlite

import (
	"context"
	"fmt"
	"sync"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// sqliteBatchWriter implements store.BatchWriter for SQLite.
// It accumulates books and writes them all in a single transaction on Flush.
type sqliteBatchWriter struct {
	store    *Store
	maxSize  int
	books    []*domain.Book
	count    int
	mu       sync.Mutex
	canceled bool
}

// NewBatchWriter creates a new batch writer that accumulates books
// and commits them atomically in Flush.
func (s *Store) NewBatchWriter(maxSize int) store.BatchWriter {
	return &sqliteBatchWriter{
		store:   s,
		maxSize: maxSize,
	}
}

// CreateBook accumulates a book for the batch.
// Returns an error if the batch has been canceled.
func (bw *sqliteBatchWriter) CreateBook(_ context.Context, book *domain.Book) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if bw.canceled {
		return context.Canceled
	}

	bw.books = append(bw.books, book)
	return nil
}

// Flush writes all accumulated books in a single transaction.
// If any book fails, the entire batch is rolled back.
func (bw *sqliteBatchWriter) Flush(ctx context.Context) error {
	bw.mu.Lock()
	books := bw.books
	bw.books = nil
	bw.mu.Unlock()

	if len(books) == 0 {
		return nil
	}

	tx, err := bw.store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin batch tx: %w", err)
	}
	defer tx.Rollback()

	for _, book := range books {
		if err := createBookTx(ctx, tx, book); err != nil {
			return fmt.Errorf("batch create book %s: %w", book.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}

	bw.mu.Lock()
	bw.count += len(books)
	bw.mu.Unlock()

	return nil
}

// Cancel marks the batch writer as canceled.
// Subsequent CreateBook calls will return context.Canceled.
func (bw *sqliteBatchWriter) Cancel() {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	bw.canceled = true
	bw.books = nil
}

// Count returns the number of books successfully flushed.
func (bw *sqliteBatchWriter) Count() int {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.count
}
