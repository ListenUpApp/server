package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const (
	bookPrefix        = "book:"
	bookByPathPrefix  = "idx:books:path:"
	bookByInodePrefix = "idx:books:inode"
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

	if s.logger != nil {
		s.logger.Info("book updated", "id", book.ID, "title", book.Title)
	}

	return nil
}

// DeleteBook deletes a book and removes it from all collections
func (s *Store) DeleteBook(ctx context.Context, id string) error {
	book, err := s.GetBook(ctx, id)
	if err != nil {
		return err
	}

	// Remove from all collections
	collections, err := s.GetCollectionsForBook(ctx, id)
	if err != nil {
		return fmt.Errorf("get collections for book: %w", err)
	}

	for _, coll := range collections {
		if err := s.RemoveBookFromCollection(ctx, id, coll.ID); err != nil {
			return fmt.Errorf("remove from collection %s: %w", coll.ID, err)
		}
	}

	// Delete book and indices
	err = s.db.Update(func(txn *badger.Txn) error {
		// Delete Book
		key := []byte(bookPrefix + id)
		if err := txn.Delete(key); err != nil {
			return err
		}

		// Delete Path Index
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
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete book: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("book deleted", "id", id, "title", book.Title)
	}

	return nil
}

// BookExists checks if a book exists in our db by ID
func (s *Store) BookExists(ctx context.Context, id string) (bool, error) {
	key := []byte(bookPrefix + id)
	return s.exists(key)
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

		// Collect items up to limit + 1
		count := 0
		for ; it.ValidForPrefix(prefix) && count <= params.Limit; it.Next() {
			item := it.Item()
			key := string(item.Key())

			// If we've hit limit + 1, we know there are more items
			if count == params.Limit {
				// Don't fetch this item, just note that there are more
				hasMore = true
				break
			}

			err := item.Value(func(val []byte) error {
				var book domain.Book
				if err := json.Unmarshal(val, &book); err != nil {
					return err
				}

				books = append(books, &book)
				lastKey = key
				return nil
			})
			if err != nil {
				return err
			}

			count++
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
