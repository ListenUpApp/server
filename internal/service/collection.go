// Package service provides business logic layer for managing audiobooks, libraries, and synchronization.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
)

// collectionServiceStore is the narrow store surface CollectionService needs:
// CollectionStore, plus library lookup, user lookup (for admin checks), and
// book reads (for in-collection access checks).
type collectionServiceStore interface {
	store.CollectionStore
	GetLibrary(ctx context.Context, id string) (*domain.Library, error)
	GetDefaultLibrary(ctx context.Context) (*domain.Library, error)
	GetUser(ctx context.Context, id string) (*domain.User, error)
	GetBook(ctx context.Context, id string, userID string) (*domain.Book, error)
}

// CollectionService orchestrates collection operations with ACL enforcement.
type CollectionService struct {
	store  collectionServiceStore
	logger *slog.Logger
}

// NewCollectionService creates a new collection service.
func NewCollectionService(s collectionServiceStore, logger *slog.Logger) *CollectionService {
	return &CollectionService{
		store:  s,
		logger: logger,
	}
}

// CreateCollectionOptions contains optional parameters for collection creation.
type CreateCollectionOptions struct {
	IsGlobalAccess bool // When shared, grants access to ALL books (admin only)
}

// CreateCollection creates a new collection for the user.
// The user becomes the owner and has full write access.
func (s *CollectionService) CreateCollection(ctx context.Context, userID, libraryID, name string) (*domain.Collection, error) {
	return s.CreateCollectionWithOptions(ctx, userID, libraryID, name, CreateCollectionOptions{})
}

// CreateCollectionWithOptions creates a new collection with optional settings.
// Only admins can create global access collections.
func (s *CollectionService) CreateCollectionWithOptions(ctx context.Context, userID, libraryID, name string, opts CreateCollectionOptions) (*domain.Collection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Verify library exists
	if _, err := s.store.GetLibrary(ctx, libraryID); err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	// Only admins can create global access collections
	if opts.IsGlobalAccess {
		user, err := s.store.GetUser(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("get user: %w", err)
		}
		if !user.IsAdmin() {
			return nil, errors.New("only admins can create global access collections")
		}
	}

	// Generate collection ID
	collectionID, err := id.Generate("coll")
	if err != nil {
		return nil, fmt.Errorf("generate collection ID: %w", err)
	}

	collection := &domain.Collection{
		ID:             collectionID,
		LibraryID:      libraryID,
		OwnerID:        userID,
		Name:           name,
		BookIDs:        []string{},
		IsInbox:        false,
		IsGlobalAccess: opts.IsGlobalAccess,
	}

	if err := s.store.CreateCollection(ctx, collection); err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}

	s.logger.Info("collection created",
		"collection_id", collectionID,
		"library_id", libraryID,
		"owner_id", userID,
		"name", name,
		"is_global_access", opts.IsGlobalAccess,
	)

	return collection, nil
}

// GetCollection retrieves a collection by ID.
// Returns ErrCollectionNotFound if user doesn't have access.
func (s *CollectionService) GetCollection(ctx context.Context, userID, collectionID string) (*domain.Collection, error) {
	return s.store.GetCollection(ctx, collectionID, userID)
}

// ListCollections returns all collections the user can access in a library.
// This includes collections they own and collections shared with them.
func (s *CollectionService) ListCollections(ctx context.Context, userID, libraryID string) ([]*domain.Collection, error) {
	return s.store.ListCollectionsByLibrary(ctx, libraryID, userID)
}

// UpdateCollection updates collection metadata (name only for now).
// Requires Write permission on the collection.
func (s *CollectionService) UpdateCollection(ctx context.Context, userID, collectionID, newName string) (*domain.Collection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get collection with ACL check
	collection, err := s.store.GetCollection(ctx, collectionID, userID)
	if err != nil {
		return nil, err
	}

	// Update name
	collection.Name = newName

	// UpdateCollection in store will check Write permission
	if err := s.store.UpdateCollection(ctx, collection, userID); err != nil {
		return nil, fmt.Errorf("update collection: %w", err)
	}

	s.logger.Info("collection updated",
		"collection_id", collectionID,
		"user_id", userID,
		"new_name", newName,
	)

	return collection, nil
}

