package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/genre"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/listenupapp/listenup-server/internal/validation"
)

// GenreService orchestrates genre operations.
type GenreService struct {
	store     *store.Store
	logger    *slog.Logger
	validator *validation.Validator
}

// NewGenreService creates a new genre service.
func NewGenreService(store *store.Store, logger *slog.Logger) *GenreService {
	return &GenreService{
		store:     store,
		logger:    logger,
		validator: validation.New(),
	}
}

// ListGenres returns the full genre tree.
func (s *GenreService) ListGenres(ctx context.Context) ([]*domain.Genre, error) {
	return s.store.ListGenres(ctx)
}

// GetGenre returns a single genre.
func (s *GenreService) GetGenre(ctx context.Context, id string) (*domain.Genre, error) {
	return s.store.GetGenre(ctx, id)
}

// GetGenreChildren returns direct children of a genre.
func (s *GenreService) GetGenreChildren(ctx context.Context, parentID string) ([]*domain.Genre, error) {
	return s.store.GetGenreChildren(ctx, parentID)
}

// CreateGenreRequest contains fields for creating a genre.
type CreateGenreRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100"`
	ParentID    string `json:"parent_id"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// CreateGenre creates a new genre.
func (s *GenreService) CreateGenre(ctx context.Context, req CreateGenreRequest) (*domain.Genre, error) {
	if err := s.validator.Validate(req); err != nil {
		return nil, err
	}

	slug := genre.Slugify(req.Name)

	// Check if slug already exists.
	existing, err := s.store.GetGenreBySlug(ctx, slug)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("genre with slug %q already exists", slug)
	}

	genreID, err := id.Generate("genre")
	if err != nil {
		return nil, err
	}

	// Calculate path and depth.
	var path string
	var depth int
	if req.ParentID != "" {
		parent, err := s.store.GetGenre(ctx, req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("parent genre not found: %w", err)
		}
		path = parent.Path + "/" + slug
		depth = parent.Depth + 1
	} else {
		path = "/" + slug
		depth = 0
	}

	g := &domain.Genre{
		Syncable:    domain.Syncable{ID: genreID},
		Name:        req.Name,
		Slug:        slug,
		Description: req.Description,
		ParentID:    req.ParentID,
		Path:        path,
		Depth:       depth,
		Color:       req.Color,
	}
	g.InitTimestamps()

	if err := s.store.CreateGenre(ctx, g); err != nil {
		return nil, err
	}

	s.logger.Info("genre created", "id", genreID, "name", req.Name, "parent", req.ParentID)
	return g, nil
}

// UpdateGenreRequest contains fields for updating a genre.
type UpdateGenreRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	SortOrder   *int    `json:"sort_order"`
}

// UpdateGenre updates a genre.
func (s *GenreService) UpdateGenre(ctx context.Context, genreID string, req UpdateGenreRequest) (*domain.Genre, error) {
	g, err := s.store.GetGenre(ctx, genreID)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		g.Name = *req.Name
		// Note: We don't change the slug on rename to preserve URLs.
	}
	if req.Description != nil {
		g.Description = *req.Description
	}
	if req.Color != nil {
		g.Color = *req.Color
	}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}

	g.Touch()

	if err := s.store.UpdateGenre(ctx, g); err != nil {
		return nil, err
	}

	return g, nil
}

// MoveGenre changes a genre's parent.
func (s *GenreService) MoveGenre(ctx context.Context, genreID, newParentID string) (*domain.Genre, error) {
	if err := s.store.MoveGenre(ctx, genreID, newParentID); err != nil {
		return nil, err
	}

	return s.store.GetGenre(ctx, genreID)
}

// MergeGenresRequest contains fields for merging genres.
type MergeGenresRequest struct {
	SourceID string `json:"source_id" validate:"required"`
	TargetID string `json:"target_id" validate:"required"`
}

// MergeGenres merges source into target.
func (s *GenreService) MergeGenres(ctx context.Context, req MergeGenresRequest) error {
	if err := s.validator.Validate(req); err != nil {
		return err
	}

	if req.SourceID == req.TargetID {
		return fmt.Errorf("cannot merge genre into itself")
	}

	return s.store.MergeGenres(ctx, req.SourceID, req.TargetID)
}

// DeleteGenre deletes a genre.
func (s *GenreService) DeleteGenre(ctx context.Context, genreID string) error {
	return s.store.DeleteGenre(ctx, genreID)
}

// GetBooksForGenre returns book IDs in a genre (optionally including descendants).
func (s *GenreService) GetBooksForGenre(ctx context.Context, genreID string, includeDescendants bool) ([]string, error) {
	if includeDescendants {
		return s.store.GetBookIDsForGenreTree(ctx, genreID)
	}
	return s.store.GetBookIDsForGenre(ctx, genreID)
}

// ListUnmappedGenres returns raw genre strings that need mapping.
func (s *GenreService) ListUnmappedGenres(ctx context.Context) ([]*domain.UnmappedGenre, error) {
	return s.store.ListUnmappedGenres(ctx)
}

// MapUnmappedGenreRequest contains fields for mapping an unmapped genre.
type MapUnmappedGenreRequest struct {
	RawValue string   `json:"raw_value" validate:"required"`
	GenreIDs []string `json:"genre_ids" validate:"required,min=1"`
}

// MapUnmappedGenre creates an alias for an unmapped genre.
func (s *GenreService) MapUnmappedGenre(ctx context.Context, userID string, req MapUnmappedGenreRequest) error {
	if err := s.validator.Validate(req); err != nil {
		return err
	}

	// Verify all genre IDs exist.
	for _, gid := range req.GenreIDs {
		if _, err := s.store.GetGenre(ctx, gid); err != nil {
			return fmt.Errorf("genre %s not found: %w", gid, err)
		}
	}

	return s.store.ResolveUnmappedGenre(ctx, req.RawValue, req.GenreIDs, userID)
}

// SetBookGenres sets all genres for a book.
func (s *GenreService) SetBookGenres(ctx context.Context, bookID string, genreIDs []string) error {
	return s.store.SetBookGenres(ctx, bookID, genreIDs)
}

// SeedDefaultGenres creates the default genre hierarchy if not already seeded.
func (s *GenreService) SeedDefaultGenres(ctx context.Context) error {
	return s.store.SeedDefaultGenres(ctx)
}
