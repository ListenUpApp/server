package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ManifestResponse is the response for GET /api/v1/sync/manifest.
// This is the first phase of initial sync, the idea is to give the client.
// minimal info to display immediately to the user while we fetch the real.
// stuff in the background.
type ManifestResponse struct {
	LibraryVersion string   `json:"library_version"`
	Checkpoint     string   `json:"checkpoint"`
	BookIDs        []string `json:"book_ids"`
	Counts         struct {
		Books        int `json:"books"`
		Contributors int `json:"contributors"`
		Series       int `json:"series"`
	} `json:"counts"`
}

// SyncService orchestrates sync operations between server and clients.
type SyncService struct {
	store    *store.Store
	enricher *dto.Enricher
	logger   *slog.Logger
}

// NewSyncService creates a new sync service.
func NewSyncService(store *store.Store, logger *slog.Logger) *SyncService {
	return &SyncService{
		store:    store,
		enricher: dto.NewEnricher(store),
		logger:   logger,
	}
}

// GetManifest returns the library manifest for sync.
// This provides a high-level overview of the library state including.
// the current checkpoint and counts of various entities.
func (s *SyncService) GetManifest(ctx context.Context) (*ManifestResponse, error) {
	// Get all book IDs (efficient - doesn't deserialize full books).
	bookIDs, err := s.store.GetAllBookIDs(ctx)
	if err != nil {
		return nil, err
	}

	// Get library checkpoint (ie. most recent UpdatedAt timestamp across all our entities).
	checkpoint, err := s.store.GetLibraryCheckpoint(ctx)
	if err != nil {
		return nil, err
	}

	// If no books exist, use current time as checkpoint.
	if checkpoint.IsZero() {
		checkpoint = time.Now()
	}

	// Get contributor count (efficient count-only query)
	contributorCount, err := s.store.CountContributors(ctx)
	if err != nil {
		return nil, err
	}

	// Get series count (efficient count-only query)
	seriesCount, err := s.store.CountSeries(ctx)
	if err != nil {
		return nil, err
	}

	// Build response.
	manifest := &ManifestResponse{
		LibraryVersion: checkpoint.Format(time.RFC3339),
		Checkpoint:     checkpoint.Format(time.RFC3339),
		BookIDs:        bookIDs,
	}

	manifest.Counts.Books = len(bookIDs)
	manifest.Counts.Contributors = contributorCount
	manifest.Counts.Series = seriesCount

	s.logger.Info("manifest generated",
		"book_count", len(bookIDs),
		"contributor_count", contributorCount,
		"series_count", seriesCount,
		"checkpoint", checkpoint.Format(time.RFC3339),
	)

	return manifest, nil
}

// BooksResponse represents paginated books with related entities.
// Uses shared dto.Book for consistency with SSE events.
type BooksResponse struct {
	NextCursor     string      `json:"next_cursor,omitempty"`
	Books          []*dto.Book `json:"books"`
	DeletedBookIDs []string    `json:"deleted_book_ids,omitempty"`
	HasMore        bool        `json:"has_more"`
}

