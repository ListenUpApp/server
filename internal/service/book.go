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

// ListBooks returns a paginated list of books.
func (s *BookService) ListBooks(ctx context.Context, params store.PaginationParams) (*store.PaginatedResult[*domain.Book], error) {
	return s.store.ListBooks(ctx, params)
}

// GetBook retrieves a single book by ID.
func (s *BookService) GetBook(ctx context.Context, id string) (*domain.Book, error) {
	return s.store.GetBook(ctx, id)
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
	book, err := scanner.ConvertToBook(item)
	if err != nil {
		return nil, fmt.Errorf("convert to book: %w", err)
	}

	return book, nil
}
