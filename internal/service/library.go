// Package service provides business logic layer for managing audiobooks, libraries, and synchronization.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// libraryServiceStore is the narrow store interface required by LibraryService.
type libraryServiceStore interface {
	store.LibraryStore
	store.UserStore
	store.BookStore
}

// LibraryService orchestrates library operations.
type LibraryService struct {
	store      libraryServiceStore
	sseManager *sse.Manager
	logger     *slog.Logger
}

// NewLibraryService creates a new library service.
func NewLibraryService(libraries libraryServiceStore, sseManager *sse.Manager, logger *slog.Logger) *LibraryService {
	return &LibraryService{
		store:      libraries,
		sseManager: sseManager,
		logger:     logger,
	}
}

// ListLibraries returns all libraries.
func (s *LibraryService) ListLibraries(ctx context.Context) ([]*domain.Library, error) {
	return s.store.ListLibraries(ctx)
}

// GetLibrary returns a single library by ID.
func (s *LibraryService) GetLibrary(ctx context.Context, id string) (*domain.Library, error) {
	return s.store.GetLibrary(ctx, id)
}

// LibraryStatusResult contains the result of GetLibraryStatus.
type LibraryStatusResult struct {
	Exists    bool
	Library   *domain.Library
	IsAdmin   bool
	BookCount int
}

// GetLibraryStatus returns library existence and basic info for a user.
func (s *LibraryService) GetLibraryStatus(ctx context.Context, userID string) (*LibraryStatusResult, error) {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	library, err := s.store.GetDefaultLibrary(ctx)
	if err != nil {
		// No library exists — report via status fields, not error.
		return &LibraryStatusResult{ //nolint:nilerr // missing library is reported via status field, not error
			Exists:  false,
			IsAdmin: user.IsAdmin(),
		}, nil
	}

	bookCount, _ := s.store.CountBooks(ctx)

	return &LibraryStatusResult{
		Exists:    true,
		Library:   library,
		IsAdmin:   user.IsAdmin(),
		BookCount: bookCount,
	}, nil
}

// SetupLibraryInput is the input for setting up a library.
type SetupLibraryInput struct {
	AdminID    string
	Name       string
	ScanPaths  []string // At least one path required.
	SkipInbox  *bool
}

// SetupLibrary creates the initial library if none exists.
// Returns the created library. Returns an error if a library already exists.
func (s *LibraryService) SetupLibrary(ctx context.Context, input SetupLibraryInput) (*domain.Library, error) {
	if existing, err := s.store.GetDefaultLibrary(ctx); err == nil && existing != nil {
		return nil, errors.New("library already exists")
	}

	result, err := s.store.EnsureLibrary(ctx, input.ScanPaths[0], input.AdminID)
	if err != nil {
		return nil, fmt.Errorf("ensure library: %w", err)
	}

	lib := result.Library
	lib.Name = input.Name
	if input.SkipInbox != nil {
		lib.SkipInbox = *input.SkipInbox
	}

	// Add additional scan paths
	for _, p := range input.ScanPaths[1:] {
		lib.AddScanPath(p)
	}

	lib.UpdatedAt = time.Now()
	if err := s.store.UpdateLibrary(ctx, lib); err != nil {
		return nil, fmt.Errorf("update library: %w", err)
	}

	return lib, nil
}

// AddScanPath adds a scan path to an existing library.
func (s *LibraryService) AddScanPath(ctx context.Context, libraryID, cleanPath string) (*domain.Library, error) {
	lib, err := s.store.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, err
	}

	lib.AddScanPath(cleanPath)
	lib.UpdatedAt = time.Now()

	if err := s.store.UpdateLibrary(ctx, lib); err != nil {
		return nil, fmt.Errorf("update library: %w", err)
	}

	return lib, nil
}

// RemoveScanPath removes a scan path from an existing library.
func (s *LibraryService) RemoveScanPath(ctx context.Context, libraryID, cleanPath string) (*domain.Library, error) {
	lib, err := s.store.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, err
	}

	lib.RemoveScanPath(cleanPath)
	lib.UpdatedAt = time.Now()

	if err := s.store.UpdateLibrary(ctx, lib); err != nil {
		return nil, fmt.Errorf("update library: %w", err)
	}

	return lib, nil
}

// UpdateLibrary updates a library's settings.
// Only admins can update libraries (enforced at API layer).
func (s *LibraryService) UpdateLibrary(ctx context.Context, libraryID string, name *string, accessMode *string) (*domain.Library, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get library
	lib, err := s.store.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	// Track if access mode is changing
	oldAccessMode := lib.GetAccessMode()
	accessModeChanged := false

	// Update fields if provided
	if name != nil {
		lib.Name = *name
	}
	if accessMode != nil {
		newMode := domain.AccessMode(*accessMode)
		if newMode != oldAccessMode {
			lib.AccessMode = newMode
			accessModeChanged = true
		}
	}

	// Save changes
	lib.UpdatedAt = time.Now()
	if err := s.store.UpdateLibrary(ctx, lib); err != nil {
		return nil, fmt.Errorf("update library: %w", err)
	}

	s.logger.Info("library updated",
		"library_id", libraryID,
		"name", lib.Name,
		"access_mode", lib.GetAccessMode(),
	)

	// Broadcast SSE event if access mode changed
	// This tells all connected clients to refresh their book lists
	if accessModeChanged && s.sseManager != nil {
		event := sse.NewLibraryAccessModeChangedEvent(libraryID, string(lib.GetAccessMode()))
		s.sseManager.Emit(event)
		s.logger.Info("broadcast library access mode change",
			"library_id", libraryID,
			"old_mode", oldAccessMode,
			"new_mode", lib.GetAccessMode(),
		)
	}

	return lib, nil
}
