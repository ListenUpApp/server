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

// LensActivityRecorder records lens-related activities.
// This avoids a circular dependency between LensService and ActivityService.
type LensActivityRecorder interface {
	RecordLensCreated(ctx context.Context, userID string, lens *domain.Lens) error
}

// LensService orchestrates lens operations with ownership enforcement and SSE events.
type LensService struct {
	store            *store.Store
	sseManager       *sse.Manager
	logger           *slog.Logger
	activityRecorder LensActivityRecorder
}

// NewLensService creates a new lens service.
func NewLensService(store *store.Store, sseManager *sse.Manager, logger *slog.Logger) *LensService {
	return &LensService{
		store:      store,
		sseManager: sseManager,
		logger:     logger,
	}
}

// SetActivityRecorder sets the activity recorder for recording social activities.
// This is set after construction to avoid circular dependencies.
func (s *LensService) SetActivityRecorder(recorder LensActivityRecorder) {
	s.activityRecorder = recorder
}

// CreateLens creates a new lens for the user.
func (s *LensService) CreateLens(ctx context.Context, ownerID, name, description string) (*domain.Lens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate name is not empty
	if name == "" {
		return nil, domainerrors.Validation("lens name cannot be empty")
	}

	// Generate lens ID
	lensID, err := id.Generate("lens")
	if err != nil {
		return nil, fmt.Errorf("generate lens ID: %w", err)
	}

	now := time.Now()
	lens := &domain.Lens{
		ID:          lensID,
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		BookIDs:     []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateLens(ctx, lens); err != nil {
		return nil, fmt.Errorf("create lens: %w", err)
	}

	s.logger.Info("lens created",
		"lens_id", lensID,
		"owner_id", ownerID,
		"name", name,
	)

	// Emit SSE event
	owner, _ := s.store.GetUser(ctx, ownerID)
	displayName, avatarColor := s.getOwnerInfo(owner)
	s.sseManager.Emit(sse.NewLensCreatedEvent(lens, displayName, avatarColor))

	// Record activity for social feed
	if s.activityRecorder != nil {
		if err := s.activityRecorder.RecordLensCreated(ctx, ownerID, lens); err != nil {
			s.logger.Warn("failed to record lens created activity",
				"lens_id", lensID,
				"owner_id", ownerID,
				"error", err,
			)
		}
	}

	return lens, nil
}

// GetLens retrieves a lens by ID.
func (s *LensService) GetLens(ctx context.Context, id string) (*domain.Lens, error) {
	return s.store.GetLens(ctx, id)
}

// UpdateLens updates lens metadata.
// Requires ownership.
func (s *LensService) UpdateLens(ctx context.Context, userID, lensID, name, description string) (*domain.Lens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get lens and verify ownership
	lens, err := s.store.GetLens(ctx, lensID)
	if err != nil {
		return nil, err
	}

	if lens.OwnerID != userID {
		return nil, domainerrors.Forbidden("you do not own this lens")
	}

	// Validate name is not empty
	if name == "" {
		return nil, domainerrors.Validation("lens name cannot be empty")
	}

	// Update fields
	lens.Name = name
	lens.Description = description
	lens.UpdatedAt = time.Now()

	if err := s.store.UpdateLens(ctx, lens); err != nil {
		return nil, fmt.Errorf("update lens: %w", err)
	}

	s.logger.Info("lens updated",
		"lens_id", lensID,
		"user_id", userID,
		"name", name,
	)

	// Emit SSE event
	owner, _ := s.store.GetUser(ctx, lens.OwnerID)
	displayName, avatarColor := s.getOwnerInfo(owner)
	s.sseManager.Emit(sse.NewLensUpdatedEvent(lens, displayName, avatarColor))

	return lens, nil
}

// DeleteLens deletes a lens.
// Requires ownership.
func (s *LensService) DeleteLens(ctx context.Context, userID, lensID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get lens and verify ownership
	lens, err := s.store.GetLens(ctx, lensID)
	if err != nil {
		return err
	}

	if lens.OwnerID != userID {
		return domainerrors.Forbidden("you do not own this lens")
	}

	ownerID := lens.OwnerID

	if err := s.store.DeleteLens(ctx, lensID); err != nil {
		return fmt.Errorf("delete lens: %w", err)
	}

	s.logger.Info("lens deleted",
		"lens_id", lensID,
		"user_id", userID,
	)

	// Emit SSE event
	s.sseManager.Emit(sse.NewLensDeletedEvent(lensID, ownerID))

	return nil
}

// ListMyLenses returns all lenses owned by the user.
func (s *LensService) ListMyLenses(ctx context.Context, ownerID string) ([]*domain.Lens, error) {
	return s.store.ListLensesByOwner(ctx, ownerID)
}

// ListDiscoverLenses returns lenses from other users that contain books the requesting user can access.
// Returns a map of owner ID to lenses.
func (s *LensService) ListDiscoverLenses(ctx context.Context, userID string) (map[string][]*domain.Lens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get all lenses
	allLenses, err := s.store.ListAllLenses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all lenses: %w", err)
	}

	result := make(map[string][]*domain.Lens)

	for _, lens := range allLenses {
		// Skip user's own lenses
		if lens.OwnerID == userID {
			continue
		}

		// Check if user can access at least one book in the lens
		canSeeAnyBook := false
		for _, bookID := range lens.BookIDs {
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
			result[lens.OwnerID] = append(result[lens.OwnerID], lens)
		}
	}

	return result, nil
}

