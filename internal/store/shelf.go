package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// Key prefixes for shelf storage (NOTE: actual key strings use legacy "shelf" prefix for backward compatibility).
const (
	shelfPrefix        = "shelf:"             // Storage key uses legacy "shelf:" prefix for backward compatibility
	shelvesByOwnerPrefix = "idx:shelves:owner:" // Storage key uses legacy "shelves" prefix for backward compatibility
	bookShelfPrefix    = "idx:books:shelves:" // Storage key uses legacy "shelves" prefix for backward compatibility
)

// CreateShelf creates a new shelf in the store.
// Creates the shelf, owner index, and book indexes for any initial BookIDs.
func (s *Store) CreateShelf(_ context.Context, shelf *domain.Shelf) error {
	key := []byte(shelfPrefix + shelf.ID)

	// Check if already exists.
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check shelf exists: %w", err)
	}
	if exists {
		return ErrDuplicateShelf
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(shelf)
		if err != nil {
			return fmt.Errorf("marshal shelf: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create owner index: idx:shelves:owner:{ownerID}:{shelfID}
		ownerIndexKey := fmt.Appendf(nil, "%s%s:%s", shelvesByOwnerPrefix, shelf.OwnerID, shelf.ID)
		if err := txn.Set(ownerIndexKey, []byte{}); err != nil {
			return fmt.Errorf("set owner index: %w", err)
		}

		// Create book indexes for initial BookIDs: idx:books:shelves:{bookID}:{shelfID}
		for _, bookID := range shelf.BookIDs {
			bookShelfKey := fmt.Appendf(nil, "%s%s:%s", bookShelfPrefix, bookID, shelf.ID)
			if err := txn.Set(bookShelfKey, []byte{}); err != nil {
				return fmt.Errorf("set book-shelf index: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create shelf: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("shelf created",
			"id", shelf.ID,
			"name", shelf.Name,
			"owner_id", shelf.OwnerID,
			"book_count", len(shelf.BookIDs),
		)
	}
	return nil
}

// GetShelf retrieves a shelf by ID.
func (s *Store) GetShelf(_ context.Context, id string) (*domain.Shelf, error) {
	key := []byte(shelfPrefix + id)

	var shelf domain.Shelf
	if err := s.get(key, &shelf); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrShelfNotFound
		}
		return nil, fmt.Errorf("get shelf: %w", err)
	}

	return &shelf, nil
}

// UpdateShelf updates an existing shelf in the store.
// Maintains book indexes by diffing old vs new BookIDs.
func (s *Store) UpdateShelf(ctx context.Context, shelf *domain.Shelf) error {
	key := []byte(shelfPrefix + shelf.ID)

	// Get old shelf state to detect BookIDs changes.
	oldShelf, err := s.GetShelf(ctx, shelf.ID)
	if err != nil {
		return err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Marshal and update shelf.
		data, err := json.Marshal(shelf)
		if err != nil {
			return fmt.Errorf("marshal shelf: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return fmt.Errorf("set shelf: %w", err)
		}

		// Build sets for efficient comparison.
		oldBooks := make(map[string]bool)
		for _, bookID := range oldShelf.BookIDs {
			oldBooks[bookID] = true
		}

		newBooks := make(map[string]bool)
		for _, bookID := range shelf.BookIDs {
			newBooks[bookID] = true
		}

		// Add indexes for new books.
		for bookID := range newBooks {
			if !oldBooks[bookID] {
				// Book was added to shelf.
				bookShelfKey := fmt.Appendf(nil, "%s%s:%s", bookShelfPrefix, bookID, shelf.ID)
				if err := txn.Set(bookShelfKey, []byte{}); err != nil {
					return fmt.Errorf("set book-shelf index: %w", err)
				}
			}
		}

		// Remove indexes for removed books.
		for bookID := range oldBooks {
			if !newBooks[bookID] {
				// Book was removed from shelf.
				bookShelfKey := fmt.Appendf(nil, "%s%s:%s", bookShelfPrefix, bookID, shelf.ID)
				if err := txn.Delete(bookShelfKey); err != nil {
					return fmt.Errorf("delete book-shelf index: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("update shelf: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("shelf updated",
			"id", shelf.ID,
			"name", shelf.Name,
		)
	}
	return nil
}

// DeleteShelf deletes a shelf and all its indexes.
func (s *Store) DeleteShelf(ctx context.Context, id string) error {
	// Get shelf to retrieve owner and book IDs for index cleanup.
	shelf, err := s.GetShelf(ctx, id)
	if err != nil {
		return err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Delete the shelf itself.
		key := []byte(shelfPrefix + id)
		if err := txn.Delete(key); err != nil {
			return fmt.Errorf("delete shelf: %w", err)
		}

		// Delete owner index.
		ownerIndexKey := fmt.Appendf(nil, "%s%s:%s", shelvesByOwnerPrefix, shelf.OwnerID, id)
		if err := txn.Delete(ownerIndexKey); err != nil {
			// Ignore if key doesn't exist.
			if !errors.Is(err, badger.ErrKeyNotFound) {
				return fmt.Errorf("delete owner index: %w", err)
			}
		}

		// Delete all book-shelf indexes.
		for _, bookID := range shelf.BookIDs {
			bookShelfKey := fmt.Appendf(nil, "%s%s:%s", bookShelfPrefix, bookID, id)
			if err := txn.Delete(bookShelfKey); err != nil {
				// Ignore if key doesn't exist.
				if !errors.Is(err, badger.ErrKeyNotFound) {
					return fmt.Errorf("delete book-shelf index: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("delete shelf: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("shelf deleted", "id", id)
	}
	return nil
}

// ListShelvesByOwner returns all shelves owned by a user.
func (s *Store) ListShelvesByOwner(ctx context.Context, ownerID string) ([]*domain.Shelf, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var shelfIDs []string

	// Scan owner index: idx:shelves:owner:{ownerID}:{shelfID}
	prefix := fmt.Appendf(nil, "%s%s:", shelvesByOwnerPrefix, ownerID)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only need keys.
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			// Key format: idx:shelves:owner:{ownerID}:{shelfID}
			// Extract shelfID (everything after the last colon).
			parts := string(key)
			lastColon := -1
			for i := len(parts) - 1; i >= 0; i-- {
				if parts[i] == ':' {
					lastColon = i
					break
				}
			}
			if lastColon != -1 && lastColon < len(parts)-1 {
				shelfID := parts[lastColon+1:]
				shelfIDs = append(shelfIDs, shelfID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan owner index: %w", err)
	}

	// Load the shelves.
	shelves := make([]*domain.Shelf, 0, len(shelfIDs))
	for _, shelfID := range shelfIDs {
		shelf, err := s.GetShelf(ctx, shelfID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to get shelf from index", "shelf_id", shelfID, "error", err)
			}
			continue
		}
		shelves = append(shelves, shelf)
	}

	return shelves, nil
}

// ListAllShelves returns all shelves in the store.
// Useful for Discover functionality.
func (s *Store) ListAllShelves(ctx context.Context) ([]*domain.Shelf, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var shelves []*domain.Shelf

	prefix := []byte(shelfPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var shelf domain.Shelf
				if err := json.Unmarshal(val, &shelf); err != nil {
					return err
				}
				shelves = append(shelves, &shelf)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list shelves: %w", err)
	}

	return shelves, nil
}

// GetShelvesContainingBook returns all shelves that contain a specific book.
// Uses reverse index for efficient lookup.
func (s *Store) GetShelvesContainingBook(ctx context.Context, bookID string) ([]*domain.Shelf, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var shelfIDs []string

	// Scan book-shelf index: idx:books:shelves:{bookID}:{shelfID}
	prefix := fmt.Appendf(nil, "%s%s:", bookShelfPrefix, bookID)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only need keys.
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			// Key format: idx:books:shelves:{bookID}:{shelfID}
			// Extract shelfID (everything after the last colon).
			parts := string(key)
			lastColon := -1
			for i := len(parts) - 1; i >= 0; i-- {
				if parts[i] == ':' {
					lastColon = i
					break
				}
			}
			if lastColon != -1 && lastColon < len(parts)-1 {
				shelfID := parts[lastColon+1:]
				shelfIDs = append(shelfIDs, shelfID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan book-shelf index: %w", err)
	}

	// Load the shelves.
	shelves := make([]*domain.Shelf, 0, len(shelfIDs))
	for _, shelfID := range shelfIDs {
		shelf, err := s.GetShelf(ctx, shelfID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to get shelf from index", "shelf_id", shelfID, "error", err)
			}
			continue
		}
		shelves = append(shelves, shelf)
	}

	return shelves, nil
}

// AddBookToShelf adds a book to a shelf.
// This is a convenience wrapper that loads the shelf, adds the book, and saves.
func (s *Store) AddBookToShelf(ctx context.Context, shelfID, bookID string) error {
	shelf, err := s.GetShelf(ctx, shelfID)
	if err != nil {
		return err
	}

	// Use shelf's AddBook helper method.
	if !shelf.AddBook(bookID) {
		// Book already in shelf.
		return nil
	}

	return s.UpdateShelf(ctx, shelf)
}

// RemoveBookFromShelf removes a book from a shelf.
// This is a convenience wrapper that loads the shelf, removes the book, and saves.
func (s *Store) RemoveBookFromShelf(ctx context.Context, shelfID, bookID string) error {
	shelf, err := s.GetShelf(ctx, shelfID)
	if err != nil {
		return err
	}

	// Use shelf's RemoveBook helper method.
	if !shelf.RemoveBook(bookID) {
		// Book not in shelf.
		return nil
	}

	return s.UpdateShelf(ctx, shelf)
}

// DeleteShelvesForUser deletes all shelves owned by a user.
// This is a cascade delete operation for user account cleanup.
func (s *Store) DeleteShelvesForUser(ctx context.Context, userID string) error {
	// Get all shelves for user.
	shelves, err := s.ListShelvesByOwner(ctx, userID)
	if err != nil {
		return fmt.Errorf("list shelves for user: %w", err)
	}

	// Delete each shelf.
	for _, shelf := range shelves {
		if err := s.DeleteShelf(ctx, shelf.ID); err != nil {
			return fmt.Errorf("delete shelf %s: %w", shelf.ID, err)
		}
	}

	if s.logger != nil {
		s.logger.Info("deleted shelves for user",
			"user_id", userID,
			"count", len(shelves),
		)
	}

	return nil
}