// GetBooksForSync returns paginated books for sync.
// Supports both full sync (paginated iteration of all books) and delta sync (books updated after timestamp).
func (s *SyncService) GetBooksForSync(ctx context.Context, userID string, params store.PaginationParams) (*BooksResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate and set defaults.
	params.Validate()

	var books []*domain.Book
	var deletedBookIDs []string
	var err error

	// DELTA SYNC: If UpdatedAfter is set, fetch only changed items.
	if !params.UpdatedAfter.IsZero() {
		// 1. Get books updated after timestamp
		books, err = s.store.GetBooksForUserUpdatedAfter(ctx, userID, params.UpdatedAfter)
		if err != nil {
			return nil, err
		}

		// 2. Get books deleted after timestamp (for local deletion)
		// Note: We don't filter deletions by ACL because if a user had access to a book
		// that was deleted, they should probably delete it locally too.
		// Leaking that a book ID *existed* and was deleted is a minor privacy trade-off
		// for simpler sync logic.
		deletedBookIDs, err = s.store.GetBooksDeletedAfter(ctx, params.UpdatedAfter)
		if err != nil {
			return nil, err
		}

		// Delta sync results are typically small enough to not need pagination,
		// but we apply it anyway if the result set is huge.
		// However, for simplicity in this iteration, we return all changes in one go
		// if they fit in reasonable memory, or we could reuse the in-memory pagination logic below.
	} else {
		// FULL SYNC: Get all books the user can access (permissive access model)
		books, err = s.store.GetBooksForUser(ctx, userID)
		if err != nil {
			return nil, err
		}
	}

	// Apply cursor-based pagination manually (in-memory)
	// This applies to both full sync and delta sync results
	total := len(books)

	// Decode cursor to get starting index
	startIdx := 0
	if params.Cursor != "" {
		decoded, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		// For in-memory pagination, cursor is just the index as string
		if _, err := fmt.Sscanf(decoded, "%d", &startIdx); err != nil {
			return nil, err
		}
	}

	if startIdx >= total && len(deletedBookIDs) == 0 {
		return &BooksResponse{
			Books:   []*dto.Book{},
			HasMore: false,
		}, nil
	}

	// Calculate end index
	endIdx := startIdx + params.Limit
	if endIdx > total {
		endIdx = total
	}

	// Slice the results
	pageBooks := books[startIdx:endIdx]
	hasMore := endIdx < total

	// Enrich books with denormalized display fields.
	enrichedBooks, err := s.enricher.EnrichBooks(ctx, pageBooks)
	if err != nil {
		return nil, fmt.Errorf("failed to enrich books: %w", err)
	}

	response := &BooksResponse{
		Books:          enrichedBooks,
		DeletedBookIDs: deletedBookIDs, // Include deleted IDs (usually sent on first page of delta sync)
		HasMore:        hasMore,
	}

	// Only send deleted IDs on the first page to avoid duplication
	if startIdx > 0 {
		response.DeletedBookIDs = nil
	}

	// Set next cursor if there are more results
	if hasMore {
		response.NextCursor = store.EncodeCursor(fmt.Sprintf("%d", endIdx))
	}

	s.logger.Info("books fetched for sync",
		"user_id", userID,
		"delta_sync", !params.UpdatedAfter.IsZero(),
		"count", len(pageBooks),
		"deleted_count", len(deletedBookIDs),
		"has_more", hasMore,
	)

	return response, nil
}

// ContributorsResponse represents paginated contributors for sync.
type ContributorsResponse struct {
	NextCursor            string                `json:"next_cursor,omitempty"`
	Contributors          []*domain.Contributor `json:"contributors"`
	DeletedContributorIDs []string              `json:"deleted_contributor_ids,omitempty"`
	HasMore               bool                  `json:"has_more"`
}

// GetContributorsForSync returns paginated contributors for sync.
// Supports both full sync and delta sync (contributors updated after timestamp).
func (s *SyncService) GetContributorsForSync(ctx context.Context, userID string, params store.PaginationParams) (*ContributorsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	params.Validate()

	var contributors []*domain.Contributor
	var deletedContributorIDs []string
	var err error

	// DELTA SYNC: If UpdatedAfter is set, fetch only changed items.
	if !params.UpdatedAfter.IsZero() {
		// 1. Get contributors updated after timestamp
		contributors, err = s.store.GetContributorsUpdatedAfter(ctx, params.UpdatedAfter)
		if err != nil {
			return nil, err
		}

		// 2. Get contributors deleted after timestamp
		deletedContributorIDs, err = s.store.GetContributorsDeletedAfter(ctx, params.UpdatedAfter)
		if err != nil {
			return nil, err
		}
	} else {
		// FULL SYNC: Get all contributors
		// We use the existing list method which supports pagination
		result, err := s.store.ListContributors(ctx, params)
		if err != nil {
			return nil, err
		}

		return &ContributorsResponse{
			Contributors: result.Items,
			NextCursor:   result.NextCursor,
			HasMore:      result.HasMore,
		}, nil
	}

	// Apply manual pagination for delta sync results
	total := len(contributors)
	startIdx := 0
	if params.Cursor != "" {
		decoded, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		if _, err := fmt.Sscanf(decoded, "%d", &startIdx); err != nil {
			return nil, err
		}
	}

	if startIdx >= total && len(deletedContributorIDs) == 0 {
		return &ContributorsResponse{
			Contributors: []*domain.Contributor{},
			HasMore:      false,
		}, nil
	}

	endIdx := startIdx + params.Limit
	if endIdx > total {
		endIdx = total
	}

	pageItems := contributors[startIdx:endIdx]
	hasMore := endIdx < total

	response := &ContributorsResponse{
		Contributors:          pageItems,
		DeletedContributorIDs: deletedContributorIDs, // Include deleted IDs (usually sent on first page of delta sync)
		HasMore:               hasMore,
	}

	// Only send deleted IDs on the first page to avoid duplication
	if startIdx > 0 {
		response.DeletedContributorIDs = nil
	}

	if hasMore {
		response.NextCursor = store.EncodeCursor(fmt.Sprintf("%d", endIdx))
	}

	s.logger.Info("contributors fetched for sync",
		"user_id", userID,
		"delta_sync", !params.UpdatedAfter.IsZero(),
		"count", len(pageItems),
		"deleted_count", len(deletedContributorIDs),
		"has_more", hasMore,
	)

	return response, nil
}