// DeleteCollection deletes a collection.
// Only the owner can delete a collection.
func (s *CollectionService) DeleteCollection(ctx context.Context, userID, collectionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// DeleteCollection in store will verify ownership
	if err := s.store.DeleteCollection(ctx, collectionID, userID); err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}

	s.logger.Info("collection deleted",
		"collection_id", collectionID,
		"user_id", userID,
	)

	return nil
}

// AddBookToCollection adds a book to a collection.
// Requires Write permission on the collection.
func (s *CollectionService) AddBookToCollection(ctx context.Context, userID, collectionID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Verify book exists (use internal method to avoid ACL chicken-egg problem)
	// The user needs Write access to the collection, which is checked in AddBookToCollection
	if _, err := s.store.GetBook(ctx, bookID, userID); err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	// AddBookToCollection will check Write permission
	if err := s.store.AddBookToCollection(ctx, collectionID, bookID, userID); err != nil {
		return fmt.Errorf("add book to collection: %w", err)
	}

	s.logger.Info("book added to collection",
		"collection_id", collectionID,
		"book_id", bookID,
		"user_id", userID,
	)

	return nil
}

// RemoveBookFromCollection removes a book from a collection.
// Requires Write permission on the collection.
func (s *CollectionService) RemoveBookFromCollection(ctx context.Context, userID, collectionID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// RemoveBookFromCollection will check Write permission
	if err := s.store.RemoveBookFromCollection(ctx, collectionID, bookID, userID); err != nil {
		return fmt.Errorf("remove book from collection: %w", err)
	}

	s.logger.Info("book removed from collection",
		"collection_id", collectionID,
		"book_id", bookID,
		"user_id", userID,
	)

	return nil
}

// GetCollectionBooks returns all books in a collection.
// User must have Read access to the collection.
func (s *CollectionService) GetCollectionBooks(ctx context.Context, userID, collectionID string) ([]*domain.Book, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get collection with ACL check (verifies user has access)
	collection, err := s.store.GetCollection(ctx, collectionID, userID)
	if err != nil {
		return nil, err
	}

	// Get all books in the collection
	var books []*domain.Book
	for _, bookID := range collection.BookIDs {
		book, err := s.store.GetBook(ctx, bookID, userID)
		if err != nil {
			// If user can't access a book in the collection (edge case),
			// log and skip it rather than failing the entire request
			s.logger.Warn("user cannot access book in collection",
				"user_id", userID,
				"collection_id", collectionID,
				"book_id", bookID,
				"error", err,
			)
			continue
		}
		books = append(books, book)
	}

	return books, nil
}

// ============================================================
// Admin methods (no ACL — callers must already be admins)
// ============================================================

// AdminListAllCollections returns every collection in the system.
func (s *CollectionService) AdminListAllCollections(ctx context.Context) ([]*domain.Collection, error) {
	return s.store.AdminListAllCollections(ctx)
}

// AdminGetCollection fetches a collection by ID without ACL enforcement.
func (s *CollectionService) AdminGetCollection(ctx context.Context, collectionID string) (*domain.Collection, error) {
	return s.store.AdminGetCollection(ctx, collectionID)
}

// AdminCreateCollectionResult holds the output of AdminCreateCollection.
type AdminCreateCollectionResult struct {
	Collection *domain.Collection
}

// AdminCreateCollection creates a new collection using the default library when libraryID is empty.
// Returns ErrDefaultLibraryNotFound (wrapping store.ErrNotFound) if no library exists.
func (s *CollectionService) AdminCreateCollection(ctx context.Context, ownerID, libraryID, name string) (*domain.Collection, error) {
	if libraryID == "" {
		lib, err := s.store.GetDefaultLibrary(ctx)
		if err != nil {
			return nil, fmt.Errorf("get default library: %w", err)
		}
		libraryID = lib.ID
	}

	collID, err := id.Generate("coll")
	if err != nil {
		return nil, fmt.Errorf("generate collection ID: %w", err)
	}

	now := time.Now()
	coll := &domain.Collection{
		ID:        collID,
		LibraryID: libraryID,
		OwnerID:   ownerID,
		Name:      name,
		BookIDs:   []string{},
		IsInbox:   false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.store.CreateCollection(ctx, coll); err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}

	s.logger.Info("admin created collection",
		"collection_id", collID,
		"library_id", libraryID,
		"owner_id", ownerID,
		"name", name,
	)

	return coll, nil
}

