package store

import (
	"context"
	"fmt"

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
func (s *Store) GetBooksForUser(ctx context.Context, userID string) ([]*domain.Book, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get all books (use ListAllBooks for no pagination)
	allBooks, err := s.ListAllBooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all books: %w", err)
	}

	// Get collections user has access to
	userCollections, err := s.GetCollectionsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user collections: %w", err)
	}

	// Create a set of collection IDs the user has access to
	userCollectionIDs := make(map[string]bool)
	for _, coll := range userCollections {
		userCollectionIDs[coll.ID] = true
	}

	// Get all collections to determine which books are uncollected
	var allCollections []*domain.Collection
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
		allCollections = append(allCollections, collections...)
	}

	// Create a map of bookID -> list of collection IDs containing it
	bookToCollections := make(map[string][]string)
	for _, coll := range allCollections {
		for _, bookID := range coll.BookIDs {
			bookToCollections[bookID] = append(bookToCollections[bookID], coll.ID)
		}
	}

	// Filter books based on access rules
	var accessibleBooks []*domain.Book
	for _, book := range allBooks {
		collectionIDs, inAnyCollection := bookToCollections[book.ID]

		if !inAnyCollection {
			// Book is not in any collection -> public to all users
			accessibleBooks = append(accessibleBooks, book)
			continue
		}

		// Book is in at least one collection -> check if user has access to any of them
		hasAccess := false
		for _, collID := range collectionIDs {
			if userCollectionIDs[collID] {
				hasAccess = true
				break
			}
		}

		if hasAccess {
			accessibleBooks = append(accessibleBooks, book)
		}
	}

	return accessibleBooks, nil
}

// CanUserAccessBook checks if a user can see a specific book.
// Returns true if book is uncollected OR user has access to at least one collection containing it.
func (s *Store) CanUserAccessBook(ctx context.Context, userID, bookID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// Verify book exists (use internal method to bypass ACL)
	_, err := s.getBookInternal(ctx, bookID)
	if err != nil {
		return false, err
	}

	// Get collections containing this book
	collections, err := s.GetCollectionsContainingBook(ctx, bookID)
	if err != nil {
		return false, fmt.Errorf("get collections for book: %w", err)
	}

	// If book is in no collections, it's public
	if len(collections) == 0 {
		return true, nil
	}

	// Check if user has access to at least one collection
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
	for _, coll := range collections {
		if userCollectionIDs[coll.ID] {
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
