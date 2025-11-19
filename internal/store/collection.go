package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
)

// Key prefixes for BadgerDB.
const (
	libraryPrefix    = "library:"
	collectionPrefix = "collection:"

	// Index Key.
	collectionsByLibraryPrefix = "idx:collections:library:"
)

var (
	// ErrLibraryNotFound is returned when a library is not found in the store.
	ErrLibraryNotFound = errors.New("library not found")
	// ErrCollectionNotFound is returned when a collection is not found in the store.
	ErrCollectionNotFound = errors.New("collection not found")
	// ErrDuplicateLibrary is returned when trying to create a library that already exists.
	ErrDuplicateLibrary = errors.New("library already exists")
	// ErrDuplicateCollection is returned when trying to create a collection that already exists.
	ErrDuplicateCollection = errors.New("collection already exists")
	// ErrPermissionDenied is returned when a user lacks permission for an operation.
	ErrPermissionDenied = errors.New("insufficient permissions")
)

// BootstrapResult contains the initialized library and collections.
type BootstrapResult struct {
	Library         *domain.Library
	InboxCollection *domain.Collection
	IsNewLibrary    bool
}

// EnsureLibrary ensures a library exists with the given scan path and owner.
// If no library exists, creates one with an inbox collection.
// If a library exists, adds the scan path if not already present.
// Returns the library and its inbox collection.
//
// This function is idempotent - calling it multiple times is safe:
//   - First call: Creates library + inbox collection.
//   - Subsequent calls: Returns existing library.
//   - New scan path: Adds to existing library.
//
// "There are other worlds than these" - but this one needs an owner.
func (s *Store) EnsureLibrary(ctx context.Context, scanPath string, userID string) (*BootstrapResult, error) {
	result := &BootstrapResult{}

	// Try to get existing library.
	library, err := s.GetDefaultLibrary(ctx)

	switch {
	case errors.Is(err, ErrLibraryNotFound):
		// No library exists - create everything from scratch.
		if s.logger != nil {
			s.logger.Info("no library found, creating new library",
				"scan_path", scanPath,
				"owner_id", userID,
			)
		}

		// Generate IDs.
		libraryID, err := id.Generate("lib")
		if err != nil {
			return nil, fmt.Errorf("generate library ID: %w", err)
		}

		inboxCollID, err := id.Generate("coll")
		if err != nil {
			return nil, fmt.Errorf("generate inbox collection ID: %w", err)
		}

		// Create library.
		library = &domain.Library{
			ID:        libraryID,
			OwnerID:   userID,
			Name:      "My Library",
			ScanPaths: []string{scanPath},
			SkipInbox: false, // Inbox enabled by default
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.CreateLibrary(ctx, library); err != nil {
			return nil, fmt.Errorf("create library: %w", err)
		}

		result.IsNewLibrary = true

		// Create inbox collection.
		inboxColl := &domain.Collection{
			ID:        inboxCollID,
			LibraryID: library.ID,
			OwnerID:   userID,
			Name:      "Inbox",
			IsInbox:   true,
			BookIDs:   []string{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.CreateCollection(ctx, inboxColl); err != nil {
			return nil, fmt.Errorf("create inbox collection: %w", err)
		}

		result.InboxCollection = inboxColl

		if s.logger != nil {
			s.logger.Info("library initialized",
				"library_id", library.ID,
				"owner_id", userID,
				"inbox_collection_id", inboxColl.ID,
			)
		}
	case err != nil:
		return nil, fmt.Errorf("get default library: %w", err)
	default:
		// Library exists - ensure scan path is included.
		result.IsNewLibrary = false

		hasPath := false
		for _, p := range library.ScanPaths {
			if p == scanPath {
				hasPath = true
				break
			}
		}

		if !hasPath {
			if s.logger != nil {
				s.logger.Info("adding scan path to existing library",
					"library_id", library.ID,
					"scan_path", scanPath,
				)
			}

			library.AddScanPath(scanPath)
			library.UpdatedAt = time.Now()

			if err := s.UpdateLibrary(ctx, library); err != nil {
				return nil, fmt.Errorf("update library: %w", err)
			}
		}

		// Get existing inbox collection.
		inboxColl, err := s.GetInboxForLibrary(ctx, library.ID)
		if err != nil {
			return nil, fmt.Errorf("get inbox collection: %w", err)
		}
		result.InboxCollection = inboxColl

		if s.logger != nil {
			s.logger.Info("using existing library",
				"library_id", library.ID,
				"owner_id", library.OwnerID,
				"scan_paths", len(library.ScanPaths),
			)
		}
	}

	result.Library = library
	return result, nil
}

// CreateLibrary creates a new library in the store.
func (s *Store) CreateLibrary(_ context.Context, lib *domain.Library) error {
	key := []byte(libraryPrefix + lib.ID)

	// Check if already exists.
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check library exists: %w", err)
	}
	if exists {
		return ErrDuplicateLibrary
	}

	if err := s.set(key, lib); err != nil {
		return fmt.Errorf("save library: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("library created",
			"id", lib.ID,
			"name", lib.Name,
			"owner_id", lib.OwnerID,
			"scan_paths", len(lib.ScanPaths),
		)
	}
	return nil
}

// GetLibrary retrieves a library by ID.
func (s *Store) GetLibrary(_ context.Context, id string) (*domain.Library, error) {
	key := []byte(libraryPrefix + id)

	var lib domain.Library
	if err := s.get(key, &lib); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrLibraryNotFound
		}
		return nil, fmt.Errorf("get library: %w", err)
	}

	return &lib, nil
}