// AdminUpdateCollectionName fetches a collection and applies a name change without ACL enforcement.
func (s *CollectionService) AdminUpdateCollectionName(ctx context.Context, collectionID string, name *string) (*domain.Collection, error) {
	coll, err := s.store.AdminGetCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	if name != nil {
		coll.Name = *name
	}
	coll.UpdatedAt = time.Now()

	if err := s.store.AdminUpdateCollection(ctx, coll); err != nil {
		return nil, fmt.Errorf("admin update collection: %w", err)
	}

	s.logger.Info("admin updated collection",
		"collection_id", collectionID,
		"name", coll.Name,
	)

	return coll, nil
}

// AdminDeleteCollectionResult carries the pre-deletion state needed by the handler for SSE.
type AdminDeleteCollectionResult struct {
	// Collection is the collection as it existed before deletion.
	Collection *domain.Collection
	// BooksBecomingPublic lists book IDs that were only in this collection and
	// will be visible to all users after it is removed.
	BooksBecomingPublic []string
	// MemberUserIDs is the set of user IDs (owner + share recipients) who had
	// access to the collection; used to target EmitToNonMembers.
	MemberUserIDs map[string]bool
}

// AdminDeleteCollection removes a collection and gathers the information the
// caller needs to emit SSE notifications. It does NOT prevent deleting inbox
// collections — the caller must check Collection.IsInbox before proceeding.
func (s *CollectionService) AdminDeleteCollection(ctx context.Context, collectionID string) (*AdminDeleteCollectionResult, error) {
	coll, err := s.store.AdminGetCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	// Determine which books will become publicly visible after deletion.
	var booksBecomingPublic []string
	for _, bookID := range coll.BookIDs {
		colls, err := s.store.GetCollectionsForBook(ctx, bookID)
		if err != nil {
			continue
		}
		if len(colls) == 1 && colls[0].ID == collectionID {
			booksBecomingPublic = append(booksBecomingPublic, bookID)
		}
	}

	// Build member set (owner + share recipients) for non-member SSE targeting.
	memberUserIDs := make(map[string]bool)
	if len(booksBecomingPublic) > 0 {
		memberUserIDs[coll.OwnerID] = true
		shares, _ := s.store.GetSharesForCollection(ctx, collectionID)
		for _, share := range shares {
			memberUserIDs[share.SharedWithUserID] = true
		}
	}

	if err := s.store.AdminDeleteCollection(ctx, collectionID); err != nil {
		return nil, fmt.Errorf("admin delete collection: %w", err)
	}

	s.logger.Info("admin deleted collection",
		"collection_id", collectionID,
		"books_becoming_public", len(booksBecomingPublic),
	)

	return &AdminDeleteCollectionResult{
		Collection:          coll,
		BooksBecomingPublic: booksBecomingPublic,
		MemberUserIDs:       memberUserIDs,
	}, nil
}

// AddBookToCollectionResult carries per-book outcome for admin batch-add.
type AddBookToCollectionResult struct {
	BookID         string
	WasUncollected bool // true if the book was public before this operation
	Added          bool // false if the store returned an error
}

// AdminAddBooksToCollectionResult is the output of AdminAddBooksToCollection.
type AdminAddBooksToCollectionResult struct {
	// Collection is the collection as it existed before the books were added.
	Collection *domain.Collection
	// MemberUserIDs is the set of users who have access to the collection (owner + share recipients).
	MemberUserIDs map[string]bool
	// Results contains per-book outcomes in input order.
	Results []AddBookToCollectionResult
}

