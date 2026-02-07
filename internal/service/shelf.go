package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ShelfActivityRecorder records shelf-related activities.
// This avoids a circular dependency between ShelfService and ActivityService.
type ShelfActivityRecorder interface {
	RecordShelfCreated(ctx context.Context, userID string, shelf *domain.Shelf) error
}

// ShelfService orchestrates shelf operations with ownership enforcement and SSE events.
type ShelfService struct {
	store            *store.Store
	sseManager       *sse.Manager
	logger           *slog.Logger
	activityRecorder ShelfActivityRecorder
}

// NewShelfService creates a new shelf service.
func NewShelfService(store *store.Store, sseManager *sse.Manager, logger *slog.Logger) *ShelfService {
	return &ShelfService{
		store:      store,
		sseManager: sseManager,
		logger:     logger,
	}
}

// SetActivityRecorder sets the activity recorder for recording social activities.
// This is set after construction to avoid circular dependencies.
func (s *ShelfService) SetActivityRecorder(recorder ShelfActivityRecorder) {
	s.activityRecorder = recorder
}

// CreateShelf creates a new shelf for the user.
func (s *ShelfService) CreateShelf(ctx context.Context, ownerID, name, description string) (*domain.Shelf, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate name is not empty
	if name == "" {
		return nil, domainerrors.Validation("shelf name cannot be empty")
	}

	// Generate shelf ID
	shelfID, err := id.Generate("shelf")
	if err != nil {
		return nil, fmt.Errorf("generate shelf ID: %w", err)
	}

	now := time.Now()
	shelf := &domain.Shelf{
		ID:          shelfID,
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		BookIDs:     []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateShelf(ctx, shelf); err != nil {
		return nil, fmt.Errorf("create shelf: %w", err)
	}

	s.logger.Info("shelf created",
		"shelf_id", shelfID,
		"owner_id", ownerID,
		"name", name,
	)

	// Emit SSE event
	owner, _ := s.store.GetUser(ctx, ownerID)
	displayName, avatarColor := s.getOwnerInfo(owner)
	s.sseManager.Emit(sse.NewShelfCreatedEvent(shelf, displayName, avatarColor))

	// Record activity for social feed
	if s.activityRecorder != nil {
		if err := s.activityRecorder.RecordShelfCreated(ctx, ownerID, shelf); err != nil {
			s.logger.Warn("failed to record shelf created activity",
				"shelf_id", shelfID,
				"owner_id", ownerID,
				"error", err,
			)
		}
	}

	return shelf, nil
}

// GetShelf retrieves a shelf by ID.
func (s *ShelfService) GetShelf(ctx context.Context, id string) (*domain.Shelf, error) {
	return s.store.GetShelf(ctx, id)
}

// UpdateShelf updates shelf metadata.
// Requires ownership.
func (s *ShelfService) UpdateShelf(ctx context.Context, userID, shelfID, name, description string) (*domain.Shelf, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get shelf and verify ownership
	shelf, err := s.store.GetShelf(ctx, shelfID)
	if err != nil {
		return nil, err
	}

	if shelf.OwnerID != userID {
		return nil, domainerrors.Forbidden("you do not own this shelf")
	}

	// Validate name is not empty
	if name == "" {
		return nil, domainerrors.Validation("shelf name cannot be empty")
	}

	// Update fields
	shelf.Name = name
	shelf.Description = description
	shelf.UpdatedAt = time.Now()

	if err := s.store.UpdateShelf(ctx, shelf); err != nil {
		return nil, fmt.Errorf("update shelf: %w", err)
	}

	s.logger.Info("shelf updated",
		"shelf_id", shelfID,
		"user_id", userID,
		"name", name,
	)

	// Emit SSE event
	owner, _ := s.store.GetUser(ctx, shelf.OwnerID)
	displayName, avatarColor := s.getOwnerInfo(owner)
	s.sseManager.Emit(sse.NewShelfUpdatedEvent(shelf, displayName, avatarColor))

	return shelf, nil
}

// DeleteShelf deletes a shelf.
// Requires ownership.
func (s *ShelfService) DeleteShelf(ctx context.Context, userID, shelfID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get shelf and verify ownership
	shelf, err := s.store.GetShelf(ctx, shelfID)
	if err != nil {
		return err
	}

	if shelf.OwnerID != userID {
		return domainerrors.Forbidden("you do not own this shelf")
	}

	ownerID := shelf.OwnerID

	if err := s.store.DeleteShelf(ctx, shelfID); err != nil {
		return fmt.Errorf("delete shelf: %w", err)
	}

	s.logger.Info("shelf deleted",
		"shelf_id", shelfID,
		"user_id", userID,
	)

	// Emit SSE event
	s.sseManager.Emit(sse.NewShelfDeletedEvent(shelfID, ownerID))

	return nil
}

// ListMyShelves returns all shelves owned by the user.
func (s *ShelfService) ListMyShelves(ctx context.Context, ownerID string) ([]*domain.Shelf, error) {
	return s.store.ListShelvesByOwner(ctx, ownerID)
}

// ListDiscoverShelves returns shelves from other users that contain books the requesting user can access.
// Returns a map of owner ID to shelves.
func (s *ShelfService) ListDiscoverShelves(ctx context.Context, userID string) (map[string][]*domain.Shelf, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get all shelves
	allShelves, err := s.store.ListAllShelves(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all shelves: %w", err)
	}

	result := make(map[string][]*domain.Shelf)

	for _, shelf := range allShelves {
		// Skip user's own shelves
		if shelf.OwnerID == userID {
			continue
		}

		// Check if user can access at least one book in the shelf
		canSeeAnyBook := false
		for _, bookID := range shelf.BookIDs {
			canAccess, err := s.store.CanUserAccessBook(ctx, userID, bookID)
			if err != nil {
				s.logger.Warn("failed to check book access",
					"user_id", userID,
					"book_id", bookID,
					"error", err,
				)
				continue
			}
			if canAccess {
				canSeeAnyBook = true
				break
			}
		}

		if canSeeAnyBook {
			result[shelf.OwnerID] = append(result[shelf.OwnerID], shelf)
		}
	}

	return result, nil
}

// AddBookToShelf adds a book to a shelf.
// Requires ownership of the shelf.
func (s *ShelfService) AddBookToShelf(ctx context.Context, userID, shelfID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get shelf and verify ownership
	shelf, err := s.store.GetShelf(ctx, shelfID)
	if err != nil {
		return err
	}

	if shelf.OwnerID != userID {
		return domainerrors.Forbidden("you do not own this shelf")
	}

	// Check if book already in shelf (no-op if so)
	if shelf.ContainsBook(bookID) {
		return nil
	}

	// Add book via store
	if err := s.store.AddBookToShelf(ctx, shelfID, bookID); err != nil {
		return fmt.Errorf("add book to shelf: %w", err)
	}

	// Re-fetch shelf for updated state
	shelf, err = s.store.GetShelf(ctx, shelfID)
	if err != nil {
		return fmt.Errorf("get updated shelf: %w", err)
	}

	s.logger.Info("book added to shelf",
		"shelf_id", shelfID,
		"book_id", bookID,
		"user_id", userID,
	)

	// Emit SSE event
	s.sseManager.Emit(sse.NewShelfBookAddedEvent(shelf, bookID))

	return nil
}

// RemoveBookFromShelf removes a book from a shelf.
// Requires ownership of the shelf.
func (s *ShelfService) RemoveBookFromShelf(ctx context.Context, userID, shelfID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get shelf and verify ownership
	shelf, err := s.store.GetShelf(ctx, shelfID)
	if err != nil {
		return err
	}

	if shelf.OwnerID != userID {
		return domainerrors.Forbidden("you do not own this shelf")
	}

	// Check if book in shelf (no-op if not)
	if !shelf.ContainsBook(bookID) {
		return nil
	}

	// Remove book via store
	if err := s.store.RemoveBookFromShelf(ctx, shelfID, bookID); err != nil {
		return fmt.Errorf("remove book from shelf: %w", err)
	}

	// Re-fetch shelf for updated state
	shelf, err = s.store.GetShelf(ctx, shelfID)
	if err != nil {
		return fmt.Errorf("get updated shelf: %w", err)
	}

	s.logger.Info("book removed from shelf",
		"shelf_id", shelfID,
		"book_id", bookID,
		"user_id", userID,
	)

	// Emit SSE event
	s.sseManager.Emit(sse.NewShelfBookRemovedEvent(shelf, bookID))

	return nil
}

// GetShelvesForBook returns shelves containing the specified book that are owned by the user.
func (s *ShelfService) GetShelvesForBook(ctx context.Context, userID, bookID string) ([]*domain.Shelf, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get all shelves containing the book
	shelves, err := s.store.GetShelvesContainingBook(ctx, bookID)
	if err != nil {
		return nil, fmt.Errorf("get shelves containing book: %w", err)
	}

	// Filter to user's own shelves only
	var userShelves []*domain.Shelf
	for _, shelf := range shelves {
		if shelf.OwnerID == userID {
			userShelves = append(userShelves, shelf)
		}
	}

	return userShelves, nil
}

// CreateDefaultShelf creates a default "To Read" shelf for a new user.
// This is a best-effort operation that logs but doesn't fail registration.
func (s *ShelfService) CreateDefaultShelf(ctx context.Context, userID string) error {
	_, err := s.CreateShelf(ctx, userID, "To Read", "")
	if err != nil {
		s.logger.Warn("failed to create default shelf for user",
			"user_id", userID,
			"error", err,
		)
		return err
	}

	s.logger.Info("created default shelf for user",
		"user_id", userID,
	)

	return nil
}

// getOwnerInfo extracts display name and avatar color from a user.
func (s *ShelfService) getOwnerInfo(user *domain.User) (displayName, avatarColor string) {
	if user == nil {
		return "Unknown", "#6B7280"
	}
	displayName = user.DisplayName
	if displayName == "" {
		displayName = user.FirstName + " " + user.LastName
	}
	avatarColor = "#6B7280" // Default color - AvatarColor field may be added to User later
	return
}