// SeriesResponse represents paginated series for sync.
type SeriesResponse struct {
	NextCursor       string           `json:"next_cursor,omitempty"`
	Series           []*domain.Series `json:"series"`
	DeletedSeriesIDs []string         `json:"deleted_series_ids,omitempty"`
	HasMore          bool             `json:"has_more"`
}

// GetSeriesForSync returns paginated series for sync.
// Supports both full sync and delta sync (series updated after timestamp).
func (s *SyncService) GetSeriesForSync(ctx context.Context, userID string, params store.PaginationParams) (*SeriesResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	params.Validate()

	var seriesList []*domain.Series
	var deletedSeriesIDs []string
	var err error

	// DELTA SYNC: If UpdatedAfter is set, fetch only changed items.
	if !params.UpdatedAfter.IsZero() {
		// 1. Get series updated after timestamp
		seriesList, err = s.store.GetSeriesUpdatedAfter(ctx, params.UpdatedAfter)
		if err != nil {
			return nil, err
		}

		// 2. Get series deleted after timestamp
		deletedSeriesIDs, err = s.store.GetSeriesDeletedAfter(ctx, params.UpdatedAfter)
		if err != nil {
			return nil, err
		}
	} else {
		// FULL SYNC: Get all series
		result, err := s.store.ListSeries(ctx, params)
		if err != nil {
			return nil, err
		}

		return &SeriesResponse{
			Series:     result.Items,
			NextCursor: result.NextCursor,
			HasMore:    result.HasMore,
		}, nil
	}

	// Apply manual pagination for delta sync results
	total := len(seriesList)
	startIdx := 0
	if params.Cursor != "" {
		decoded, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, err
		}
		if _, err := fmt.Sscanf(decoded, "%d", &startIdx); err != nil {
			return nil, err
		}
	}

	if startIdx >= total && len(deletedSeriesIDs) == 0 {
		return &SeriesResponse{
			Series:  []*domain.Series{},
			HasMore: false,
		}, nil
	}

	endIdx := startIdx + params.Limit
	if endIdx > total {
		endIdx = total
	}

	pageItems := seriesList[startIdx:endIdx]
	hasMore := endIdx < total

	response := &SeriesResponse{
		Series:           pageItems,
		DeletedSeriesIDs: deletedSeriesIDs, // Include deleted IDs (usually sent on first page of delta sync)
		HasMore:          hasMore,
	}

	// Only send deleted IDs on the first page to avoid duplication
	if startIdx > 0 {
		response.DeletedSeriesIDs = nil
	}

	if hasMore {
		response.NextCursor = store.EncodeCursor(fmt.Sprintf("%d", endIdx))
	}

	s.logger.Info("series fetched for sync",
		"user_id", userID,
		"delta_sync", !params.UpdatedAfter.IsZero(),
		"count", len(pageItems),
		"deleted_count", len(deletedSeriesIDs),
		"has_more", hasMore,
	)

	return response, nil
}