// AdminAddBooksToCollection adds a list of books to a collection without ACL enforcement.
// It collects per-book results so the caller can emit appropriate SSE events.
func (s *CollectionService) AdminAddBooksToCollection(ctx context.Context, collectionID string, bookIDs []string) (*AdminAddBooksToCollectionResult, error) {
	coll, err := s.store.AdminGetCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	// Build member set for SSE targeting.
	memberUserIDs := make(map[string]bool)
	memberUserIDs[coll.OwnerID] = true
	shares, err := s.store.GetSharesForCollection(ctx, collectionID)
	if err == nil {
		for _, share := range shares {
			memberUserIDs[share.SharedWithUserID] = true
		}
	}

	results := make([]AddBookToCollectionResult, 0, len(bookIDs))
	for _, bookID := range bookIDs {
		existingColls, _ := s.store.GetCollectionsForBook(ctx, bookID)
		wasUncollected := len(existingColls) == 0

		addErr := s.store.AdminAddBookToCollection(ctx, bookID, collectionID)
		if addErr != nil {
			s.logger.Warn("admin failed to add book to collection",
				"book_id", bookID,
				"collection_id", collectionID,
				"error", addErr,
			)
			results = append(results, AddBookToCollectionResult{BookID: bookID, WasUncollected: wasUncollected, Added: false})
			continue
		}
		results = append(results, AddBookToCollectionResult{BookID: bookID, WasUncollected: wasUncollected, Added: true})
	}

	return &AdminAddBooksToCollectionResult{
		Collection:    coll,
		MemberUserIDs: memberUserIDs,
		Results:       results,
	}, nil
}

// AdminRemoveBookFromCollectionResult carries the pre-removal state needed for SSE.
type AdminRemoveBookFromCollectionResult struct {
	// Collection is the collection as it existed before removal.
	Collection *domain.Collection
	// WillBecomePublic is true if the book was only in this collection and will
	// become visible to all users after removal.
	WillBecomePublic bool
	// MemberUserIDs is the set of users who had access (owner + share recipients).
	// Populated only when WillBecomePublic is true.
	MemberUserIDs map[string]bool
}

// AdminRemoveBookFromCollection removes a book from a collection without ACL enforcement.
func (s *CollectionService) AdminRemoveBookFromCollection(ctx context.Context, collectionID, bookID string) (*AdminRemoveBookFromCollectionResult, error) {
	coll, err := s.store.AdminGetCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}

	existingColls, _ := s.store.GetCollectionsForBook(ctx, bookID)
	willBecomePublic := len(existingColls) == 1 && existingColls[0].ID == collectionID

	memberUserIDs := make(map[string]bool)
	if willBecomePublic {
		memberUserIDs[coll.OwnerID] = true
		shares, _ := s.store.GetSharesForCollection(ctx, collectionID)
		for _, share := range shares {
			memberUserIDs[share.SharedWithUserID] = true
		}
	}

	if err := s.store.AdminRemoveBookFromCollection(ctx, bookID, collectionID); err != nil {
		return nil, fmt.Errorf("admin remove book from collection: %w", err)
	}

	s.logger.Info("admin removed book from collection",
		"collection_id", collectionID,
		"book_id", bookID,
		"will_become_public", willBecomePublic,
	)

	return &AdminRemoveBookFromCollectionResult{
		Collection:       coll,
		WillBecomePublic: willBecomePublic,
		MemberUserIDs:    memberUserIDs,
	}, nil
}

// ============================================================
// Helpers used by share SSE logic in the handler layer
// ============================================================

// GetBook returns a book visible to ownerID (no user-facing ACL; used for SSE enrichment).
func (s *CollectionService) GetBook(ctx context.Context, bookID, ownerID string) (*domain.Book, error) {
	return s.store.GetBook(ctx, bookID, ownerID)
}

// CanUserAccessBook reports whether userID can access bookID.
func (s *CollectionService) CanUserAccessBook(ctx context.Context, userID, bookID string) (bool, error) {
	return s.store.CanUserAccessBook(ctx, userID, bookID)
}
