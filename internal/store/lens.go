package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// Key prefixes for lens storage.
const (
	lensPrefix        = "lens:"             // lens:{id} → Lens
	lensByOwnerPrefix = "idx:lenses:owner:" // idx:lenses:owner:{ownerID}:{lensID}
	bookLensPrefix    = "idx:books:lenses:" // idx:books:lenses:{bookID}:{lensID}
)

// CreateLens creates a new lens in the store.
// Creates the lens, owner index, and book indexes for any initial BookIDs.
func (s *Store) CreateLens(_ context.Context, lens *domain.Lens) error {
	key := []byte(lensPrefix + lens.ID)

	// Check if already exists.
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check lens exists: %w", err)
	}
	if exists {
		return ErrDuplicateLens
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(lens)
		if err != nil {
			return fmt.Errorf("marshal lens: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create owner index: idx:lenses:owner:{ownerID}:{lensID}
		ownerIndexKey := fmt.Appendf(nil, "%s%s:%s", lensByOwnerPrefix, lens.OwnerID, lens.ID)
		if err := txn.Set(ownerIndexKey, []byte{}); err != nil {
			return fmt.Errorf("set owner index: %w", err)
		}

		// Create book indexes for initial BookIDs: idx:books:lenses:{bookID}:{lensID}
		for _, bookID := range lens.BookIDs {
			bookLensKey := fmt.Appendf(nil, "%s%s:%s", bookLensPrefix, bookID, lens.ID)
			if err := txn.Set(bookLensKey, []byte{}); err != nil {
				return fmt.Errorf("set book-lens index: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create lens: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("lens created",
			"id", lens.ID,
			"name", lens.Name,
			"owner_id", lens.OwnerID,
			"book_count", len(lens.BookIDs),
		)
	}
	return nil
}

// GetLens retrieves a lens by ID.
func (s *Store) GetLens(_ context.Context, id string) (*domain.Lens, error) {
	key := []byte(lensPrefix + id)

	var lens domain.Lens
	if err := s.get(key, &lens); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrLensNotFound
		}
		return nil, fmt.Errorf("get lens: %w", err)
	}

	return &lens, nil
}

// UpdateLens updates an existing lens in the store.
// Maintains book indexes by diffing old vs new BookIDs.
func (s *Store) UpdateLens(ctx context.Context, lens *domain.Lens) error {
	key := []byte(lensPrefix + lens.ID)

	// Get old lens state to detect BookIDs changes.
	oldLens, err := s.GetLens(ctx, lens.ID)
	if err != nil {
		return err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Marshal and update lens.
		data, err := json.Marshal(lens)
		if err != nil {
			return fmt.Errorf("marshal lens: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return fmt.Errorf("set lens: %w", err)
		}

		// Build sets for efficient comparison.
		oldBooks := make(map[string]bool)
		for _, bookID := range oldLens.BookIDs {
			oldBooks[bookID] = true
		}

		newBooks := make(map[string]bool)
		for _, bookID := range lens.BookIDs {
			newBooks[bookID] = true
		}

		// Add indexes for new books.
		for bookID := range newBooks {
			if !oldBooks[bookID] {
				// Book was added to lens.
				bookLensKey := fmt.Appendf(nil, "%s%s:%s", bookLensPrefix, bookID, lens.ID)
				if err := txn.Set(bookLensKey, []byte{}); err != nil {
					return fmt.Errorf("set book-lens index: %w", err)
				}
			}
		}

		// Remove indexes for removed books.
		for bookID := range oldBooks {
			if !newBooks[bookID] {
				// Book was removed from lens.
				bookLensKey := fmt.Appendf(nil, "%s%s:%s", bookLensPrefix, bookID, lens.ID)
				if err := txn.Delete(bookLensKey); err != nil {
					return fmt.Errorf("delete book-lens index: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("update lens: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("lens updated",
			"id", lens.ID,
			"name", lens.Name,
		)
	}
	return nil
}

// DeleteLens deletes a lens and all its indexes.
func (s *Store) DeleteLens(ctx context.Context, id string) error {
	// Get lens to retrieve owner and book IDs for index cleanup.
	lens, err := s.GetLens(ctx, id)
	if err != nil {
		return err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Delete the lens itself.
		key := []byte(lensPrefix + id)
		if err := txn.Delete(key); err != nil {
			return fmt.Errorf("delete lens: %w", err)
		}

		// Delete owner index.
		ownerIndexKey := fmt.Appendf(nil, "%s%s:%s", lensByOwnerPrefix, lens.OwnerID, id)
		if err := txn.Delete(ownerIndexKey); err != nil {
			// Ignore if key doesn't exist.
			if !errors.Is(err, badger.ErrKeyNotFound) {
				return fmt.Errorf("delete owner index: %w", err)
			}
		}

		// Delete all book-lens indexes.
		for _, bookID := range lens.BookIDs {
			bookLensKey := fmt.Appendf(nil, "%s%s:%s", bookLensPrefix, bookID, id)
			if err := txn.Delete(bookLensKey); err != nil {
				// Ignore if key doesn't exist.
				if !errors.Is(err, badger.ErrKeyNotFound) {
					return fmt.Errorf("delete book-lens index: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("delete lens: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("lens deleted", "id", id)
	}
	return nil
}

// ListLensesByOwner returns all lenses owned by a user.
func (s *Store) ListLensesByOwner(ctx context.Context, ownerID string) ([]*domain.Lens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var lensIDs []string

	// Scan owner index: idx:lenses:owner:{ownerID}:{lensID}
	prefix := fmt.Appendf(nil, "%s%s:", lensByOwnerPrefix, ownerID)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only need keys.
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			// Key format: idx:lenses:owner:{ownerID}:{lensID}
			// Extract lensID (everything after the last colon).
			parts := string(key)
			lastColon := -1
			for i := len(parts) - 1; i >= 0; i-- {
				if parts[i] == ':' {
					lastColon = i
					break
				}
			}
			if lastColon != -1 && lastColon < len(parts)-1 {
				lensID := parts[lastColon+1:]
				lensIDs = append(lensIDs, lensID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan owner index: %w", err)
	}

	// Load the lenses.
	lenses := make([]*domain.Lens, 0, len(lensIDs))
	for _, lensID := range lensIDs {
		lens, err := s.GetLens(ctx, lensID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to get lens from index", "lens_id", lensID, "error", err)
			}
			continue
		}
		lenses = append(lenses, lens)
	}

	return lenses, nil
}

// ListAllLenses returns all lenses in the store.
// Useful for Discover functionality.
func (s *Store) ListAllLenses(ctx context.Context) ([]*domain.Lens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var lenses []*domain.Lens

	prefix := []byte(lensPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var lens domain.Lens
				if err := json.Unmarshal(val, &lens); err != nil {
					return err
				}
				lenses = append(lenses, &lens)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list lenses: %w", err)
	}

	return lenses, nil
}

// GetLensesContainingBook returns all lenses that contain a specific book.
// Uses reverse index for efficient lookup.
func (s *Store) GetLensesContainingBook(ctx context.Context, bookID string) ([]*domain.Lens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var lensIDs []string

	// Scan book-lens index: idx:books:lenses:{bookID}:{lensID}
	prefix := fmt.Appendf(nil, "%s%s:", bookLensPrefix, bookID)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only need keys.
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			// Key format: idx:books:lenses:{bookID}:{lensID}
			// Extract lensID (everything after the last colon).
			parts := string(key)
			lastColon := -1
			for i := len(parts) - 1; i >= 0; i-- {
				if parts[i] == ':' {
					lastColon = i
					break
				}
			}
			if lastColon != -1 && lastColon < len(parts)-1 {
				lensID := parts[lastColon+1:]
				lensIDs = append(lensIDs, lensID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan book-lens index: %w", err)
	}

	// Load the lenses.
	lenses := make([]*domain.Lens, 0, len(lensIDs))
	for _, lensID := range lensIDs {
		lens, err := s.GetLens(ctx, lensID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to get lens from index", "lens_id", lensID, "error", err)
			}
			continue
		}
		lenses = append(lenses, lens)
	}

	return lenses, nil
}

// AddBookToLens adds a book to a lens.
// This is a convenience wrapper that loads the lens, adds the book, and saves.
func (s *Store) AddBookToLens(ctx context.Context, lensID, bookID string) error {
	lens, err := s.GetLens(ctx, lensID)
	if err != nil {
		return err
	}

	// Use lens's AddBook helper method.
	if !lens.AddBook(bookID) {
		// Book already in lens.
		return nil
	}

	return s.UpdateLens(ctx, lens)
}

// RemoveBookFromLens removes a book from a lens.
// This is a convenience wrapper that loads the lens, removes the book, and saves.
func (s *Store) RemoveBookFromLens(ctx context.Context, lensID, bookID string) error {
	lens, err := s.GetLens(ctx, lensID)
	if err != nil {
		return err
	}

	// Use lens's RemoveBook helper method.
	if !lens.RemoveBook(bookID) {
		// Book not in lens.
		return nil
	}

	return s.UpdateLens(ctx, lens)
}

// DeleteLensesForUser deletes all lenses owned by a user.
// This is a cascade delete operation for user account cleanup.
func (s *Store) DeleteLensesForUser(ctx context.Context, userID string) error {
	// Get all lenses for user.
	lenses, err := s.ListLensesByOwner(ctx, userID)
	if err != nil {
		return fmt.Errorf("list lenses for user: %w", err)
	}

	// Delete each lens.
	for _, lens := range lenses {
		if err := s.DeleteLens(ctx, lens.ID); err != nil {
			return fmt.Errorf("delete lens %s: %w", lens.ID, err)
		}
	}

	if s.logger != nil {
		s.logger.Info("deleted lenses for user",
			"user_id", userID,
			"count", len(lenses),
		)
	}

	return nil
}
