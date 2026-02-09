package store

import (
	"cmp"
	"context"
	"errors"
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

// GetBooksForUser returns all books the user can access based on library access mode.
//
// In OPEN mode (default):
//   - Uncollected books are visible to everyone
//   - Books in collections are only visible to collection members
//   - This is the existing "permissive" behavior
//
// In RESTRICTED mode:
//   - No books are visible by default
//   - Users only see books in collections they have access to
//   - Users with access to a GlobalAccess collection see everything
//
// Uses reverse indexes for O(Collections + BookIDs) instead of O(Books × Collections).
func (s *Store) GetBooksForUser(ctx context.Context, userID string) ([]*domain.Book, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get library to check access mode (defaults to open mode if no library exists)
	library, err := s.GetDefaultLibrary(ctx)
	if err != nil {
		// No library exists - use a default "open" library for access checks
		library = &domain.Library{AccessMode: domain.AccessModeOpen}
	}

	// Get collections user has access to
	userCollections, err := s.GetCollectionsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user collections: %w", err)
	}

	// Check if user has global access (via GlobalAccess collection)
	hasGlobalAccess := false
	for _, coll := range userCollections {
		if coll.IsGlobalAccess {
			hasGlobalAccess = true
			break
		}
	}

	// Determine effective access based on library mode
	var accessibleBookIDs map[string]bool

	switch library.GetAccessMode() {
	case domain.AccessModeRestricted:
		if hasGlobalAccess {
			// User has global access - return all books
			accessibleBookIDs, err = s.getAllBookIDs(ctx)
			if err != nil {
				return nil, fmt.Errorf("get all book IDs: %w", err)
			}
		} else {
			// User only sees books from their collections
			accessibleBookIDs, err = s.getBookIDsFromCollections(ctx, userCollections)
			if err != nil {
				return nil, fmt.Errorf("get books from collections: %w", err)
			}
		}

	default: // AccessModeOpen
		if hasGlobalAccess {
			// Global access in open mode - see everything
			accessibleBookIDs, err = s.getAllBookIDs(ctx)
			if err != nil {
				return nil, fmt.Errorf("get all book IDs: %w", err)
			}
		} else {
			// Existing permissive logic: uncollected + collection-accessible books
			accessibleBookIDs, err = s.getAccessibleBookIDsOpenMode(ctx, userCollections)
			if err != nil {
				return nil, fmt.Errorf("get accessible books (open mode): %w", err)
			}
		}
	}

	// Filter out inbox books
	inboxBookIDs, err := s.getInboxBookIDs(ctx, library.ID)
	if err != nil {
		return nil, fmt.Errorf("get inbox book IDs: %w", err)
	}

	// Build list of book IDs to fetch (excluding inbox books)
	bookIDsToFetch := make([]string, 0, len(accessibleBookIDs))
	for bookID := range accessibleBookIDs {
		if inboxBookIDs[bookID] {
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

// getAllBookIDs returns all book IDs in the database.
func (s *Store) getAllBookIDs(ctx context.Context) (map[string]bool, error) {
	bookIDs := make(map[string]bool)
	bookPrefix := []byte("book:")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = bookPrefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(bookPrefix); it.ValidForPrefix(bookPrefix); it.Next() {
			key := it.Item().Key()
			bookID := string(key[len(bookPrefix):])
			bookIDs[bookID] = true
		}
		return nil
	})

	return bookIDs, err
}

// getBookIDsFromCollections returns book IDs from the given collections.
func (s *Store) getBookIDsFromCollections(ctx context.Context, collections []*domain.Collection) (map[string]bool, error) {
	bookIDs := make(map[string]bool)

	err := s.db.View(func(txn *badger.Txn) error {
		for _, coll := range collections {
			// Skip inbox and global access collections (they don't contain actual books)
			if coll.IsInbox || coll.IsGlobalAccess {
				continue
			}

			prefix := fmt.Appendf(nil, "%s%s:", bookCollectionsPrefix, coll.ID)

			opts := badger.DefaultIteratorOptions
			opts.PrefetchValues = false
			opts.Prefix = prefix

			it := txn.NewIterator(opts)

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				key := string(it.Item().Key())
				if lastColon := strings.LastIndexByte(key, ':'); lastColon != -1 && lastColon < len(key)-1 {
					bookIDs[key[lastColon+1:]] = true
				}
			}
			it.Close()
		}
		return nil
	})

	return bookIDs, err
}

// getAccessibleBookIDsOpenMode implements the existing permissive access logic.
// A book is accessible if: (1) uncollected, OR (2) in a user-accessible collection.
func (s *Store) getAccessibleBookIDsOpenMode(ctx context.Context, userCollections []*domain.Collection) (map[string]bool, error) {
	accessibleBookIDs := make(map[string]bool)

	// Get books from user's collections
	err := s.db.View(func(txn *badger.Txn) error {
		for _, coll := range userCollections {
			if coll.IsInbox || coll.IsGlobalAccess {
				continue
			}

			prefix := fmt.Appendf(nil, "%s%s:", bookCollectionsPrefix, coll.ID)

			opts := badger.DefaultIteratorOptions
			opts.PrefetchValues = false
			opts.Prefix = prefix

			it := txn.NewIterator(opts)

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				key := string(it.Item().Key())
				if lastColon := strings.LastIndexByte(key, ':'); lastColon != -1 && lastColon < len(key)-1 {
					accessibleBookIDs[key[lastColon+1:]] = true
				}
			}
			it.Close()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Add uncollected books (books with no collection index entries)
	bookPrefix := []byte("book:")
	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = bookPrefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(bookPrefix); it.ValidForPrefix(bookPrefix); it.Next() {
			key := it.Item().Key()
			bookID := string(key[len(bookPrefix):])

			// Check if book is in any collection
			checkPrefix := fmt.Appendf(nil, "%s%s:", collectionBooksPrefix, bookID)

			checkOpts := badger.DefaultIteratorOptions
			checkOpts.PrefetchValues = false
			checkOpts.Prefix = checkPrefix

			checkIt := txn.NewIterator(checkOpts)
			checkIt.Seek(checkPrefix)
			hasIndex := checkIt.ValidForPrefix(checkPrefix)
			checkIt.Close()

			if !hasIndex {
				// Book is uncollected -> public in open mode
				accessibleBookIDs[bookID] = true
			}
		}
		return nil
	})

	return accessibleBookIDs, err
}

