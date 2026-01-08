package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/sse"
)

const (
	bookPrefix              = "book:"
	bookByPathPrefix        = "idx:books:path:"
	bookByInodePrefix       = "idx:books:inode"
	bookByUpdatedAtPrefix   = "idx:books:updated_at:"  // Format: idx:books:updated_at:{RFC3339Nano}:book:{uuid}
	bookByDeletedAtPrefix   = "idx:books:deleted_at:"  // Format: idx:books:deleted_at:{RFC3339Nano}:book:{uuid}
	bookByContributorPrefix = "idx:books:contributor:" // Format: idx:books:contributor:{contributorID}:{bookID}
	bookBySeriesPrefix      = "idx:books:series:"      // Format: idx:books:series:{seriesID}:{bookID}
)

var (
	// ErrBookNotFound is returned when a book is not found in the store.
	ErrBookNotFound = errors.New("book not found")
	// ErrBookExists is returned when trying to create a book that already exists.
	ErrBookExists = errors.New("book already exists")
)

// stringSetDiff computes the difference between two string slices.
// Returns (added, removed) where:
//   - added contains elements in newItems but not in oldItems
//   - removed contains elements in oldItems but not in newItems
func stringSetDiff(oldItems, newItems []string) (added, removed []string) {
	oldSet := make(map[string]bool, len(oldItems))
	for _, item := range oldItems {
		oldSet[item] = true
	}

	newSet := make(map[string]bool, len(newItems))
	for _, item := range newItems {
		newSet[item] = true
		if !oldSet[item] {
			added = append(added, item)
		}
	}

	for _, item := range oldItems {
		if !newSet[item] {
			removed = append(removed, item)
		}
	}

	return added, removed
}

// uint64SetDiff computes the difference between two uint64 slices.
// Returns (added, removed) where:
//   - added contains elements in newItems but not in oldItems
//   - removed contains elements in oldItems but not in newItems
func uint64SetDiff(oldItems, newItems []uint64) (added, removed []uint64) {
	oldSet := make(map[uint64]bool, len(oldItems))
	for _, item := range oldItems {
		oldSet[item] = true
	}

	newSet := make(map[uint64]bool, len(newItems))
	for _, item := range newItems {
		newSet[item] = true
		if !oldSet[item] {
			added = append(added, item)
		}
	}

	for _, item := range oldItems {
		if !newSet[item] {
			removed = append(removed, item)
		}
	}

	return added, removed
}

// Book Operations.

