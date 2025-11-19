// Package service provides business logic layer for managing audiobooks, libraries, and synchronization.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
)

// CollectionService orchestrates collection operations with ACL enforcement.
type CollectionService struct {
	store  *store.Store
	logger *slog.Logger
}

// NewCollectionService creates a new collection service.
func NewCollectionService(store *store.Store, logger *slog.Logger) *CollectionService {
	return &CollectionService{
		store:  store,
		logger: logger,
	}
}

// CreateCollection creates a new collection for the user.
// The user becomes the owner and has full write access.
func (s *CollectionService) CreateCollection(ctx context.Context, userID, libraryID, name string) (*domain.Collection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Verify library exists
	if _, err := s.store.GetLibrary(ctx, libraryID); err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	// Generate collection ID
	collectionID, err := id.Generate("coll")
	if err != nil {
		return nil, fmt.Errorf("generate collection ID: %w", err)
	}

	collection := &domain.Collection{
		ID:        collectionID,
		LibraryID: libraryID,
		OwnerID:   userID,
		Name:      name,
		BookIDs:   []string{},
		IsInbox:   false,
	}

	if err := s.store.CreateCollection(ctx, collection); err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}

	s.logger.Info("collection created",
		"collection_id", collectionID,
		"library_id", libraryID,
		"owner_id", userID,
		"name", name,
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