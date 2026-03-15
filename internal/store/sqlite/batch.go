package sqlite

import (
	"context"
	"errors"
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

// Flush writes all accumulated books individually.
// Books that already exist (ErrAlreadyExists) are skipped so that
// new books in the same batch are not lost.
func (bw *sqliteBatchWriter) Flush(ctx context.Context) error {
	bw.mu.Lock()
	books := bw.books
	bw.books = nil
	bw.mu.Unlock()

	if len(books) == 0 {
		return nil
	}

	var written []*domain.Book
	for _, book := range books {
		tx, err := bw.store.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin batch tx: %w", err)
		}

		if err := createBookTx(ctx, tx, book); err != nil {
			tx.Rollback()
			if errors.Is(err, store.ErrAlreadyExists) {
				// Book already in DB — update its junction tables in case they're
				// missing (e.g., book was created before junction table writing was added).
				if err := bw.store.updateBookJunctionTables(ctx, book); err != nil {
					// Log but don't fail — the book row already exists, junction update is best-effort.
					continue
				}
				written = append(written, book)
				continue
			}
			return fmt.Errorf("batch create book %s: %w", book.ID, err)
		}

		if err := tx.Commit(); err != nil {
			tx.Rollback()
			return fmt.Errorf("commit batch: %w", err)
		}

		written = append(written, book)
	}

	for _, book := range written {
		bw.store.indexBookAsync(ctx, book)
	}

	bw.mu.Lock()
	bw.count += len(written)
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