// CreateBook creates a new book.
func (s *Store) CreateBook(ctx context.Context, book *domain.Book) error {
	key := []byte(bookPrefix + book.ID)

	// Check if it already exists.
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check book exists: %w", err)
	}
	if exists {
		return ErrBookExists
	}

	// Use transaction to create book indicies atomically.
	err = s.db.Update(func(txn *badger.Txn) error {
		// Save book.
		data, err := json.Marshal(book)
		if err != nil {
			return fmt.Errorf("marshal book: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create path index.
		pathKey := []byte(bookByPathPrefix + book.Path)
		if err := txn.Set(pathKey, []byte(book.ID)); err != nil {
			return err
		}

		// Create inode indices for eac haudio file (for fast file watching lookups).
		for _, audioFile := range book.AudioFiles {
			if audioFile.Inode > 0 {
				inodeKey := fmt.Appendf(nil, "%s%d", bookByInodePrefix, audioFile.Inode)
				if err := txn.Set(inodeKey, []byte(book.ID)); err != nil {
					return err
				}
			}
		}

		// Create contributor reverse indexes for efficient contributor -> books lookups.
		for _, bc := range book.Contributors {
			contributorBookKey := fmt.Appendf(nil, "%s%s:%s", bookByContributorPrefix, bc.ContributorID, book.ID)
			if err := txn.Set(contributorBookKey, []byte{}); err != nil {
				return err
			}
		}

		// Create series reverse indexes for all series the book belongs to.
		for _, bs := range book.Series {
			seriesBookKey := fmt.Appendf(nil, "%s%s:%s", bookBySeriesPrefix, bs.SeriesID, book.ID)
			if err := txn.Set(seriesBookKey, []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create book: %w", err)
	}

	if s.logger != nil {
		s.logger.LogAttrs(ctx, slog.LevelInfo, "book created",
			slog.String("id", book.ID),
			slog.String("title", book.Title),
			slog.String("path", book.Path),
			slog.Int("audio_files", len(book.AudioFiles)),
		)
	}

	// Index for search asynchronously with timeout.
	// Use a detached context with timeout rather than the request context
	// (which may be canceled early) or Background (which never times out).
	if s.searchIndexer != nil {
		go func() {
			indexCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := s.searchIndexer.IndexBook(indexCtx, book); err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to index book for search",
						slog.String("book_id", book.ID),
						slog.Any("error", err))
				}
			}
		}()
	}

	return nil
}

// BroadcastBookCreated enriches and broadcasts a book.created SSE event.
// Should be called AFTER cover extraction to ensure cover is available when clients receive event.
func (s *Store) BroadcastBookCreated(ctx context.Context, book *domain.Book) error {
	// Enrich book with denormalized fields before broadcasting SSE event.
	// This ensures clients receive immediately-renderable data (author names, etc.)
	// without waiting for separate contributor events or making additional queries.
	if s.logger != nil {
		s.logger.Debug("enriching book for SSE",
			"book_id", book.ID,
			"contributors_count", len(book.Contributors),
		)
	}

	enrichedBook, err := s.enricher.EnrichBook(ctx, book)
	if err != nil {
		// Don't fail broadcasting if enrichment fails.
		// Log warning and send un-enriched book (author will be empty string).
		if s.logger != nil {
			s.logger.Warn("failed to enrich book for SSE event",
				"book_id", book.ID,
				"error", err,
			)
		}
		// Fallback: wrap domain.Book in dto.Book without enrichment
		enrichedBook = &dto.Book{Book: book}
	} else if s.logger != nil {
		s.logger.Debug("enrichment complete",
			"book_id", book.ID,
			"author", enrichedBook.Author,
			"narrator", enrichedBook.Narrator,
		)
	}

	s.eventEmitter.Emit(sse.NewBookCreatedEvent(enrichedBook))
	return nil
}

// GetBook retrieves a book by ID.
// GetBook retrieves a book by ID with access control.
// User must be able to access the book via collections or it must be uncollected.
// Returns ErrBookNotFound if book doesn't exist OR user doesn't have access.
func (s *Store) GetBook(ctx context.Context, id string, userID string) (*domain.Book, error) {
	// Get the book without access checks first
	book, err := s.getBookInternal(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check if user can access this book
	canAccess, err := s.CanUserAccessBook(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		// Don't leak that book exists
		return nil, ErrBookNotFound
	}

	return book, nil
}

// GetBookNoAccessCheck retrieves a book by ID without access control.
// Use for system-level operations like search indexing where user context isn't available.
func (s *Store) GetBookNoAccessCheck(ctx context.Context, id string) (*domain.Book, error) {
	return s.getBookInternal(ctx, id)
}

// getBookInternal retrieves a book by ID without access control.
// For internal store use only.
func (s *Store) getBookInternal(_ context.Context, id string) (*domain.Book, error) {
	key := []byte(bookPrefix + id)

	var book domain.Book
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &book)
		})
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrBookNotFound
		}
		return nil, fmt.Errorf("get book: %w", err)
	}

	// Treat soft-deleted books as not found.
	if book.IsDeleted() {
		return nil, ErrBookNotFound
	}

	return &book, nil
}

// getBooksInternalByIDs retrieves multiple books by ID in a single transaction.
// Skips books that are not found or soft-deleted.
// For internal store use only.
func (s *Store) getBooksInternalByIDs(_ context.Context, ids []string) ([]*domain.Book, error) {
	if len(ids) == 0 {
		return []*domain.Book{}, nil
	}

	books := make([]*domain.Book, 0, len(ids))

	err := s.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			key := []byte(bookPrefix + id)
			item, err := txn.Get(key)
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue // Skip not found
				}
				return err
			}

			var book domain.Book
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &book)
			}); err != nil {
				continue // Skip malformed entries
			}

			if book.IsDeleted() {
				continue // Skip soft-deleted
			}

			books = append(books, &book)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get books by ids: %w", err)
	}

	return books, nil
}