// UpdateLibrary updates an existing library in the store.
func (s *Store) UpdateLibrary(_ context.Context, lib *domain.Library) error {
	key := []byte(libraryPrefix + lib.ID)

	// Verify exists.
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check library exists: %w", err)
	}
	if !exists {
		return ErrLibraryNotFound
	}

	if err := s.set(key, lib); err != nil {
		return fmt.Errorf("update library: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("library updated", "id", lib.ID, "name", lib.Name)
	}

	return nil
}

// DeleteLibrary deletes a library and all its collections.
// This is a destructive operation - use with caution.
func (s *Store) DeleteLibrary(ctx context.Context, id string) error {
	// Get all collections for this library.
	collections, err := s.ListAllCollectionsByLibrary(ctx, id)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}

	// Delete all collections first (including their shares).
	for _, coll := range collections {
		// Delete shares for this collection
		if err := s.DeleteSharesForCollection(ctx, coll.ID); err != nil {
			return fmt.Errorf("delete shares for collection %s: %w", coll.ID, err)
		}
		// Delete the collection itself (bypass ACL since we're deleting the whole library)
		if err := s.deleteCollectionInternal(ctx, coll.ID); err != nil {
			return fmt.Errorf("delete collection %s: %w", coll.ID, err)
		}
	}

	key := []byte(libraryPrefix + id)
	if err := s.delete(key); err != nil {
		return fmt.Errorf("delete library: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("library deleted", "id", id)
	}

	return nil
}

// ListLibraries returns all libraries in the store.
func (s *Store) ListLibraries(ctx context.Context) ([]*domain.Library, error) {
	var libraries []*domain.Library

	prefix := []byte(libraryPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var lib domain.Library
				if err := json.Unmarshal(val, &lib); err != nil {
					return err
				}
				libraries = append(libraries, &lib)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	return libraries, nil
}

// GetDefaultLibrary returns the first library in the store.
// In a multi-library future, this would need to be smarter.
func (s *Store) GetDefaultLibrary(ctx context.Context) (*domain.Library, error) {
	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}

	if len(libraries) == 0 {
		return nil, ErrLibraryNotFound
	}

	return libraries[0], nil
}

// CreateCollection creates a new collection in the store.
// Note: OwnerID should already be set on the collection.
// Validation happens in the service layer.
func (s *Store) CreateCollection(_ context.Context, coll *domain.Collection) error {
	key := []byte(collectionPrefix + coll.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}
	if exists {
		return ErrDuplicateCollection
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(coll)
		if err != nil {
			return fmt.Errorf("marshal collection: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create library index.
		indexKey := []byte(collectionsByLibraryPrefix + coll.LibraryID)
		var collectionIDs []string

		item, err := txn.Get(indexKey)
		if err == nil {
			err = item.Value(func(val []byte) error {
				return json.Unmarshal(val, &collectionIDs)
			})
			if err != nil {
				return err
			}
		}

		collectionIDs = append(collectionIDs, coll.ID)
		indexData, err := json.Marshal(collectionIDs)
		if err != nil {
			return err
		}

		if err := txn.Set(indexKey, indexData); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("collection created",
			"id", coll.ID,
			"name", coll.Name,
			"owner_id", coll.OwnerID,
			"is_inbox", coll.IsInbox,
			"library_id", coll.LibraryID,
		)
	}
	return nil
}

// GetCollection retrieves a collection by ID with access control.
// Returns ErrCollectionNotFound if collection doesn't exist OR user doesn't have access.
func (s *Store) GetCollection(ctx context.Context, id string, userID string) (*domain.Collection, error) {
	key := []byte(collectionPrefix + id)

	var coll domain.Collection
	if err := s.get(key, &coll); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrCollectionNotFound
		}
		return nil, fmt.Errorf("get collection: %w", err)
	}

	// Check if user has access to this collection.
	canAccess, _, _, err := s.CanUserAccessCollection(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !canAccess {
		// Don't leak that collection exists
		return nil, ErrCollectionNotFound
	}

	return &coll, nil
}

// getCollectionInternal retrieves a collection by ID without access control.
// For internal store use only.
func (s *Store) getCollectionInternal(_ context.Context, id string) (*domain.Collection, error) {
	key := []byte(collectionPrefix + id)

	var coll domain.Collection
	if err := s.get(key, &coll); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrCollectionNotFound
		}
		return nil, fmt.Errorf("get collection: %w", err)
	}

	return &coll, nil
}

