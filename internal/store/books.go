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
	"github.com/listenupapp/listenup-server/internal/sse"
)

const (
	bookPrefix            = "book:"
	bookByPathPrefix      = "idx:books:path:"
	bookByInodePrefix     = "idx:books:inode"
	bookByUpdatedAtPrefix = "idx:books:updated_at:" // Format: idx:books:updated_at:{RFC3339Nano}:book:{uuid}
	bookByDeletedAtPrefix = "idx:books:deleted_at:" // Format: idx:books:deleted_at:{RFC3339Nano}:book:{uuid}
)

var (
	ErrBookNotFound = errors.New("book not found")
	ErrBookExists   = errors.New("book already exists")
)

// Book Operations

// CreateBook creates a new book
func (s *Store) CreateBook(ctx context.Context, book *domain.Book) error {
	key := []byte(bookPrefix + book.ID)

	// Check if it already exists
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check book exists: %w", err)
	}
	if exists {
		return ErrBookExists
	}

	// Use transaction to create book indicies atomically
	err = s.db.Update(func(txn *badger.Txn) error {
		// Save book
		data, err := json.Marshal(book)
		if err != nil {
			return fmt.Errorf("marshal book: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create path index
		pathKey := []byte(bookByPathPrefix + book.Path)
		if err := txn.Set(pathKey, []byte(book.ID)); err != nil {
			return err
		}

		// Create inode indices for eac haudio file (for fast file watching lookups)
		for _, audioFile := range book.AudioFiles {
			if audioFile.Inode > 0 {
				inodeKey := []byte(fmt.Sprintf("%s%d", bookByInodePrefix, audioFile.Inode))
				if err := txn.Set(inodeKey, []byte(book.ID)); err != nil {
					return err
				}
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

	s.eventEmitter.Emit(sse.NewBookCreatedEvent(book))
	return nil
}

// GetBook retrieves a book by ID
func (s *Store) GetBook(ctx context.Context, id string) (*domain.Book, error) {
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

	// Treat soft-deleted books as not found
	if book.IsDeleted() {
		return nil, ErrBookNotFound
	}

	return &book, nil
}

// GetBookByInode retrieves a book by an audio file inode
// This is used during file watching for fast lookups when a file changes
func (s *Store) GetBookByInode(ctx context.Context, inode int64) (*domain.Book, error) {
	inodeKey := []byte(fmt.Sprintf("%s%d", bookByInodePrefix, inode))

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
	return s.GetBook(ctx, bookID)
}

// UpdateBook updates an existing book
func (s *Store) UpdateBook(ctx context.Context, book *domain.Book) error {
	key := []byte(bookPrefix + book.ID)

	// Get old book for index updates
	oldBook, err := s.GetBook(ctx, book.ID)
	if err != nil {
		return err
	}

	book.Touch()

	// Use Transaction to update book and indices atomically
	err = s.db.Update(func(txn *badger.Txn) error {
		book.Touch()
		// Update book
		data, err := json.Marshal(book)
		if err != nil {
			return fmt.Errorf("marshal book: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update path index if path changes
		if oldBook.Path != book.Path {
			// Delete old path index
			oldPathKey := []byte(bookByPathPrefix + oldBook.Path)
			if err := txn.Delete(oldPathKey); err != nil {
				return err
			}

			// Create new path index
			newPathKey := []byte(bookByPathPrefix + book.Path)
			if err := txn.Set(newPathKey, []byte(book.ID)); err != nil {
				return err
			}
		}

		// Update inode indices
		// Build maps of old and new inodes
		oldInodes := make(map[uint64]bool)
		for _, af := range oldBook.AudioFiles {
			if af.Inode > 0 {
				oldInodes[af.Inode] = true
			}
		}
		newInodes := make(map[uint64]bool)
		for _, af := range book.AudioFiles {
			if af.Inode > 0 {
				newInodes[af.Inode] = true
			}
		}

		// Delete removed inodes
		for inode := range oldInodes {
			if !newInodes[inode] {
				inodeKey := []byte(fmt.Sprintf("%s%d", bookByInodePrefix, inode))
				if err := txn.Delete(inodeKey); err != nil {
					return err
				}
			}
		}

		// Add new inodes
		for inode := range newInodes {
			if !oldInodes[inode] {
				inodeKey := []byte(fmt.Sprintf("%s%d", bookByInodePrefix, inode))
				if err := txn.Set(inodeKey, []byte(book.ID)); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("update book: %w", err)
	}

	if err := domain.CascadeBookUpdate(ctx, s, book.ID); err != nil {
		// Log but don't fail the update
		if s.logger != nil {
			s.logger.Error("cascaed uodate failed", "book_id", book.ID, "error", err)
		}
	}

	if s.logger != nil {
		s.logger.Info("book updated", "id", book.ID, "title", book.Title)
	}

	s.eventEmitter.Emit(sse.NewBookUpdatedEvent(book))
	return nil
}

// DeleteBook deletes a book and removes it from all collections
func (s *Store) DeleteBook(ctx context.Context, id string) error {
	book, err := s.GetBook(ctx, id)
	if err != nil {
		return err
	}

	// Remove from all collections first
	collections, err := s.GetCollectionsForBook(ctx, id)
	if err != nil {
		return fmt.Errorf("get collections for book: %w", err)
	}

	for _, coll := range collections {
		if err := s.RemoveBookFromCollection(ctx, id, coll.ID); err != nil {
			return fmt.Errorf("remove from collection %s: %w", coll.ID, err)
		}
	}

	// Soft delete the book
	err = s.db.Update(func(txn *badger.Txn) error {
		// Mark book as deleted (sets DeletedAt and updates UpdatedAt)
		oldUpdatedAt := book.UpdatedAt
		book.MarkDeleted()

		// Save updated book with DeletedAt set
		key := []byte(bookPrefix + id)
		data, err := json.Marshal(book)
		if err != nil {
			return fmt.Errorf("marshal book: %w", err)
		}
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Delete secondary indexes (path, inode) - deleted books shouldn't be found by these
		pathKey := []byte(bookByPathPrefix + book.Path)
		if err := txn.Delete(pathKey); err != nil {
			return err
		}

		for _, audioFile := range book.AudioFiles {
			if audioFile.Inode > 0 {
				inodeKey := []byte(fmt.Sprintf("%s%d", bookByInodePrefix, audioFile.Inode))
				if err := txn.Delete(inodeKey); err != nil {
					return err
				}
			}
		}

		// Update updated_at index (deleted books still appear in delta queries)
		oldUpdatedAtKey := formatTimestampIndexKey(bookByUpdatedAtPrefix, oldUpdatedAt, "book", book.ID)
		if err := txn.Delete(oldUpdatedAtKey); err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		newUpdatedAtKey := formatTimestampIndexKey(bookByUpdatedAtPrefix, book.UpdatedAt, "book", book.ID)
		if err := txn.Set(newUpdatedAtKey, []byte{}); err != nil {
			return err
		}

		// Create deleted_at index
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
	return nil
}

// GetBooksDeletedAfter efficiently queries all books with DeletedAt > timestamp.
// This is used for delta sync to inform clients which books were deleted.
// Returns a list of book IDs that were soft-deleted after the given timestamp.
func (s *Store) GetBooksDeletedAfter(ctx context.Context, timestamp time.Time) ([]string, error) {
	var bookIDs []string

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		// Seek to the timestamp
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

// BookExists checks if a book exists in our db by ID
func (s *Store) BookExists(ctx context.Context, id string) (bool, error) {
	book, err := s.GetBook(ctx, id)
	if err != nil {
		if errors.Is(err, ErrBookNotFound) {
			return false, nil
		}
		return false, err
	}
	return book != nil, nil
}

func (s *Store) ListBooks(ctx context.Context, params PaginationParams) (*PaginatedResult[*domain.Book], error) {
	params.Validate()

	var books []*domain.Book
	var lastKey string
	var hasMore bool

	prefix := []byte(bookPrefix)

	// Decode cursor to get starting key
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

		// Start from cursor or beginning
		var seekKey []byte
		if startKey != "" {
			seekKey = []byte(startKey)
			it.Seek(seekKey)
			// Skip the cursor key itself (we've already returned it)
			if it.Valid() && string(it.Item().Key()) == startKey {
				it.Next()
			}
		} else {
			seekKey = prefix
			it.Seek(seekKey)
		}

		// Collect items up to limit (excluding deleted books)
		count := 0
		for it.ValidForPrefix(prefix) {
			item := it.Item()
			key := string(item.Key())

			// If we've collected enough items, check if there are more non-deleted books
			if count == params.Limit {
				// Check if there's at least one more non-deleted book
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

				// Skip deleted books
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

	// Create result
	result := &PaginatedResult[*domain.Book]{
		Items:   books,
		HasMore: hasMore,
	}

	// Set next cursor if there are more results
	if hasMore && lastKey != "" {
		// Use the last returned items key as cursor
		if len(books) > 0 {
			result.NextCursor = EncodeCursor(bookPrefix + books[len(books)-1].ID)
		}
	}

	return result, nil
}

// ListAllBooks returns all books (non-paginated)
// WARNING: this is probably not the function you're looking for. It's mostly here cause I think we'll
// need it for synching down the the line.  Likely, you'll want to use the paginated function (ListBooks) instead.
// But you're an adult (probably) do what you like.
func (s *Store) ListAllBooks(ctx context.Context) ([]*domain.Book, error) {
	var books []*domain.Book

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
func (s *Store) GetAllBookIDs(ctx context.Context) ([]string, error) {
	var bookIDs []string

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

				// Skip deleted books
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

// GetBooksByCollectionPaginated returns paginated books in a collection
func (s *Store) GetBooksByCollectionPaginated(ctx context.Context, collectionID string, params PaginationParams) (*PaginatedResult[*domain.Book], error) {
	params.Validate()

	coll, err := s.GetCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	// Decode cursor to get starting index
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

	// Calculate end index
	endIdx := startIdx + params.Limit
	if endIdx > len(coll.BookIDs) {
		endIdx = len(coll.BookIDs)
	}

	// Get slice of book IDs for this page
	pageBookIDs := coll.BookIDs[startIdx:endIdx]

	// Fetch Books
	books := make([]*domain.Book, 0, len(pageBookIDs))
	for _, bookID := range pageBookIDs {
		book, err := s.GetBook(ctx, bookID)
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
		result.NextCursor = EncodeCursor(fmt.Sprintf("%d", endIdx))
	}

	return result, nil
}

// TouchEntity updated the Updated timestamp for an entity.
// This implements our CascadeUpdater interface.
func (s *Store) TouchEntity(ctx context.Context, entityType string, id string) error {
	switch entityType {
	case "book":
		return s.touchBook(ctx, id)
	// TODO: add authors etc when they're here.
	default:
		return fmt.Errorf("unknown entity type: %s", entityType)
	}
}

// touchBook updates just the UpdatedAt timestampe for a book without rewriting all data
func (s *Store) touchBook(ctx context.Context, id string) error {
	key := []byte(bookPrefix + id)

	return s.db.Update(func(txn *badger.Txn) error {
		// Get existing book
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
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

		// Update timestamp
		book.Touch()

		// Marshal and Save
		data, err := json.Marshal(&book)
		if err != nil {
			return fmt.Errorf("marshal book: %w", err)
		}

		return txn.Set(key, data)
	})
}
