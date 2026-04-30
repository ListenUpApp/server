package service

import (
	"context"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/search/asyncindexer"
	"github.com/listenupapp/listenup-server/internal/store"
)

// seriesServiceStore is the narrow store interface SeriesService depends on.
type seriesServiceStore interface {
	store.SeriesStore
	// ACL helpers used by read paths in the handlers.
	GetAccessibleBookIDSet(ctx context.Context, userID string) (map[string]bool, error)
}

// SeriesService coordinates series CRUD with search indexing.
type SeriesService struct {
	store   seriesServiceStore
	indexer *asyncindexer.Indexer
	logger  *slog.Logger
}

// NewSeriesService creates a new SeriesService.
func NewSeriesService(s seriesServiceStore, indexer *asyncindexer.Indexer, logger *slog.Logger) *SeriesService {
	return &SeriesService{store: s, indexer: indexer, logger: logger}
}

// ListSeries returns a paginated list of series.
func (s *SeriesService) ListSeries(ctx context.Context, params store.PaginationParams) (*store.PaginatedResult[*domain.Series], error) {
	return s.store.ListSeries(ctx, params)
}

// GetSeries returns a series by ID.
func (s *SeriesService) GetSeries(ctx context.Context, id string) (*domain.Series, error) {
	return s.store.GetSeries(ctx, id)
}

// CreateSeries persists a new series and enqueues an index update.
func (s *SeriesService) CreateSeries(ctx context.Context, series *domain.Series) error {
	if err := s.store.CreateSeries(ctx, series); err != nil {
		return err
	}
	s.indexer.SubmitIndexSeries(series)
	return nil
}

// UpdateSeries persists changes to an existing series and enqueues an index update.
func (s *SeriesService) UpdateSeries(ctx context.Context, series *domain.Series) error {
	if err := s.store.UpdateSeries(ctx, series); err != nil {
		return err
	}
	s.indexer.SubmitIndexSeries(series)
	return nil
}

// DeleteSeries soft-deletes a series and enqueues an index removal.
func (s *SeriesService) DeleteSeries(ctx context.Context, id string) error {
	if err := s.store.DeleteSeries(ctx, id); err != nil {
		return err
	}
	s.indexer.SubmitDeleteSeries(id)
	return nil
}

// GetBooksBySeries returns all books that belong to a series.
func (s *SeriesService) GetBooksBySeries(ctx context.Context, seriesID string) ([]*domain.Book, error) {
	return s.store.GetBooksBySeries(ctx, seriesID)
}

// GetBookIDsBySeries returns the book IDs belonging to a series.
func (s *SeriesService) GetBookIDsBySeries(ctx context.Context, seriesID string) ([]string, error) {
	return s.store.GetBookIDsBySeries(ctx, seriesID)
}

// GetAccessibleBookIDSet returns the set of book IDs the given user can access.
func (s *SeriesService) GetAccessibleBookIDSet(ctx context.Context, userID string) (map[string]bool, error) {
	return s.store.GetAccessibleBookIDSet(ctx, userID)
}

// MergeSeries merges the source series into the target series.
// The source is deleted from the index; the merged target is re-indexed.
func (s *SeriesService) MergeSeries(ctx context.Context, sourceID, targetID string) (*domain.Series, error) {
	target, err := s.store.MergeSeries(ctx, sourceID, targetID)
	if err != nil {
		return nil, err
	}
	s.indexer.SubmitDeleteSeries(sourceID)
	s.indexer.SubmitIndexSeries(target)
	return target, nil
}