// UpdateCollection updates an existing collection in the store.
// User must be owner OR have Write permission.
func (s *Store) UpdateCollection(ctx context.Context, coll *domain.Collection, userID string) error {
	key := []byte(collectionPrefix + coll.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}

	if !exists {
		return ErrCollectionNotFound
	}

	// Check permissions.
	canAccess, permission, isOwner, err := s.CanUserAccessCollection(ctx, userID, coll.ID)
	if err != nil {
		return err
	}
	if !canAccess {
		return ErrCollectionNotFound
	}
	if !isOwner && permission != domain.PermissionWrite {
		return ErrPermissionDenied
	}

	if err := s.set(key, coll); err != nil {
		return fmt.Errorf("update collection: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("collection updated",
			"id", coll.ID,
			"name", coll.Name,
			"user_id", userID,
		)
	}
	return nil
}

// updateCollectionInternal updates a collection without access control checks.
// For internal store use only.
func (s *Store) updateCollectionInternal(_ context.Context, coll *domain.Collection) error {
	key := []byte(collectionPrefix + coll.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}

	if !exists {
		return ErrCollectionNotFound
	}

	if err := s.set(key, coll); err != nil {
		return fmt.Errorf("update collection: %w", err)
	}

	return nil
}

// DeleteCollection deletes a collection from the store.
// User must be the owner (not just Write permission).
// Deletes all shares for the collection as well.
func (s *Store) DeleteCollection(ctx context.Context, id string, userID string) error {
	coll, err := s.getCollectionInternal(ctx, id)
	if err != nil {
		return err
	}

	// Only owner can delete
	if coll.OwnerID != userID {
		return ErrPermissionDenied
	}

	// Cannot delete system Inbox collection
	if coll.IsInbox {
		return errors.New("cannot delete system collection")
	}

	// Delete all shares for this collection
	if err := s.DeleteSharesForCollection(ctx, id); err != nil {
		return fmt.Errorf("delete shares: %w", err)
	}

	// Delete the collection
	if err := s.deleteCollectionInternal(ctx, id); err != nil {
		return err
	}

	if s.logger != nil {
		s.logger.Info("collection deleted",
			"id", id,
			"user_id", userID,
		)
	}

	return nil
}

// deleteCollectionInternal deletes a collection without access control checks.
// For internal store use only (e.g., when deleting a library).
func (s *Store) deleteCollectionInternal(ctx context.Context, id string) error {
	coll, err := s.getCollectionInternal(ctx, id)
	if err != nil {
		return err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		key := []byte(collectionPrefix + id)
		if err := txn.Delete(key); err != nil {
			return err
		}

		// Remove from library index
		indexKey := []byte(collectionsByLibraryPrefix + coll.LibraryID)
		var collectionIDs []string

		item, err := txn.Get(indexKey)
		if err != nil {
			return err
		}

		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &collectionIDs)
		})
		if err != nil {
			return err
		}

		collectionIDs = slices.DeleteFunc(collectionIDs, func(cid string) bool {
			return cid == id
		})

		indexData, err := json.Marshal(collectionIDs)
		if err != nil {
			return err
		}

		return txn.Set(indexKey, indexData)
	})
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}

	return nil
}

// ListCollectionsByLibrary returns collections for a library that the user can access.
// Filters by user ownership and shares.
func (s *Store) ListCollectionsByLibrary(ctx context.Context, libraryID string, userID string) ([]*domain.Collection, error) {
	// Get all collections for library
	allCollections, err := s.ListAllCollectionsByLibrary(ctx, libraryID)
	if err != nil {
		return nil, err
	}

	// Filter using GetCollectionsForUser
	userCollections, err := s.GetCollectionsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Create set of user's collection IDs for fast lookup
	userCollectionIDs := make(map[string]bool)
	for _, coll := range userCollections {
		userCollectionIDs[coll.ID] = true
	}

	// Filter to only collections user has access to
	var accessibleCollections []*domain.Collection
	for _, coll := range allCollections {
		if userCollectionIDs[coll.ID] {
			accessibleCollections = append(accessibleCollections, coll)
		}
	}

	return accessibleCollections, nil
}