// GetBookByPath retrieves a book by its filesystem path.
// This is used during file watching to check if a book already exists.
// No access control - for internal system use only.
func (s *Store) GetBookByPath(ctx context.Context, path string) (*domain.Book, error) {
	pathKey := []byte(bookByPathPrefix + path)

	var bookID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(pathKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			bookID = string(val)
			return nil
		})
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrBookNotFound
		}
		return nil, fmt.Errorf("get book by path: %w", err)
	}
	return s.getBookInternal(ctx, bookID)
}

// GetBookByInode retrieves a book by an audio file inode.
// This is used during file watching for fast lookups when a file changes.
// No access control - for internal system use only.
func (s *Store) GetBookByInode(ctx context.Context, inode int64) (*domain.Book, error) {
	inodeKey := fmt.Appendf(nil, "%s%d", bookByInodePrefix, inode)

	var bookID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(inodeKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			bookID = string(val)
			return nil
		})
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrBookNotFound
		}
		return nil, fmt.Errorf("get book by inode: %w", err)
	}
	return s.getBookInternal(ctx, bookID)
}

// UpdateBook updates an existing book.
func (s *Store) UpdateBook(ctx context.Context, book *domain.Book) error {
	key := []byte(bookPrefix + book.ID)

	// Get old book for index updates.
	oldBook, err := s.getBookInternal(ctx, book.ID)
	if err != nil {
		return err
	}

	book.Touch()

	// Use Transaction to update book and indices atomically.
	err = s.db.Update(func(txn *badger.Txn) error {
		// Update book.
		data, err := json.Marshal(book)
		if err != nil {
			return fmt.Errorf("marshal book: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update path index if path changes.
		if oldBook.Path != book.Path {
			// Delete old path index.
			oldPathKey := []byte(bookByPathPrefix + oldBook.Path)
			if err := txn.Delete(oldPathKey); err != nil {
				return err
			}

			// Create new path index.
			newPathKey := []byte(bookByPathPrefix + book.Path)
			if err := txn.Set(newPathKey, []byte(book.ID)); err != nil {
				return err
			}
		}

		// Update inode indices using set diff.
		oldInodes := make([]uint64, 0, len(oldBook.AudioFiles))
		for _, af := range oldBook.AudioFiles {
			if af.Inode > 0 {
				oldInodes = append(oldInodes, af.Inode)
			}
		}
		newInodes := make([]uint64, 0, len(book.AudioFiles))
		for _, af := range book.AudioFiles {
			if af.Inode > 0 {
				newInodes = append(newInodes, af.Inode)
			}
		}

		addedInodes, removedInodes := uint64SetDiff(oldInodes, newInodes)

		for _, inode := range removedInodes {
			inodeKey := fmt.Appendf(nil, "%s%d", bookByInodePrefix, inode)
			if err := txn.Delete(inodeKey); err != nil {
				return err
			}
		}
		for _, inode := range addedInodes {
			inodeKey := fmt.Appendf(nil, "%s%d", bookByInodePrefix, inode)
			if err := txn.Set(inodeKey, []byte(book.ID)); err != nil {
				return err
			}
		}

		// Update contributor reverse indexes using set diff.
		oldContributorIDs := make([]string, len(oldBook.Contributors))
		for i, bc := range oldBook.Contributors {
			oldContributorIDs[i] = bc.ContributorID
		}
		newContributorIDs := make([]string, len(book.Contributors))
		for i, bc := range book.Contributors {
			newContributorIDs[i] = bc.ContributorID
		}

		addedContributors, removedContributors := stringSetDiff(oldContributorIDs, newContributorIDs)

		for _, contributorID := range removedContributors {
			contributorBookKey := fmt.Appendf(nil, "%s%s:%s", bookByContributorPrefix, contributorID, book.ID)
			if err := txn.Delete(contributorBookKey); err != nil {
				return err
			}
		}
		for _, contributorID := range addedContributors {
			contributorBookKey := fmt.Appendf(nil, "%s%s:%s", bookByContributorPrefix, contributorID, book.ID)
			if err := txn.Set(contributorBookKey, []byte{}); err != nil {
				return err
			}
		}

		// Update series reverse indexes using set diff.
		oldSeriesIDs := make([]string, len(oldBook.Series))
		for i, bs := range oldBook.Series {
			oldSeriesIDs[i] = bs.SeriesID
		}
		newSeriesIDs := make([]string, len(book.Series))
		for i, bs := range book.Series {
			newSeriesIDs[i] = bs.SeriesID
		}

		addedSeries, removedSeries := stringSetDiff(oldSeriesIDs, newSeriesIDs)

		for _, seriesID := range removedSeries {
			seriesBookKey := fmt.Appendf(nil, "%s%s:%s", bookBySeriesPrefix, seriesID, book.ID)
			if err := txn.Delete(seriesBookKey); err != nil {
				return err
			}
		}
		for _, seriesID := range addedSeries {
			seriesBookKey := fmt.Appendf(nil, "%s%s:%s", bookBySeriesPrefix, seriesID, book.ID)
			if err := txn.Set(seriesBookKey, []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("update book: %w", err)
	}

	if err := domain.CascadeBookUpdate(ctx, s, book.ID); err != nil {
		// Log but don't fail the update.
		if s.logger != nil {
			s.logger.Error("cascaded update failed", "book_id", book.ID, "error", err)
		}
	}

	if s.logger != nil {
		s.logger.Info("book updated", "id", book.ID, "title", book.Title)
	}

	// Enrich book with denormalized fields before broadcasting SSE event.
	// This ensures clients receive immediately-renderable data (author names, etc.)
	// without waiting for separate contributor events or making additional queries.
	enrichedBook, err := s.enricher.EnrichBook(ctx, book)
	if err != nil {
		// Don't fail book update if enrichment fails.
		// Log warning and send un-enriched book (author will be empty string).
		if s.logger != nil {
			s.logger.Warn("failed to enrich book for SSE event",
				"book_id", book.ID,
				"error", err,
			)
		}
		// Fallback: wrap domain.Book in dto.Book without enrichment
		enrichedBook = &dto.Book{Book: book}
	}

	s.eventEmitter.Emit(sse.NewBookUpdatedEvent(enrichedBook))

	// Reindex for search asynchronously with timeout
	if s.searchIndexer != nil {
		go func() {
			indexCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := s.searchIndexer.IndexBook(indexCtx, book); err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to reindex book for search", "book_id", book.ID, "error", err)
				}
			}
		}()
	}

	return nil
}

// DeleteBook deletes a book and removes it from all collections.
// This is a system operation that bypasses ACL checks.
func (s *Store) DeleteBook(ctx context.Context, id string) error {
	book, err := s.getBookInternal(ctx, id)
	if err != nil {
		return err
	}

	// Remove from all collections first.
	collections, err := s.GetCollectionsForBook(ctx, id)
	if err != nil {
		return fmt.Errorf("get collections for book: %w", err)
	}

	for _, coll := range collections {
		if err := s.removeBookFromCollectionInternal(ctx, coll.ID, id); err != nil {
			return fmt.Errorf("remove from collection %s: %w", coll.ID, err)
		}
	}

	// Soft delete the book.
	err = s.db.Update(func(txn *badger.Txn) error {
		// Mark book as deleted (sets DeletedAt and updates UpdatedAt).
		oldUpdatedAt := book.UpdatedAt
		book.MarkDeleted()

		// Save updated book with DeletedAt set.
		key := []byte(bookPrefix + id)
		data, err := json.Marshal(book)
		if err != nil {
			return fmt.Errorf("marshal book: %w", err)
		}
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Delete secondary indexes (path, inode) - deleted books shouldn't be found by these.
		pathKey := []byte(bookByPathPrefix + book.Path)
		if err := txn.Delete(pathKey); err != nil {
			return err
		}

		for _, audioFile := range book.AudioFiles {
			if audioFile.Inode > 0 {
				inodeKey := fmt.Appendf(nil, "%s%d", bookByInodePrefix, audioFile.Inode)
				if err := txn.Delete(inodeKey); err != nil {
					return err
				}
			}
		}

		// Delete contributor reverse indexes.
		for _, bc := range book.Contributors {
			contributorBookKey := fmt.Appendf(nil, "%s%s:%s", bookByContributorPrefix, bc.ContributorID, book.ID)
			if err := txn.Delete(contributorBookKey); err != nil {
				return err
			}
		}

		// Delete series reverse indexes for all series the book was in.
		for _, bs := range book.Series {
			seriesBookKey := fmt.Appendf(nil, "%s%s:%s", bookBySeriesPrefix, bs.SeriesID, book.ID)
			if err := txn.Delete(seriesBookKey); err != nil {
				return err
			}
		}

		// Update updated_at index (deleted books still appear in delta queries).
		oldUpdatedAtKey := formatTimestampIndexKey(bookByUpdatedAtPrefix, oldUpdatedAt, "book", book.ID)
		if err := txn.Delete(oldUpdatedAtKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		newUpdatedAtKey := formatTimestampIndexKey(bookByUpdatedAtPrefix, book.UpdatedAt, "book", book.ID)
		if err := txn.Set(newUpdatedAtKey, []byte{}); err != nil {
			return err
		}

		// Create deleted_at index.
		deletedAtKey := formatTimestampIndexKey(bookByDeletedAtPrefix, *book.DeletedAt, "book", book.ID)
		if err := txn.Set(deletedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("soft delete book: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("book soft deleted", "id", id, "title", book.Title, "deleted_at", book.DeletedAt)
	}

	s.eventEmitter.Emit(sse.NewBookDeletedEvent(book.ID, *book.DeletedAt))

	// Remove from search index asynchronously with timeout
	if s.searchIndexer != nil {
		go func() {
			deleteCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := s.searchIndexer.DeleteBook(deleteCtx, id); err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to remove book from search index", "book_id", id, "error", err)
				}
			}
		}()
	}

	// Delete transcoded files asynchronously with timeout
	if s.transcodeDeleter != nil {
		go func() {
			deleteCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			if err := s.transcodeDeleter.DeleteTranscodesForBook(deleteCtx, id); err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to delete transcodes for book", "book_id", id, "error", err)
				}
			}
		}()
	}

	return nil
}

// GetBooksDeletedAfter efficiently queries all books with DeletedAt > timestamp.
// This is used for delta sync to inform clients which books were deleted.
// Returns a list of book IDs that were soft-deleted after the given timestamp.
func (s *Store) GetBooksDeletedAfter(_ context.Context, timestamp time.Time) ([]string, error) {
	// Pre-allocate with small capacity - deletions are typically rare
	bookIDs := make([]string, 0, 16)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		// Seek to the timestamp.
		seekKey := formatTimestampIndexKey(bookByDeletedAtPrefix, timestamp, "", "")
		prefix := []byte(bookByDeletedAtPrefix)

		it.Seek(seekKey)
		for it.ValidForPrefix(prefix) {
			key := it.Item().Key()

			entityType, entityID, err := parseTimestampIndexKey(key, bookByDeletedAtPrefix)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to parse deleted_at key", "key", string(key), "error", err)
				}
				it.Next()
				continue
			}

			if entityType == "book" {
				bookIDs = append(bookIDs, entityID)
			}
			it.Next()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan deleted_at index: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("deleted books query completed",
			"timestamp", timestamp.Format(time.RFC3339),
			"books_deleted", len(bookIDs),
		)
	}

	return bookIDs, nil
}

// BookExists checks if a book exists in our db by ID.
// This is a system operation that bypasses ACL.
func (s *Store) BookExists(ctx context.Context, id string) (bool, error) {
	book, err := s.getBookInternal(ctx, id)
	if err != nil {
		if errors.Is(err, ErrBookNotFound) {
			return false, nil
		}
		return false, err
	}
	return book != nil, nil
}

// ListBooks returns a paginated list of all books.
func (s *Store) ListBooks(ctx context.Context, params PaginationParams) (*PaginatedResult[*domain.Book], error) {
	params.Validate()

	books := make([]*domain.Book, 0, params.Limit)
	var lastKey string
	var hasMore bool

	prefix := []byte(bookPrefix)

	// Decode cursor to get starting key.
	startKey, err := DecodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchSize = params.Limit + 1 // We fetch one extra to check if there's more items.

		it := txn.NewIterator(opts)
		defer it.Close()

		// Start from cursor or beginning.
		var seekKey []byte
		if startKey != "" {
			seekKey = []byte(startKey)
			it.Seek(seekKey)
			// Skip the cursor key itself (we've already returned it).
			if it.Valid() && string(it.Item().Key()) == startKey {
				it.Next()
			}
		} else {
			seekKey = prefix
			it.Seek(seekKey)
		}

		// Collect items up to limit (excluding deleted books).
		count := 0
		for it.ValidForPrefix(prefix) {
			item := it.Item()
			key := string(item.Key())

			// If we've collected enough items, check if there are more non-deleted books.
			if count == params.Limit {
				// Check if there's at least one more non-deleted book.
				for it.ValidForPrefix(prefix) {
					var checkBook domain.Book
					err := it.Item().Value(func(val []byte) error {
						return json.Unmarshal(val, &checkBook)
					})
					if err != nil {
						it.Next()
						continue
					}
					if !checkBook.IsDeleted() {
						hasMore = true
						break
					}
					it.Next()
				}
				break
			}

			err := item.Value(func(val []byte) error {
				var book domain.Book
				if err := json.Unmarshal(val, &book); err != nil {
					return err
				}

				// Skip deleted books.
				if book.IsDeleted() {
					return nil
				}

				books = append(books, &book)
				lastKey = key
				count++
				return nil
			})
			if err != nil {
				return err
			}
			it.Next()
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}

	// Create result.
	result := &PaginatedResult[*domain.Book]{
		Items:   books,
		HasMore: hasMore,
	}

	// Set next cursor if there are more results.
	if hasMore && lastKey != "" {
		// Use the last returned items key as cursor.
		if len(books) > 0 {
			result.NextCursor = EncodeCursor(bookPrefix + books[len(books)-1].ID)
		}
	}

	return result, nil
}

// ListAllBooks returns all books (non-paginated).
// WARNING: this is probably not the function you're looking for. It's mostly here cause I think we'll.
// need it for synching down the the line.  Likely, you'll want to use the paginated function (ListBooks) instead.
// But you're an adult (probably) do what you like.
func (s *Store) ListAllBooks(ctx context.Context) ([]*domain.Book, error) {
	// Pre-allocate with reasonable capacity for typical library size
	books := make([]*domain.Book, 0, 256)

	prefix := []byte(bookPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var book domain.Book
				if err := json.Unmarshal(val, &book); err != nil {
					return err
				}
				books = append(books, &book)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list all books: %w", err)
	}

	return books, nil
}

// GetAllBookIDs returns all non-deleted book IDs.
// Note: This now needs to fetch values to check DeletedAt field.
// TODO: Consider adding an "active books" index for better performance.
func (s *Store) GetAllBookIDs(_ context.Context) ([]string, error) {
	// Pre-allocate with reasonable capacity for typical library size
	bookIDs := make([]string, 0, 256)

	prefix := []byte(bookPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = true // Need values to check DeletedAt

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var book domain.Book
				if err := json.Unmarshal(val, &book); err != nil {
					return err
				}

				// Skip deleted books.
				if book.IsDeleted() {
					return nil
				}

				bookIDs = append(bookIDs, book.ID)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get all book IDs: %w", err)
	}

	return bookIDs, nil
}

// GetBooksByCollectionPaginated returns paginated books in a collection.
func (s *Store) GetBooksByCollectionPaginated(ctx context.Context, userID, collectionID string, params PaginationParams) (*PaginatedResult[*domain.Book], error) {
	params.Validate()

	coll, err := s.GetCollection(ctx, collectionID, userID)
	if err != nil {
		return nil, err
	}

	// Decode cursor to get starting index.
	startIdx := 0
	if params.Cursor != "" {
		decoded, err := DecodeCursor(params.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}

		idx, err := strconv.Atoi(decoded)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor format: %w", err)
		}
		startIdx = idx
	}

	// Calculate end index.
	endIdx := min(startIdx+params.Limit, len(coll.BookIDs))

	// Get slice of book IDs for this page.
	pageBookIDs := coll.BookIDs[startIdx:endIdx]

	// Fetch Books.
	books := make([]*domain.Book, 0, len(pageBookIDs))
	for _, bookID := range pageBookIDs {
		book, err := s.GetBook(ctx, bookID, userID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to get book from collection", "book_id", bookID, "collection_id", collectionID, "error", err)
			}
			continue
		}
		books = append(books, book)
	}

	hasMore := endIdx < len(coll.BookIDs)

	result := &PaginatedResult[*domain.Book]{
		Items:   books,
		HasMore: hasMore,
		Total:   len(coll.BookIDs),
	}

	if hasMore {
		result.NextCursor = EncodeCursor(strconv.Itoa(endIdx))
	}

	return result, nil
}

