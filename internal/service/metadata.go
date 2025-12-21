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

// Search searches the Audible catalog with caching.
func (s *MetadataService) Search(ctx context.Context, region *audible.Region, params audible.SearchParams) ([]audible.SearchResult, error) {
	r := s.resolveRegion(region)

	// Only cache keyword searches (most common case)
	cacheKey := params.Keywords
	if cacheKey != "" {
		cached, err := s.store.GetCachedSearch(ctx, r, cacheKey)
		if err != nil {
			s.logger.Warn("search cache lookup failed",
				"error", err,
				"query", cacheKey,
			)
		}

		if cached != nil {
			s.logger.Debug("cache hit for search",
				"query", cacheKey,
				"region", r,
				"results", len(cached.Results),
			)
			return cached.Results, nil
		}
	}

	s.logger.Debug("searching Audible",
		"region", r,
		"keywords", params.Keywords,
	)

	results, err := s.client.Search(ctx, r, params)
	if err != nil {
		return nil, err
	}

	// Cache the results
	if cacheKey != "" {
		if err := s.store.SetCachedSearch(ctx, r, cacheKey, results); err != nil {
			s.logger.Warn("failed to cache search results",
				"error", err,
				"query", cacheKey,
			)
		}
	}

	return results, nil
}

// SearchWithFallback searches with fallback to US region.
// Returns results, the region that succeeded, and any error.
func (s *MetadataService) SearchWithFallback(ctx context.Context, params audible.SearchParams) ([]audible.SearchResult, audible.Region, error) {
	// Try default region first
	results, err := s.Search(ctx, &s.defaultRegion, params)
	if err == nil && len(results) > 0 {
		return results, s.defaultRegion, nil
	}

	// Log if we got an error (but continue to fallback)
	if err != nil {
		s.logger.Warn("default region search failed, trying fallback",
			"error", err,
			"defaultRegion", s.defaultRegion,
		)
	}

	// Fall back to US if different from default
	if s.defaultRegion != audible.RegionUS {
		s.logger.Debug("falling back to US region",
			"defaultRegion", s.defaultRegion,
		)

		usRegion := audible.RegionUS
		results, err = s.Search(ctx, &usRegion, params)
		if err == nil && len(results) > 0 {
			return results, audible.RegionUS, nil
		}
	}

	// Return empty with the original error (if any)
	return nil, s.defaultRegion, err
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

// RefreshChapters forces a fresh fetch, bypassing and updating cache.
func (s *MetadataService) RefreshChapters(ctx context.Context, region audible.Region, asin string) ([]audible.Chapter, error) {
	s.logger.Info("refreshing chapters",
		"asin", asin,
		"region", region,
	)

	// Delete existing cache
	if err := s.store.DeleteCachedChapters(ctx, region, asin); err != nil {
		s.logger.Warn("failed to delete cached chapters",
			"error", err,
			"asin", asin,
		)
	}

	// Fetch fresh
	chapters, err := s.client.GetChapters(ctx, region, asin)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if err := s.store.SetCachedChapters(ctx, region, asin, chapters); err != nil {
		s.logger.Warn("failed to cache chapters",
			"error", err,
			"asin", asin,
		)
	}

	return chapters, nil
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