// getInboxBookIDs returns all book IDs in the inbox collection.
func (s *Store) getInboxBookIDs(ctx context.Context, libraryID string) (map[string]bool, error) {
	inboxBookIDs := make(map[string]bool)

	inbox, err := s.GetInboxForLibrary(ctx, libraryID)
	if err != nil {
		return inboxBookIDs, nil // No inbox, no filtering needed
	}

	for _, bookID := range inbox.BookIDs {
		inboxBookIDs[bookID] = true
	}

	return inboxBookIDs, nil
}

// CanUserAccessBook checks if a user can see a specific book.
// Logic depends on library access mode:
//   - OPEN: book is uncollected OR user has access to a containing collection
//   - RESTRICTED: user has global access OR access to a containing collection
//
// IMPORTANT: Inbox books are NEVER accessible, even with global access.
func (s *Store) CanUserAccessBook(ctx context.Context, userID, bookID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// Verify book exists (use internal method to bypass ACL)
	_, err := s.getBookInternal(ctx, bookID)
	if err != nil {
		return false, err
	}

	// Get library access mode (defaults to open mode if no library exists)
	library, err := s.GetDefaultLibrary(ctx)
	if err != nil {
		// No library exists - use default open mode
		library = &domain.Library{AccessMode: domain.AccessModeOpen}
	}

	// SECURITY: Check if book is in inbox - inbox books are NEVER accessible
	inboxBookIDs, err := s.getInboxBookIDs(ctx, library.ID)
	if err != nil {
		return false, fmt.Errorf("get inbox book IDs: %w", err)
	}
	if inboxBookIDs[bookID] {
		return false, nil // Inbox books blocked for everyone
	}

	// Get user's collections
	userCollections, err := s.GetCollectionsForUser(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("get user collections: %w", err)
	}

	// Build lookup map and check for global access
	userCollectionIDs := make(map[string]bool)
	hasGlobalAccess := false
	for _, coll := range userCollections {
		userCollectionIDs[coll.ID] = true
		if coll.IsGlobalAccess {
			hasGlobalAccess = true
		}
	}

	// Global access grants access to everything (except inbox, checked above)
	if hasGlobalAccess {
		return true, nil
	}

	// Get collections containing this book
	bookCollectionIDs, err := s.getCollectionIDsForBook(ctx, bookID)
	if err != nil {
		return false, fmt.Errorf("get book collections: %w", err)
	}

	// Check access based on mode
	switch library.GetAccessMode() {
	case domain.AccessModeRestricted:
		// Must have access to at least one containing collection
		for _, collID := range bookCollectionIDs {
			if userCollectionIDs[collID] {
				return true, nil
			}
		}
		return false, nil

	default: // AccessModeOpen
		// Uncollected books are public
		if len(bookCollectionIDs) == 0 {
			return true, nil
		}
		// Otherwise, must have access to at least one containing collection
		for _, collID := range bookCollectionIDs {
			if userCollectionIDs[collID] {
				return true, nil
			}
		}
		return false, nil
	}
}

// getCollectionIDsForBook returns all collection IDs containing a book.
func (s *Store) getCollectionIDsForBook(ctx context.Context, bookID string) ([]string, error) {
	var collectionIDs []string
	prefix := fmt.Appendf(nil, "%s%s:", collectionBooksPrefix, bookID)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			if lastColon := strings.LastIndexByte(key, ':'); lastColon != -1 && lastColon < len(key)-1 {
				collectionIDs = append(collectionIDs, key[lastColon+1:])
			}
		}
		return nil
	})

	return collectionIDs, err
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
		// Don't propagate ErrCollectionNotFound - return false to avoid leaking existence
		if errors.Is(err, ErrCollectionNotFound) {
			return false, domain.PermissionRead, false, nil
		}
		return false, domain.PermissionRead, false, err
	}

	// Check if user is the owner
	if collection.OwnerID == userID {
		return true, domain.PermissionWrite, true, nil
	}

	// Check if collection is shared with user
	share, err := s.GetShareForUserAndCollection(ctx, userID, collectionID)
	if err != nil {
		// Share not found or other error - return false without permission
		// Don't propagate ErrShareNotFound to avoid leaking collection existence
		if errors.Is(err, ErrShareNotFound) {
			return false, domain.PermissionRead, false, nil
		}
		// Real errors (db failure, etc.) should propagate
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

// GetAccessibleBookIDSet returns the set of book IDs the user can access.
// This is a convenience wrapper around GetBooksForUser that returns just IDs
// as a map for efficient lookups.
func (s *Store) GetAccessibleBookIDSet(ctx context.Context, userID string) (map[string]bool, error) {
	books, err := s.GetBooksForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(books))
	for _, b := range books {
		ids[b.ID] = true
	}
	return ids, nil
}
