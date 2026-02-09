package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// SettingsService manages server-wide settings.
type SettingsService struct {
	store        *store.Store
	inboxService *InboxService
	logger       *slog.Logger
}

// NewSettingsService creates a new settings service.
func NewSettingsService(store *store.Store, inboxService *InboxService, logger *slog.Logger) *SettingsService {
	return &SettingsService{
		store:        store,
		inboxService: inboxService,
		logger:       logger,
	}
}

// GetServerSettings retrieves server-wide settings.
func (s *SettingsService) GetServerSettings(ctx context.Context) (*domain.ServerSettings, error) {
	return s.store.GetServerSettings(ctx)
}

// SettingsUpdate contains fields that can be updated.
type SettingsUpdate struct {
	Name         *string
	InboxEnabled *bool
}

// UpdateServerSettings updates server-wide settings.
// If disabling inbox with books in it, auto-releases all books.
func (s *SettingsService) UpdateServerSettings(ctx context.Context, update *SettingsUpdate) (*domain.ServerSettings, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	current, err := s.store.GetServerSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get current settings: %w", err)
	}

	// Handle inbox disable with auto-release
	if update.InboxEnabled != nil && current.InboxEnabled && !*update.InboxEnabled {
		inboxBooks, err := s.inboxService.ListBooks(ctx)
		if err != nil {
			return nil, fmt.Errorf("list inbox books: %w", err)
		}

		if len(inboxBooks) > 0 {
			s.logger.Info("auto-releasing inbox books before disable",
				"book_count", len(inboxBooks),
			)

			bookIDs := make([]string, len(inboxBooks))
			for i, book := range inboxBooks {
				bookIDs[i] = book.ID
			}

			result, err := s.inboxService.ReleaseBooks(ctx, bookIDs)
			if err != nil {
				return nil, fmt.Errorf("auto-release inbox books: %w", err)
			}

			s.logger.Info("inbox books auto-released",
				"released", result.Released,
				"public", result.Public,
				"to_collections", result.ToCollections,
			)
		}
	}

	// Apply updates
	if update.Name != nil {
		current.Name = *update.Name
	}
	if update.InboxEnabled != nil {
		current.InboxEnabled = *update.InboxEnabled
	}
	current.UpdatedAt = time.Now()

	if err := s.store.UpdateServerSettings(ctx, current); err != nil {
		return nil, fmt.Errorf("update settings: %w", err)
	}

	s.logger.Info("server settings updated",
		"name", current.Name,
		"inbox_enabled", current.InboxEnabled,
	)

	return current, nil
}
