package store

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// BatchWriter provides efficient bulk write operations using BadgerDB's WriteBatch
type BatchWriter struct {
	store     *Store
	batch     *badger.WriteBatch
	maxSize   int
	count     int
	autoFlush bool
}

// NewBatchWriter creates a new batch writer that will auto-flush when maxSize is reached
func (s *Store) NewBatchWriter(maxSize int) *BatchWriter {
	return &BatchWriter{
		store:     s,
		batch:     s.db.NewWriteBatch(),
		maxSize:   maxSize,
		autoFlush: true,
	}
}

// CreateBook adds a book to the batch
// If autoFlush is enabled and batch reaches maxSize, it will flush automatically
func (b *BatchWriter) CreateBook(ctx context.Context, book *domain.Book) error {
	// Marshal book data
	data, err := json.Marshal(book)
	if err != nil {
		return fmt.Errorf("marshal book: %w", err)
	}

	// Add book to batch
	key := []byte(bookPrefix + book.ID)
	if err := b.batch.Set(key, data); err != nil {
		return fmt.Errorf("batch set book: %w", err)
	}

	// Add path index
	pathKey := []byte(bookByPathPrefix + book.Path)
	if err := b.batch.Set(pathKey, []byte(book.ID)); err != nil {
		return fmt.Errorf("batch set path index: %w", err)
	}

	// Add inode indices for each audio file
	for _, audioFile := range book.AudioFiles {
		if audioFile.Inode > 0 {
			inodeKey := []byte(fmt.Sprintf("%s%d", bookByInodePrefix, audioFile.Inode))
			if err := b.batch.Set(inodeKey, []byte(book.ID)); err != nil {
				return fmt.Errorf("batch set inode index: %w", err)
			}
		}
	}

	b.count++

	// Auto-flush if batch is full
	if b.autoFlush && b.count >= b.maxSize {
		if err := b.Flush(); err != nil {
			return fmt.Errorf("auto flush: %w", err)
		}
	}

	return nil
}

// Flush commits all pending writes in the batch
func (b *BatchWriter) Flush() error {
	if b.count == 0 {
		return nil // Nothing to flush
	}

	if err := b.batch.Flush(); err != nil {
		return fmt.Errorf("flush batch: %w", err)
	}

	if b.store.logger != nil {
		b.store.logger.LogAttrs(context.Background(), slog.LevelInfo, "batch flushed",
			slog.Int("count", b.count),
		)
	}

	// Reset for next batch
	b.count = 0
	b.batch = b.store.db.NewWriteBatch()

	return nil
}

// Cancel discards all pending writes in the batch
func (b *BatchWriter) Cancel() {
	b.batch.Cancel()
	b.count = 0
}

// Count returns the number of operations in the current batch
func (b *BatchWriter) Count() int {
	return b.count
}
