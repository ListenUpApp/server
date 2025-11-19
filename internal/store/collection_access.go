package store

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// GetCollectionsForUser returns all collections a user owns or has been shared with them.
// This includes both owned collections and collections shared via CollectionShare.
func (s *Store) GetCollectionsForUser(ctx context.Context, userID string) ([]*domain.Collection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var result []*domain.Collection
	seen := make(map[string]bool) // Deduplicate collections

	// Get all libraries to iterate through their collections
	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	for _, lib := range libraries {
		// Use internal method to get all collections without ACL filtering
		collections, err := s.ListAllCollectionsByLibrary(ctx, lib.ID)
		if err != nil {
			continue
		}

		for _, coll := range collections {
			if seen[coll.ID] {
				continue
			}

			// Include if user owns the collection
			if coll.OwnerID == userID {
				result = append(result, coll)
				seen[coll.ID] = true
				continue
			}

			// Check if collection is shared with this user
			share, err := s.GetShareForUserAndCollection(ctx, userID, coll.ID)
			if err == nil && share != nil {
				result = append(result, coll)
				seen[coll.ID] = true
			}
		}
	}

	return result, nil
}

// GetCollectionsContainingBook returns all collections that contain a specific book ID.
func (s *Store) GetCollectionsContainingBook(ctx context.Context, bookID string) ([]*domain.Collection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Use the existing method from collection.go
	return s.GetCollectionsForBook(ctx, bookID)
}

// GetBooksForUser returns all books the user can access (permissive model).
// A user can see a book if:
//  1. The book is not in any collection (uncollected = public), OR
//  2. The book is in at least one collection the user has access to
// Uses reverse indexes for O(Collections + BookIDs) instead of O(Books × Collections).
func (s *Store) GetBooksForUser(ctx context.Context, userID string) ([]*domain.Book, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get collections user has access to
	userCollections, err := s.GetCollectionsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user collections: %w", err)
	}

	// Collect bookIDs from user's accessible collections using reverse index
	accessibleBookIDs := make(map[string]bool)

	for _, coll := range userCollections {
		// Scan idx:books:collections:{collectionID}:{bookID}
		prefix := []byte(fmt.Sprintf("%s%s:", bookCollectionsPrefix, coll.ID))

		err := s.db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.PrefetchValues = false // Only need keys
			opts.Prefix = prefix

			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				key := it.Item().Key()
				// Key format: idx:books:collections:{collectionID}:{bookID}
				// Extract bookID (everything after last colon)
				parts := string(key)
				lastColon := -1
				for i := len(parts) - 1; i >= 0; i-- {
					if parts[i] == ':' {
						lastColon = i
						break
					}
				}
				if lastColon != -1 && lastColon < len(parts)-1 {
					bookID := parts[lastColon+1:]
					accessibleBookIDs[bookID] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan books in collection %s: %w", coll.ID, err)
		}
	}

	// Find uncollected books (books with no index entries)
	// These are public to all users
	bookPrefix := []byte("book:")
	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys to get IDs
		opts.Prefix = bookPrefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(bookPrefix); it.ValidForPrefix(bookPrefix); it.Next() {
			key := it.Item().Key()
			// Extract book ID from key: "book:{id}"
			bookID := string(key[len(bookPrefix):])

			// Check if this book has any collection index entries
			// If not, it's uncollected and public
			checkPrefix := []byte(fmt.Sprintf("%s%s:", collectionBooksPrefix, bookID))

			checkOpts := badger.DefaultIteratorOptions
			checkOpts.PrefetchValues = false
			checkOpts.Prefix = checkPrefix

			checkIt := txn.NewIterator(checkOpts)
			checkIt.Seek(checkPrefix)
			hasIndex := checkIt.ValidForPrefix(checkPrefix)
			checkIt.Close()

			if !hasIndex {
				// Book is uncollected -> public
				accessibleBookIDs[bookID] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan for uncollected books: %w", err)
	}

	// Load only the accessible books
	accessibleBooks := make([]*domain.Book, 0, len(accessibleBookIDs))
	for bookID := range accessibleBookIDs {
		book, err := s.getBookInternal(ctx, bookID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to load accessible book", "book_id", bookID, "error", err)
			}
			continue
		}
		accessibleBooks = append(accessibleBooks, book)
	}

	return accessibleBooks, nil
}

// CanUserAccessBook checks if a user can see a specific book.
// Returns true if book is uncollected OR user has access to at least one collection containing it.
// Uses reverse index for O(1) lookup.
func (s *Store) CanUserAccessBook(ctx context.Context, userID, bookID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// Verify book exists (use internal method to bypass ACL)
	_, err := s.getBookInternal(ctx, bookID)
	if err != nil {
		return false, err
	}

	// Check reverse index to see if book is in any collections
	// idx:collections:books:{bookID}:{collectionID}
	prefix := []byte(fmt.Sprintf("%s%s:", collectionBooksPrefix, bookID))

	var bookCollectionIDs []string
	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			// Extract collectionID from key
			parts := string(key)
			lastColon := -1
			for i := len(parts) - 1; i >= 0; i-- {
				if parts[i] == ':' {
					lastColon = i
					break
				}
			}
			if lastColon != -1 && lastColon < len(parts)-1 {
				collectionID := parts[lastColon+1:]
				bookCollectionIDs = append(bookCollectionIDs, collectionID)
			}
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("check book collections: %w", err)
	}

	// If book is in no collections, it's public
	if len(bookCollectionIDs) == 0 {
		return true, nil
	}

	// Get user's accessible collection IDs
	userCollections, err := s.GetCollectionsForUser(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("get user collections: %w", err)
	}

	// Create set of user's collection IDs for fast lookup
	userCollectionIDs := make(map[string]bool)
	for _, coll := range userCollections {
		userCollectionIDs[coll.ID] = true
	}

	// Check if any collection containing the book is accessible to user
	for _, collID := range bookCollectionIDs {
		if userCollectionIDs[collID] {
			return true, nil // User has access via at least one collection
		}
	}

	return false, nil
}

// CanUserAccessCollection checks if a user can access a collection.
// Returns: (canAccess bool, permission SharePermission, isOwner bool, error)
// isOwner is true if user owns the collection (implies Write permission).
func (s *Store) CanUserAccessCollection(ctx context.Context, userID, collectionID string) (bool, domain.SharePermission, bool, error) {
	if err := ctx.Err(); err != nil {
		return false, domain.PermissionRead, false, err
	}

	// Get the collection (use internal method to bypass ACL)
	collection, err := s.getCollectionInternal(ctx, collectionID)
	if err != nil {
		return false, domain.PermissionRead, false, err
	}

	// Check if user is the owner
	if collection.OwnerID == userID {
		return true, domain.PermissionWrite, true, nil
	}

	// Check if collection is shared with user
	share, err := s.GetShareForUserAndCollection(ctx, userID, collectionID)
	if err != nil {
		// No share found or error
		return false, domain.PermissionRead, false, nil
	}

	return true, share.Permission, false, nil
}
