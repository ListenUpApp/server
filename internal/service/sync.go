package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
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
	store  *store.Store
	logger *slog.Logger
}

// NewSyncService creates a new sync service.
func NewSyncService(store *store.Store, logger *slog.Logger) *SyncService {
	return &SyncService{
		store:  store,
		logger: logger,
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

	// Get contributor count
	contributors, err := s.store.ListContributors(ctx, store.PaginationParams{Limit: 10000})
	if err != nil {
		return nil, err
	}

	// Get series count
	series, err := s.store.ListSeries(ctx, store.PaginationParams{Limit: 10000})
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
	manifest.Counts.Contributors = len(contributors.Items)
	manifest.Counts.Series = len(series.Items)

	s.logger.Info("manifest generated",
		"book_count", len(bookIDs),
		"contributor_count", len(contributors.Items),
		"series_count", len(series.Items),
		"checkpoint", checkpoint.Format(time.RFC3339),
	)

	return manifest, nil
}

// BooksResponse represents paginated books with related entities.
type BooksResponse struct {
	NextCursor string         `json:"next_cursor,omitempty"`
	Books      []*domain.Book `json:"books"`
	HasMore    bool           `json:"has_more"`
}

// GetBooksForSync returns paginated books for initial sync.
// Only returns books the user has access to (permissive access model).
func (s *SyncService) GetBooksForSync(ctx context.Context, userID string, params store.PaginationParams) (*BooksResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate and set defaults.
	params.Validate()

	// Get all books the user can access (implements permissive access model)
	accessibleBooks, err := s.store.GetBooksForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Apply cursor-based pagination manually
	total := len(accessibleBooks)

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

	if startIdx >= total {
		return &BooksResponse{
			Books:   []*domain.Book{},
			HasMore: false,
		}, nil
	}

	// Calculate end index
	endIdx := startIdx + params.Limit
	if endIdx > total {
		endIdx = total
	}

	// Slice the results
	books := accessibleBooks[startIdx:endIdx]
	hasMore := endIdx < total

	response := &BooksResponse{
		Books:   books,
		HasMore: hasMore,
	}

	// Set next cursor if there are more results
	if hasMore {
		response.NextCursor = store.EncodeCursor(fmt.Sprintf("%d", endIdx))
	}

	s.logger.Info("books fetched for sync",
		"user_id", userID,
		"count", len(books),
		"has_more", hasMore,
	)

	return response, nil
}

// ContributorsResponse represents paginated contributors for sync.
type ContributorsResponse struct {
	NextCursor   string                `json:"next_cursor,omitempty"`
	Contributors []*domain.Contributor `json:"contributors"`
	HasMore      bool                  `json:"has_more"`
}

// GetContributorsForSync returns paginated contributors for initial sync.
// Note: Contributors don't have direct access control - they're visible if any book they contributed to is visible.
// For simplicity, we return all contributors and rely on client-side filtering based on accessible books.
func (s *SyncService) GetContributorsForSync(ctx context.Context, userID string, params store.PaginationParams) (*ContributorsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	params.Validate()

	result, err := s.store.ListContributors(ctx, params)
	if err != nil {
		return nil, err
	}

	response := &ContributorsResponse{
		Contributors: result.Items,
		NextCursor:   result.NextCursor,
		HasMore:      result.HasMore,
	}

	s.logger.Info("contributors fetched for sync",
		"user_id", userID,
		"count", len(result.Items),
		"has_more", result.HasMore,
	)

	return response, nil
}

// SeriesResponse represents paginated series for sync.
type SeriesResponse struct {
	NextCursor string           `json:"next_cursor,omitempty"`
	Series     []*domain.Series `json:"series"`
	HasMore    bool             `json:"has_more"`
}

// GetSeriesForSync returns paginated series for initial sync.
// Note: Series don't have direct access control - they're visible if any book in them is visible.
// For simplicity, we return all series and rely on client-side filtering based on accessible books.
func (s *SyncService) GetSeriesForSync(ctx context.Context, userID string, params store.PaginationParams) (*SeriesResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	params.Validate()

	result, err := s.store.ListSeries(ctx, params)
	if err != nil {
		return nil, err
	}

	response := &SeriesResponse{
		Series:     result.Items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}

	s.logger.Info("series fetched for sync",
		"user_id", userID,
		"count", len(result.Items),
		"has_more", result.HasMore,
	)

	return response, nil
}