// TouchEntity updated the Updated timestamp for an entity.
// This implements our CascadeUpdater interface.
func (s *Store) TouchEntity(ctx context.Context, entityType, id string) error {
	switch entityType {
	case "book":
		return s.touchBook(ctx, id)
	case "contributor":
		return s.touchContributor(ctx, id)
	case "series":
		return s.touchSeries(ctx, id)
	default:
		return fmt.Errorf("unknown entity type: %s", entityType)
	}
}

// touchBook updates just the UpdatedAt timestampe for a book without rewriting all data.
func (s *Store) touchBook(_ context.Context, id string) error {
	key := []byte(bookPrefix + id)

	return s.db.Update(func(txn *badger.Txn) error {
		// Get existing book.
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrBookNotFound
			}
			return err
		}

		var book domain.Book
		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &book)
		})
		if err != nil {
			return fmt.Errorf("unmarshal book: %w", err)
		}

		// Update timestamp.
		book.Touch()

		// Marshal and Save.
		data, err := json.Marshal(&book)
		if err != nil {
			return fmt.Errorf("marshal book: %w", err)
		}

		return txn.Set(key, data)
	})
}

// EnrichBook denormalizes a book with contributor names, series name, and genre names.
// This is a convenience wrapper around the enricher for API handlers.
func (s *Store) EnrichBook(ctx context.Context, book *domain.Book) (*dto.Book, error) {
	return s.enricher.EnrichBook(ctx, book)
}

