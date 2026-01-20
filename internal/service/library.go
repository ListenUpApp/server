// Package service provides business logic layer for managing audiobooks, libraries, and synchronization.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// LibraryService orchestrates library operations.
type LibraryService struct {
	store      *store.Store
	sseManager *sse.Manager
	logger     *slog.Logger
}

// NewLibraryService creates a new library service.
func NewLibraryService(store *store.Store, sseManager *sse.Manager, logger *slog.Logger) *LibraryService {
	return &LibraryService{
		store:      store,
		sseManager: sseManager,
		logger:     logger,
	}
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
