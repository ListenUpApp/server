package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

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
//
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
	// Single transaction for all collection scans
	accessibleBookIDs := make(map[string]bool)

	err = s.db.View(func(txn *badger.Txn) error {
		for _, coll := range userCollections {
			// Scan idx:books:collections:{collectionID}:{bookID}
			prefix := fmt.Appendf(nil, "%s%s:", bookCollectionsPrefix, coll.ID)

			opts := badger.DefaultIteratorOptions
			opts.PrefetchValues = false // Only need keys
			opts.Prefix = prefix

			it := txn.NewIterator(opts)

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				key := string(it.Item().Key())
				// Key format: idx:books:collections:{collectionID}:{bookID}
				// Extract bookID (everything after last colon)
				if lastColon := strings.LastIndexByte(key, ':'); lastColon != -1 && lastColon < len(key)-1 {
					accessibleBookIDs[key[lastColon+1:]] = true
				}
			}
			it.Close()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan books in collections: %w", err)
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
			checkPrefix := fmt.Appendf(nil, "%s%s:", collectionBooksPrefix, bookID)

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

	// Get inbox collection to filter out staging books
	// Books in Inbox are hidden from all users until released
	library, _ := s.GetDefaultLibrary(ctx)
	var inboxBookIDs map[string]bool
	if library != nil {
		inbox, err := s.GetInboxForLibrary(ctx, library.ID)
		if err == nil && inbox != nil {
			inboxBookIDs = make(map[string]bool, len(inbox.BookIDs))
			for _, bookID := range inbox.BookIDs {
				inboxBookIDs[bookID] = true
			}
		}
	}

	// Build list of book IDs to fetch (excluding inbox books)
	bookIDsToFetch := make([]string, 0, len(accessibleBookIDs))
	for bookID := range accessibleBookIDs {
		// Skip books in inbox (staging area)
		if inboxBookIDs != nil && inboxBookIDs[bookID] {
			continue
		}
		bookIDsToFetch = append(bookIDsToFetch, bookID)
	}

	// Batch fetch all accessible books in a single transaction
	accessibleBooks, err := s.getBooksInternalByIDs(ctx, bookIDsToFetch)
	if err != nil {
		return nil, fmt.Errorf("load accessible books: %w", err)
	}

	// Sort by ID for deterministic pagination order.
	// This is critical: Go maps iterate in random order, so without sorting,
	// paginated sync would return overlapping/missing books across pages.
	slices.SortFunc(accessibleBooks, func(a, b *domain.Book) int {
		return cmp.Compare(a.ID, b.ID)
	})

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
	prefix := fmt.Appendf(nil, "%s%s:", collectionBooksPrefix, bookID)

	var bookCollectionIDs []string
	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			// Extract collectionID from key
			if lastColon := strings.LastIndexByte(key, ':'); lastColon != -1 && lastColon < len(key)-1 {
				bookCollectionIDs = append(bookCollectionIDs, key[lastColon+1:])
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
		// No share found - return false without permission
		return false, domain.PermissionRead, false, err
	}
	if share == nil {
		return false, domain.PermissionRead, false, nil
	}

	return true, share.Permission, false, nil
}

// GetBooksForUserUpdatedAfter efficiently retrieves books accessible to the user that have been
// updated after the specified timestamp. It uses the global updated_at index to find candidates
// and then filters them by user access (uncollected or shared via collection).
//
// This is optimized for delta sync where the number of updated books is usually small compared
// to the total library size.
func (s *Store) GetBooksForUserUpdatedAfter(ctx context.Context, userID string, timestamp time.Time) ([]*domain.Book, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get inbox collection to filter out staging books
	// Books in Inbox are hidden from all users until released
	library, _ := s.GetDefaultLibrary(ctx)
	var inboxBookIDs map[string]bool
	if library != nil {
		inbox, err := s.GetInboxForLibrary(ctx, library.ID)
		if err == nil && inbox != nil {
			inboxBookIDs = make(map[string]bool, len(inbox.BookIDs))
			for _, bookID := range inbox.BookIDs {
				inboxBookIDs[bookID] = true
			}
		}
	}

	var accessibleBooks []*domain.Book

	// Iterate over the global updated_at index starting from timestamp
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only need keys
		opts.Prefix = []byte(bookByUpdatedAtPrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		// Seek to the timestamp
		seekKey := formatTimestampIndexKey(bookByUpdatedAtPrefix, timestamp, "", "")

		for it.Seek(seekKey); it.ValidForPrefix([]byte(bookByUpdatedAtPrefix)); it.Next() {
			key := it.Item().Key()

			// Parse key to get book ID: idx:books:updated_at:{RFC3339Nano}:book:{uuid}
			// We can use the existing parseTimestampIndexKey helper or parse manually
			entityType, bookID, err := parseTimestampIndexKey(key, bookByUpdatedAtPrefix)
			if err != nil {
				// Skip invalid keys
				continue
			}

			// We only care about books (though currently this index only has books)
			if entityType != "book" {
				continue
			}

			// Skip books in inbox (staging area)
			if inboxBookIDs != nil && inboxBookIDs[bookID] {
				continue
			}

			// Check if user has access to this book
			// This uses the reverse index (O(1) relative to library size)
			canAccess, err := s.CanUserAccessBook(ctx, userID, bookID)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to check access for book during delta sync", "book_id", bookID, "error", err)
				}
				continue
			}

			if canAccess {
				// Fetch the actual book
				book, err := s.getBookInternal(ctx, bookID)
				if err != nil {
					continue
				}
				// Double check timestamp (should be redundant if index is correct, but safe)
				if book.UpdatedAt.After(timestamp) {
					accessibleBooks = append(accessibleBooks, book)
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("get updated books: %w", err)
	}

	return accessibleBooks, nil
}