// ContributorInput represents a contributor to be set on a book.
// Used by SetBookContributors to link or create contributors.
type ContributorInput struct {
	Name  string                   `json:"name"`
	Roles []domain.ContributorRole `json:"roles"`
}

// SetBookContributors replaces all contributors for a book.
// For each contributor:
//   - If name matches existing (case-insensitive, normalized) → link to that contributor
//   - Else → create new contributor and link
//
// After linking, checks old contributors for orphan cleanup (soft delete if no books reference them).
// Returns the updated book.
func (s *Store) SetBookContributors(ctx context.Context, bookID string, contributors []ContributorInput) (*domain.Book, error) {
	// Get current book (internal - no ACL check, caller should have already validated)
	book, err := s.getBookInternal(ctx, bookID)
	if err != nil {
		return nil, err
	}

	// Save old contributor IDs for orphan cleanup
	oldContributorIDs := make(map[string]bool)
	for _, bc := range book.Contributors {
		oldContributorIDs[bc.ContributorID] = true
	}

	// Build new contributor list
	newBookContributors := make([]domain.BookContributor, 0, len(contributors))
	newContributorIDs := make(map[string]bool)

	for _, input := range contributors {
		// Get or create the contributor by name (handles normalization internally)
		contributor, err := s.GetOrCreateContributorByName(ctx, input.Name)
		if err != nil {
			return nil, fmt.Errorf("get or create contributor %q: %w", input.Name, err)
		}

		newBookContributors = append(newBookContributors, domain.BookContributor{
			ContributorID: contributor.ID,
			Roles:         input.Roles,
		})
		newContributorIDs[contributor.ID] = true
	}

	// Update book with new contributors
	book.Contributors = newBookContributors

	// Save the book - UpdateBook handles indices, SSE, and search reindex
	if err := s.UpdateBook(ctx, book); err != nil {
		return nil, fmt.Errorf("update book contributors: %w", err)
	}

	// Orphan cleanup: check old contributors that are no longer linked
	for oldID := range oldContributorIDs {
		if newContributorIDs[oldID] {
			continue // Still linked to this book
		}

		// Check if this contributor has any remaining books
		count, err := s.CountBooksForContributor(ctx, oldID)
		if err != nil {
			// Log but don't fail the operation
			if s.logger != nil {
				s.logger.Warn("failed to count books for contributor during orphan cleanup",
					"contributor_id", oldID,
					"error", err,
				)
			}
			continue
		}

		if count == 0 {
			// Orphan detected - soft delete
			if err := s.DeleteContributor(ctx, oldID); err != nil {
				// Log but don't fail
				if s.logger != nil {
					s.logger.Warn("failed to delete orphan contributor",
						"contributor_id", oldID,
						"error", err,
					)
				}
			} else if s.logger != nil {
				s.logger.Info("deleted orphan contributor",
					"contributor_id", oldID,
				)
			}
		}
	}

	return book, nil
}

