package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/listenupapp/listenup-server/internal/validation"
)

// TagService orchestrates tag operations.
type TagService struct {
	store     *store.Store
	logger    *slog.Logger
	validator *validation.Validator
}

// NewTagService creates a new tag service.
func NewTagService(store *store.Store, logger *slog.Logger) *TagService {
	return &TagService{
		store:     store,
		logger:    logger,
		validator: validation.New(),
	}
}

// ListTags returns all tags for a user.
func (s *TagService) ListTags(ctx context.Context, userID string) ([]*domain.Tag, error) {
	return s.store.ListTagsForUser(ctx, userID)
}

// GetTag returns a single tag by ID.
func (s *TagService) GetTag(ctx context.Context, userID, tagID string) (*domain.Tag, error) {
	t, err := s.store.GetTag(ctx, tagID)
	if err != nil {
		return nil, err
	}

	// Verify ownership.
	if t.OwnerID != userID {
		return nil, store.ErrTagNotFound // Don't leak existence.
	}

	return t, nil
}

// CreateTagRequest contains fields for creating a tag.
type CreateTagRequest struct {
	Name  string `json:"name" validate:"required,min=1,max=50"`
	Color string `json:"color"`
}

// CreateTag creates a new tag.
func (s *TagService) CreateTag(ctx context.Context, userID string, req CreateTagRequest) (*domain.Tag, error) {
	if err := s.validator.Validate(req); err != nil {
		return nil, err
	}

	// Use GetOrCreate to handle duplicates gracefully.
	t, err := s.store.GetOrCreateTagByName(ctx, userID, req.Name)
	if err != nil {
		return nil, err
	}

	// Update color if provided and different.
	if req.Color != "" && t.Color != req.Color {
		t.Color = req.Color
		t.Touch()
		if err := s.store.UpdateTag(ctx, t); err != nil {
			return nil, err
		}
	}

	s.logger.Info("tag created", "id", t.ID, "name", req.Name, "user", userID)
	return t, nil
}

// UpdateTagRequest contains fields for updating a tag.
type UpdateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

// UpdateTag updates a tag.
func (s *TagService) UpdateTag(ctx context.Context, userID, tagID string, req UpdateTagRequest) (*domain.Tag, error) {
	t, err := s.store.GetTag(ctx, tagID)
	if err != nil {
		return nil, err
	}

	// Verify ownership.
	if t.OwnerID != userID {
		return nil, store.ErrTagNotFound
	}

	if req.Name != nil {
		t.Name = *req.Name
		// Optionally update slug too - for now keep it stable.
	}
	if req.Color != nil {
		t.Color = *req.Color
	}

	t.Touch()

	if err := s.store.UpdateTag(ctx, t); err != nil {
		return nil, err
	}

	return t, nil
}

// DeleteTag deletes a tag.
func (s *TagService) DeleteTag(ctx context.Context, userID, tagID string) error {
	t, err := s.store.GetTag(ctx, tagID)
	if err != nil {
		return err
	}

	// Verify ownership.
	if t.OwnerID != userID {
		return store.ErrTagNotFound // Don't leak existence.
	}

	return s.store.DeleteTag(ctx, tagID)
}

// AddTagToBook adds a tag to a book.
func (s *TagService) AddTagToBook(ctx context.Context, userID, bookID, tagID string) error {
	// Verify tag ownership.
	t, err := s.store.GetTag(ctx, tagID)
	if err != nil {
		return err
	}
	if t.OwnerID != userID {
		return store.ErrTagNotFound
	}

	bt := &domain.BookTag{
		BookID:    bookID,
		TagID:     tagID,
		UserID:    userID,
		CreatedAt: time.Now().UnixMilli(),
	}

	return s.store.AddBookTag(ctx, bt)
}

// RemoveTagFromBook removes a tag from a book.
func (s *TagService) RemoveTagFromBook(ctx context.Context, userID, bookID, tagID string) error {
	// Verify tag ownership.
	t, err := s.store.GetTag(ctx, tagID)
	if err != nil {
		return err
	}
	if t.OwnerID != userID {
		return store.ErrTagNotFound
	}

	return s.store.RemoveBookTag(ctx, bookID, userID, tagID)
}

// GetTagsForBook returns tags for a book (user-specific).
func (s *TagService) GetTagsForBook(ctx context.Context, userID, bookID string) ([]*domain.Tag, error) {
	tagIDs, err := s.store.GetTagIDsForBook(ctx, bookID, userID)
	if err != nil {
		return nil, err
	}

	tags := make([]*domain.Tag, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		t, err := s.store.GetTag(ctx, tagID)
		if err != nil {
			continue
		}
		tags = append(tags, t)
	}

	return tags, nil
}

// GetBooksForTag returns books with a specific tag.
func (s *TagService) GetBooksForTag(ctx context.Context, userID, tagID string) ([]string, error) {
	// Verify ownership.
	t, err := s.store.GetTag(ctx, tagID)
	if err != nil {
		return nil, err
	}
	if t.OwnerID != userID {
		return nil, fmt.Errorf("tag not found")
	}

	return s.store.GetBookIDsForTag(ctx, tagID)
}

// SetBookTags sets all tags for a book by a user.
func (s *TagService) SetBookTags(ctx context.Context, userID, bookID string, tagIDs []string) error {
	// Verify all tags belong to user.
	for _, tagID := range tagIDs {
		t, err := s.store.GetTag(ctx, tagID)
		if err != nil {
			return fmt.Errorf("tag %s not found: %w", tagID, err)
		}
		if t.OwnerID != userID {
			return fmt.Errorf("tag %s not found", tagID)
		}
	}

	return s.store.SetBookTags(ctx, bookID, userID, tagIDs)
}
