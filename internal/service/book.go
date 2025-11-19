// Package service provides business logic layer for managing audiobooks, libraries, and synchronization.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/store"
)

// BookService orchestrates book operations.
type BookService struct {
	store   *store.Store
	scanner *scanner.Scanner
	logger  *slog.Logger
}

// NewBookService creates a new book service.
func NewBookService(store *store.Store, scanner *scanner.Scanner, logger *slog.Logger) *BookService {
	return &BookService{
		store:   store,
		scanner: scanner,
		logger:  logger,
	}
}

// ListBooks returns a paginated list of books accessible to the user.
// User can see books that are: (1) not in any collection, OR (2) in at least one collection they have access to.
func (s *BookService) ListBooks(ctx context.Context, userID string, params store.PaginationParams) (*store.PaginatedResult[*domain.Book], error) {
	params.Validate()

	// Get all books the user can access
	accessibleBooks, err := s.store.GetBooksForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get accessible books: %w", err)
	}

	// Apply cursor-based pagination manually
	total := len(accessibleBooks)

	// Decode cursor to get starting index
	startIdx := 0
	if params.Cursor != "" {
		decoded, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		// For in-memory pagination, cursor is just the index as string
		if _, err := fmt.Sscanf(decoded, "%d", &startIdx); err != nil {
			return nil, fmt.Errorf("invalid cursor format: %w", err)
		}
	}

	if startIdx >= total {
		return &store.PaginatedResult[*domain.Book]{
			Items:   []*domain.Book{},
			Total:   total,
			HasMore: false,
		}, nil
	}

	// Calculate end index
	endIdx := startIdx + params.Limit
	if endIdx > total {
		endIdx = total
	}

	// Slice the results
	items := accessibleBooks[startIdx:endIdx]
	hasMore := endIdx < total

	result := &store.PaginatedResult[*domain.Book]{
		Items:   items,
		Total:   total,
		HasMore: hasMore,
	}

	// Set next cursor if there are more results
	if hasMore {
		result.NextCursor = store.EncodeCursor(fmt.Sprintf("%d", endIdx))
	}

	return result, nil
}

// GetBook retrieves a single book by ID.
// Returns ErrBookNotFound if user doesn't have access to the book.
func (s *BookService) GetBook(ctx context.Context, userID, id string) (*domain.Book, error) {
	return s.store.GetBook(ctx, id, userID)
}

// TriggerScan initiates a full library scan.
// Returns the scan result including statistics about added/updated/unchanged books.
func (s *BookService) TriggerScan(ctx context.Context, libraryID string, opts scanner.ScanOptions) (*scanner.ScanResult, error) {
	// Get library to find scan paths.
	library, err := s.store.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	if len(library.ScanPaths) == 0 {
		return nil, fmt.Errorf("library has no scan paths configured")
	}

	s.logger.Info("triggering library scan",
		"library_id", libraryID,
		"library_name", library.Name,
		"scan_paths", len(library.ScanPaths),
	)

	// For now, scan the first path.
	// TODO: Support scanning multiple paths and aggregating results
	scanPath := library.ScanPaths[0]

	// Set library ID in options for event emission.
	opts.LibraryID = libraryID

	result, err := s.scanner.Scan(ctx, scanPath, opts)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	s.logger.Info("scan complete",
		"library_id", libraryID,
		"added", result.Added,
		"updated", result.Updated,
		"unchanged", result.Unchanged,
		"errors", result.Errors,
		"duration", result.CompletedAt.Sub(result.StartedAt),
	)

	return result, nil
}

// ScanFolder scans a specific folder and returns the book data without saving.
// Useful for previewing what would be scanned.
func (s *BookService) ScanFolder(ctx context.Context, folderPath string) (*domain.Book, error) {
	// Scan the folder.
	item, err := s.scanner.ScanFolder(ctx, folderPath, scanner.ScanOptions{})
	if err != nil {
		return nil, fmt.Errorf("scan folder: %w", err)
	}

	// Convert to book.
	book, err := scanner.ConvertToBook(ctx, item, s.store)
	if err != nil {
		return nil, fmt.Errorf("convert to book: %w", err)
	}

	return book, nil
}