// SeriesInput represents a series to be set on a book.
// Used by SetBookSeries to link or create series.
type SeriesInput struct {
	Name     string `json:"name"`
	Sequence string `json:"sequence"`
}

// SetBookSeries replaces all series for a book.
// For each series:
//   - If name matches existing (case-insensitive, normalized) → link to that series
//   - Else → create new series and link
//
// After linking, checks old series for orphan cleanup (soft delete if no books reference them).
// Returns the updated book.
func (s *Store) SetBookSeries(ctx context.Context, bookID string, seriesInputs []SeriesInput) (*domain.Book, error) {
	// Get current book (internal - no ACL check, caller should have already validated)
	book, err := s.getBookInternal(ctx, bookID)
	if err != nil {
		return nil, err
	}

	// Save old series IDs for orphan cleanup
	oldSeriesIDs := make(map[string]bool)
	for _, bs := range book.Series {
		oldSeriesIDs[bs.SeriesID] = true
	}

	// Build new series list
	newBookSeries := make([]domain.BookSeries, 0, len(seriesInputs))
	newSeriesIDs := make(map[string]bool)

	for _, input := range seriesInputs {
		// Get or create the series by name (handles normalization internally)
		series, err := s.GetOrCreateSeriesByName(ctx, input.Name)
		if err != nil {
			return nil, fmt.Errorf("get or create series %q: %w", input.Name, err)
		}

		newBookSeries = append(newBookSeries, domain.BookSeries{
			SeriesID: series.ID,
			Sequence: input.Sequence,
		})
		newSeriesIDs[series.ID] = true
	}

	// Update book with new series
	book.Series = newBookSeries

	// Save the book - UpdateBook handles indices, SSE, and search reindex
	if err := s.UpdateBook(ctx, book); err != nil {
		return nil, fmt.Errorf("update book series: %w", err)
	}

	// Orphan cleanup: check old series that are no longer linked
	for oldID := range oldSeriesIDs {
		if newSeriesIDs[oldID] {
			continue // Still linked to this book
		}

		// Check if this series has any remaining books
		count, err := s.CountBooksInSeries(ctx, oldID)
		if err != nil {
			// Log but don't fail the operation
			if s.logger != nil {
				s.logger.Warn("failed to count books for series during orphan cleanup",
					"series_id", oldID,
					"error", err,
				)
			}
			continue
		}

		if count == 0 {
			// Orphan detected - soft delete
			if err := s.DeleteSeries(ctx, oldID); err != nil {
				// Log but don't fail
				if s.logger != nil {
					s.logger.Warn("failed to delete orphan series",
						"series_id", oldID,
						"error", err,
					)
				}
			} else if s.logger != nil {
				s.logger.Info("deleted orphan series",
					"series_id", oldID,
				)
			}
		}
	}

	return book, nil
}
