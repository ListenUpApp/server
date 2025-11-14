package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ManifestResponse is the response for GET /api/v1/sync/manifest
// This is the first phase of initial sync, the idea is to give the client
// minimal info to display immediately to the user while we fetch the real
// stuff in the background.
type ManifestResponse struct {
	LibraryVersion string `json:"library_version"` // RFC3339 timestamp
	Checkpoint     string `json:"checkpoint"`      // Same as library_version, for clarity
	Counts         struct {
		Books   int `json:"books"`
		Authors int `json:"authors"` // Future: will be > 0 when authors implemented
		Series  int `json:"series"`  // Future: will be > 0 when series implemented
	} `json:"counts"`
	BookIDs []string `json:"book_ids"` // All book IDs in library
}

// SyncService orchestrates sync operations between server and clients
type SyncService struct {
	store  *store.Store
	logger *slog.Logger
}

// NewSyncService creates a new sync service
func NewSyncService(store *store.Store, logger *slog.Logger) *SyncService {
	return &SyncService{
		store:  store,
		logger: logger,
	}
}

// GetManifest returns the library manifest for sync
// This provides a high-level overview of the library state including
// the current checkpoint and counts of various entities
func (s *SyncService) GetManifest(ctx context.Context) (*ManifestResponse, error) {
	// Get all book IDs (efficient - doesn't deserialize full books)
	bookIDs, err := s.store.GetAllBookIDs(ctx)
	if err != nil {
		return nil, err
	}

	// Get library checkpoint (ie. most recent UpdatedAt timestamp across all our entities)
	checkpoint, err := s.store.GetLibraryCheckpoint(ctx)
	if err != nil {
		return nil, err
	}

	// If no books exist, use current time as checkpoint
	if checkpoint.IsZero() {
		checkpoint = time.Now()
	}

	// Build response
	manifest := &ManifestResponse{
		LibraryVersion: checkpoint.Format(time.RFC3339),
		Checkpoint:     checkpoint.Format(time.RFC3339),
		BookIDs:        bookIDs,
	}

	manifest.Counts.Books = len(bookIDs)
	manifest.Counts.Authors = 0 // Adding this for the future
	manifest.Counts.Series = 0  // Adding this for the future

	s.logger.Info("manifest generated",
		"book_count", len(bookIDs),
		"checkpoint", checkpoint.Format(time.RFC3339),
	)

	return manifest, nil
}

// BooksResponse represents paginated books with related entities
type BooksResponse struct {
	Books      []*domain.Book `json:"books"`
	NextCursor string         `json:"next_cursor,omitempty"`
	HasMore    bool           `json:"has_more"`
}

// GetBooksForSync returns paginated books for initial sync
func (s *SyncService) GetBooksForSync(ctx context.Context, params store.PaginationParams) (*BooksResponse, error) {
	// Validate and set defaults
	params.Validate()

	result, err := s.store.ListBooks(ctx, params)
	if err != nil {
		return nil, err
	}

	response := &BooksResponse{
		Books:      result.Items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}

	s.logger.Info("books fetched for sync",
		"count", len(result.Items),
		"has_more", result.HasMore,
	)

	return response, nil
}