// AddBookToLens adds a book to a lens.
// Requires ownership of the lens.
func (s *LensService) AddBookToLens(ctx context.Context, userID, lensID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get lens and verify ownership
	lens, err := s.store.GetLens(ctx, lensID)
	if err != nil {
		return err
	}

	if lens.OwnerID != userID {
		return domainerrors.Forbidden("you do not own this lens")
	}

	// Check if book already in lens (no-op if so)
	if lens.ContainsBook(bookID) {
		return nil
	}

	// Add book via store
	if err := s.store.AddBookToLens(ctx, lensID, bookID); err != nil {
		return fmt.Errorf("add book to lens: %w", err)
	}

	// Re-fetch lens for updated state
	lens, err = s.store.GetLens(ctx, lensID)
	if err != nil {
		return fmt.Errorf("get updated lens: %w", err)
	}

	s.logger.Info("book added to lens",
		"lens_id", lensID,
		"book_id", bookID,
		"user_id", userID,
	)

	// Emit SSE event
	s.sseManager.Emit(sse.NewLensBookAddedEvent(lens, bookID))

	return nil
}

// RemoveBookFromLens removes a book from a lens.
// Requires ownership of the lens.
func (s *LensService) RemoveBookFromLens(ctx context.Context, userID, lensID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get lens and verify ownership
	lens, err := s.store.GetLens(ctx, lensID)
	if err != nil {
		return err
	}

	if lens.OwnerID != userID {
		return domainerrors.Forbidden("you do not own this lens")
	}

	// Check if book in lens (no-op if not)
	if !lens.ContainsBook(bookID) {
		return nil
	}

	// Remove book via store
	if err := s.store.RemoveBookFromLens(ctx, lensID, bookID); err != nil {
		return fmt.Errorf("remove book from lens: %w", err)
	}

	// Re-fetch lens for updated state
	lens, err = s.store.GetLens(ctx, lensID)
	if err != nil {
		return fmt.Errorf("get updated lens: %w", err)
	}

	s.logger.Info("book removed from lens",
		"lens_id", lensID,
		"book_id", bookID,
		"user_id", userID,
	)

	// Emit SSE event
	s.sseManager.Emit(sse.NewLensBookRemovedEvent(lens, bookID))

	return nil
}

// GetLensesForBook returns lenses containing the specified book that are owned by the user.
func (s *LensService) GetLensesForBook(ctx context.Context, userID, bookID string) ([]*domain.Lens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get all lenses containing the book
	lenses, err := s.store.GetLensesContainingBook(ctx, bookID)
	if err != nil {
		return nil, fmt.Errorf("get lenses containing book: %w", err)
	}

	// Filter to user's own lenses only
	var userLenses []*domain.Lens
	for _, lens := range lenses {
		if lens.OwnerID == userID {
			userLenses = append(userLenses, lens)
		}
	}

	return userLenses, nil
}

// CreateDefaultLens creates a default "To Read" lens for a new user.
// This is a best-effort operation that logs but doesn't fail registration.
func (s *LensService) CreateDefaultLens(ctx context.Context, userID string) error {
	_, err := s.CreateLens(ctx, userID, "To Read", "")
	if err != nil {
		s.logger.Warn("failed to create default lens for user",
			"user_id", userID,
			"error", err,
		)
		return err
	}

	s.logger.Info("created default lens for user",
		"user_id", userID,
	)

	return nil
}

// getOwnerInfo extracts display name and avatar color from a user.
func (s *LensService) getOwnerInfo(user *domain.User) (displayName, avatarColor string) {
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
