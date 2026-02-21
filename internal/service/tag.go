package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/listenupapp/listenup-server/internal/util"
)

// TagService orchestrates global tag operations.
// Tags are community-wide — no user ownership, but requires book access to tag.
type TagService struct {
	store      store.Store
	sseManager *sse.Manager
	search     *SearchService
	logger     *slog.Logger
}

// NewTagService creates a new tag service.
func NewTagService(store store.Store, sseManager *sse.Manager, search *SearchService, logger *slog.Logger) *TagService {
	return &TagService{
		store:      store,
		sseManager: sseManager,
		search:     search,
		logger:     logger,
	}
}

// Tag service errors.
var (
	ErrInvalidTagSlug = errors.New("tag slug is empty after normalization")
	ErrTagNotFound    = errors.New("tag not found")
	ErrForbidden      = errors.New("access denied: cannot access this book")
)

// ListTags returns all global tags ordered by popularity.
func (s *TagService) ListTags(ctx context.Context) ([]*domain.Tag, error) {
	return s.store.ListTags(ctx)
}

// GetTagBySlug returns a tag by its slug.
func (s *TagService) GetTagBySlug(ctx context.Context, slug string) (*domain.Tag, error) {
	t, err := s.store.GetTagBySlug(ctx, slug)
	if errors.Is(err, store.ErrTagNotFound) {
		return nil, ErrTagNotFound
	}
	return t, err
}

// GetTagsForBook returns all tags on a book.
func (s *TagService) GetTagsForBook(ctx context.Context, bookID string) ([]*domain.Tag, error) {
	return s.store.GetTagsForBook(ctx, bookID)
}

// AddTagToBook adds a tag to a book.
// Creates the tag if it doesn't exist.
// Requires user to have read access to the book.
func (s *TagService) AddTagToBook(ctx context.Context, userID, bookID, rawInput string) (*domain.Tag, bool, error) {
	// 1. Verify user can access this book.
	canAccess, err := s.store.CanUserAccessBook(ctx, userID, bookID)
	if err != nil {
		return nil, false, err
	}
	if !canAccess {
		return nil, false, ErrForbidden
	}

	// 2. Normalize input to slug.
	slug := util.NormalizeTagSlug(rawInput)
	if slug == "" {
		return nil, false, ErrInvalidTagSlug
	}

	// 3. Find or create tag.
	tag, created, err := s.store.FindOrCreateTagBySlug(ctx, slug)
	if err != nil {
		return nil, false, err
	}

	// 4. Add relationship (idempotent).
	if err := s.store.AddTagToBook(ctx, bookID, tag.ID); err != nil {
		return nil, false, err
	}

	// 5. Re-fetch tag to get updated book count.
	tag, err = s.store.GetTagByID(ctx, tag.ID)
	if err != nil {
		return nil, false, err
	}

	// 6. Trigger search re-index (async, best effort).
	s.reindexBookTags(ctx, bookID)

	// 7. Emit SSE events.
	if created {
		s.sseManager.Emit(sse.NewTagCreatedEvent(tag))
	}
	s.sseManager.Emit(sse.NewBookTagAddedEvent(bookID, tag))

	s.logger.Info("tag added to book",
		"tag_slug", tag.Slug,
		"book_id", bookID,
		"user_id", userID,
		"created", created,
	)

	return tag, created, nil
}

// RemoveTagFromBook removes a tag from a book.
// Requires user to have read access to the book.
func (s *TagService) RemoveTagFromBook(ctx context.Context, userID, bookID, slug string) error {
	// 1. Verify user can access this book.
	canAccess, err := s.store.CanUserAccessBook(ctx, userID, bookID)
	if err != nil {
		return err
	}
	if !canAccess {
		return ErrForbidden
	}

	// 2. Find tag by slug (must exist).
	tag, err := s.store.GetTagBySlug(ctx, slug)
	if errors.Is(err, store.ErrTagNotFound) {
		return ErrTagNotFound
	}
	if err != nil {
		return err
	}

	// 3. Remove relationship (idempotent).
	if err := s.store.RemoveTagFromBook(ctx, bookID, tag.ID); err != nil {
		return err
	}

	// 4. Re-fetch tag to get updated book count.
	tag, err = s.store.GetTagByID(ctx, tag.ID)
	if err != nil {
		// Tag might have been deleted in rare race, but we already removed the relationship.
		s.logger.Warn("could not fetch tag after removal", "tag_id", tag.ID, "error", err)
	}

	// 5. Trigger search re-index.
	s.reindexBookTags(ctx, bookID)

	// 6. Emit SSE event (no EventTagDeleted — orphans persist).
	if tag != nil {
		s.sseManager.Emit(sse.NewBookTagRemovedEvent(bookID, tag))
	}

	s.logger.Info("tag removed from book",
		"tag_slug", slug,
		"book_id", bookID,
		"user_id", userID,
	)

	return nil
}

// GetBooksForTag returns books with a specific tag, filtered by user's access.
func (s *TagService) GetBooksForTag(ctx context.Context, userID, slug string) ([]*domain.Book, error) {
	// 1. Find tag by slug.
	tag, err := s.store.GetTagBySlug(ctx, slug)
	if errors.Is(err, store.ErrTagNotFound) {
		return nil, ErrTagNotFound
	}
	if err != nil {
		return nil, err
	}

	// 2. Get all book IDs with this tag.
	bookIDs, err := s.store.GetBookIDsForTag(ctx, tag.ID)
	if err != nil {
		return nil, err
	}

	// 3. Filter by user access and fetch books.
	var books []*domain.Book
	for _, bookID := range bookIDs {
		canAccess, err := s.store.CanUserAccessBook(ctx, userID, bookID)
		if err != nil {
			continue
		}
		if !canAccess {
			continue
		}

		book, err := s.store.GetBook(ctx, bookID, userID)
		if err != nil {
			continue
		}
		books = append(books, book)
	}

	return books, nil
}

// reindexBookTags triggers search re-indexing for a book's tags.
func (s *TagService) reindexBookTags(ctx context.Context, bookID string) {
	if s.search == nil {
		return
	}

	// Get tag slugs for the book.
	slugs, err := s.store.GetTagSlugsForBook(ctx, bookID)
	if err != nil {
		s.logger.Warn("failed to get tag slugs for reindex", "book_id", bookID, "error", err)
		return
	}

	// Reindex the book with updated tags.
	if err := s.search.UpdateBookTags(ctx, bookID, slugs); err != nil {
		s.logger.Warn("failed to reindex book tags", "book_id", bookID, "error", err)
	}
}

// CleanupTagsForDeletedBook removes tag associations for a deleted book.
// Called from book deletion flow.
func (s *TagService) CleanupTagsForDeletedBook(ctx context.Context, bookID string) error {
	return s.store.CleanupTagsForDeletedBook(ctx, bookID)
}
