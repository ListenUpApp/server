package service

import (
	"context"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/metadata/audible"
	"github.com/listenupapp/listenup-server/internal/store"
)

// MetadataService orchestrates metadata fetching with caching.
type MetadataService struct {
	client        *audible.Client
	store         *store.Store
	defaultRegion audible.Region
	logger        *slog.Logger
}

// NewMetadataService creates a new metadata service.
func NewMetadataService(
	client *audible.Client,
	store *store.Store,
	defaultRegion audible.Region,
	logger *slog.Logger,
) *MetadataService {
	return &MetadataService{
		client:        client,
		store:         store,
		defaultRegion: defaultRegion,
		logger:        logger,
	}
}

// Search searches the Audible catalog.
// Results are not cached as they are transient.
func (s *MetadataService) Search(ctx context.Context, region *audible.Region, params audible.SearchParams) ([]audible.SearchResult, error) {
	r := s.resolveRegion(region)

	s.logger.Debug("searching Audible",
		"region", r,
		"keywords", params.Keywords,
	)

	return s.client.Search(ctx, r, params)
}

// GetBook fetches book metadata, using cache if fresh.
func (s *MetadataService) GetBook(ctx context.Context, region *audible.Region, asin string) (*audible.Book, error) {
	r := s.resolveRegion(region)

	// Check cache first
	cached, err := s.store.GetCachedBook(ctx, r, asin)
	if err != nil {
		s.logger.Warn("cache lookup failed",
			"error", err,
			"asin", asin,
		)
		// Continue to fetch fresh
	}

	if cached != nil {
		s.logger.Debug("cache hit for book",
			"asin", asin,
			"region", r,
			"age", cached.FetchedAt,
		)
		return cached.Book, nil
	}

	// Fetch from Audible
	s.logger.Debug("fetching book from Audible",
		"asin", asin,
		"region", r,
	)

	book, err := s.client.GetBook(ctx, r, asin)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if err := s.store.SetCachedBook(ctx, r, asin, book); err != nil {
		s.logger.Warn("failed to cache book",
			"error", err,
			"asin", asin,
		)
		// Don't fail the request
	}

	return book, nil
}

// GetChapters fetches chapter information, using cache if fresh.
func (s *MetadataService) GetChapters(ctx context.Context, region *audible.Region, asin string) ([]audible.Chapter, error) {
	r := s.resolveRegion(region)

	// Check cache first
	cached, err := s.store.GetCachedChapters(ctx, r, asin)
	if err != nil {
		s.logger.Warn("cache lookup failed",
			"error", err,
			"asin", asin,
		)
	}

	if cached != nil {
		s.logger.Debug("cache hit for chapters",
			"asin", asin,
			"region", r,
		)
		return cached.Chapters, nil
	}

	// Fetch from Audible
	s.logger.Debug("fetching chapters from Audible",
		"asin", asin,
		"region", r,
	)

	chapters, err := s.client.GetChapters(ctx, r, asin)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if err := s.store.SetCachedChapters(ctx, r, asin, chapters); err != nil {
		s.logger.Warn("failed to cache chapters",
			"error", err,
			"asin", asin,
		)
	}

	return chapters, nil
}

// RefreshBook forces a fresh fetch, bypassing and updating cache.
func (s *MetadataService) RefreshBook(ctx context.Context, region audible.Region, asin string) (*audible.Book, error) {
	s.logger.Info("refreshing book metadata",
		"asin", asin,
		"region", region,
	)

	// Delete existing cache
	if err := s.store.DeleteCachedBook(ctx, region, asin); err != nil {
		s.logger.Warn("failed to delete cached book",
			"error", err,
			"asin", asin,
		)
	}

	// Fetch fresh
	book, err := s.client.GetBook(ctx, region, asin)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if err := s.store.SetCachedBook(ctx, region, asin, book); err != nil {
		s.logger.Warn("failed to cache book",
			"error", err,
			"asin", asin,
		)
	}

	return book, nil
}

// resolveRegion returns the provided region or falls back to default.
func (s *MetadataService) resolveRegion(region *audible.Region) audible.Region {
	if region != nil && region.Valid() {
		return *region
	}
	return s.defaultRegion
}

// Close releases resources.
func (s *MetadataService) Close() {
	s.client.Close()
}