// ListAllCollectionsByLibrary returns ALL collections for a library without access filtering.
// For internal store use only (finding Inbox, system operations, etc.).
func (s *Store) ListAllCollectionsByLibrary(ctx context.Context, libraryID string) ([]*domain.Collection, error) {
	indexKey := []byte(collectionsByLibraryPrefix + libraryID)

	var collectionIDs []string

	err := s.get(indexKey, &collectionIDs)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return []*domain.Collection{}, nil
		}
		return nil, fmt.Errorf("get collection index: %w", err)
	}

	collections := make([]*domain.Collection, 0, len(collectionIDs))
	for _, id := range collectionIDs {
		coll, err := s.getCollectionInternal(ctx, id)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to get collection", "id", id, "error", err)
			}
			continue
		}
		collections = append(collections, coll)
	}
	return collections, nil
}

// GetInboxForLibrary returns the Inbox collection for a library.
// The Inbox is the system collection where IsInbox = true.
func (s *Store) GetInboxForLibrary(ctx context.Context, libraryID string) (*domain.Collection, error) {
	collections, err := s.ListAllCollectionsByLibrary(ctx, libraryID)
	if err != nil {
		return nil, err
	}

	// Find the one with IsInbox = true
	for _, coll := range collections {
		if coll.IsInbox {
			return coll, nil
		}
	}

	return nil, ErrCollectionNotFound
}

// AddBookToCollection adds a book to a collection.
// User must have Write permission or be the owner.
func (s *Store) AddBookToCollection(ctx context.Context, bookID, collectionID string, userID string) error {
	// Get collection with access check
	coll, err := s.GetCollection(ctx, collectionID, userID)
	if err != nil {
		return err
	}

	// Check for Write permission
	_, permission, isOwner, err := s.CanUserAccessCollection(ctx, userID, collectionID)
	if err != nil {
		return err
	}
	if !isOwner && permission != domain.PermissionWrite {
		return ErrPermissionDenied
	}

	// Use collection's AddBook helper method
	if !coll.AddBook(bookID) {
		// Book already in collection
		return nil
	}

	return s.UpdateCollection(ctx, coll, userID)
}

// RemoveBookFromCollection removes a book ID from a collection.
// User must have Write permission or be the owner.
func (s *Store) RemoveBookFromCollection(ctx context.Context, bookID, collectionID string, userID string) error {
	// Get collection with access check
	coll, err := s.GetCollection(ctx, collectionID, userID)
	if err != nil {
		return err
	}

	// Check for Write permission
	_, permission, isOwner, err := s.CanUserAccessCollection(ctx, userID, collectionID)
	if err != nil {
		return err
	}
	if !isOwner && permission != domain.PermissionWrite {
		return ErrPermissionDenied
	}

	// Use collection's RemoveBook helper method
	if !coll.RemoveBook(bookID) {
		// Book not in collection
		return nil
	}

	return s.UpdateCollection(ctx, coll, userID)
}

// removeBookFromCollectionInternal removes a book from a collection without ACL checks.
// For internal system use only (e.g., during book deletion).
func (s *Store) removeBookFromCollectionInternal(ctx context.Context, collectionID, bookID string) error {
	// Get collection without access check
	coll, err := s.getCollectionInternal(ctx, collectionID)
	if err != nil {
		return err
	}

	// Use collection's RemoveBook helper method
	if !coll.RemoveBook(bookID) {
		// Book not in collection
		return nil
	}

	// Update collection without access check
	return s.updateCollectionInternal(ctx, coll)
}

// GetCollectionsForBook returns all collections containing a specific book.
// No access control - used for determining if a book is uncollected.
func (s *Store) GetCollectionsForBook(ctx context.Context, bookID string) ([]*domain.Collection, error) {
	var matchingCollections []*domain.Collection

	// Scan all collections (no reverse index yet).
	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}

	for _, lib := range libraries {
		collections, err := s.ListAllCollectionsByLibrary(ctx, lib.ID)
		if err != nil {
			continue
		}

		for _, coll := range collections {
			if coll.ContainsBook(bookID) {
				matchingCollections = append(matchingCollections, coll)
			}
		}
	}
	return matchingCollections, nil
}