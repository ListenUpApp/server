package providers

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// SSEManagerHandle wraps the SSE manager with its context for lifecycle management.
type SSEManagerHandle struct {
	*sse.Manager
	cancel context.CancelFunc
}

// Shutdown implements do.Shutdownable.
func (h *SSEManagerHandle) Shutdown() error {
	h.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return h.Manager.Shutdown(ctx)
}

// ProvideSSEManager provides the server-sent events manager.
func ProvideSSEManager(i do.Injector) (*SSEManagerHandle, error) {
	log := do.MustInvoke[*logger.Logger](i)

	manager := sse.NewManager(log.Logger)

	// Start in background
	ctx, cancel := context.WithCancel(context.Background())
	go manager.Start(ctx)

	log.Info("SSE manager started")

	return &SSEManagerHandle{
		Manager: manager,
		cancel:  cancel,
	}, nil
}

// StoreHandle wraps the store with shutdown capability.
type StoreHandle struct {
	*store.Store
}

// Shutdown implements do.Shutdownable.
func (h *StoreHandle) Shutdown() error {
	return h.Close()
}

// ProvideStore provides the database store.
func ProvideStore(i do.Injector) (*StoreHandle, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)

	dbPath := filepath.Join(cfg.Metadata.BasePath, "db")
	db, err := store.New(dbPath, log.Logger, sseHandle.Manager)
	if err != nil {
		return nil, err
	}

	log.Info("Database initialized", "path", dbPath)

	// Wire up book access filtering for SSE broadcasts.
	// Activity and session events are filtered per-client based on book ACLs.
	sseHandle.SetBookAccessChecker(func(ctx context.Context, userID, bookID string) bool {
		canAccess, err := db.CanUserAccessBook(ctx, userID, bookID)
		if err != nil {
			log.Warn("SSE book access check failed, denying",
				"user_id", userID,
				"book_id", bookID,
				"error", err)
			return false
		}
		return canAccess
	})

	return &StoreHandle{Store: db}, nil
}

// Bootstrap contains the library bootstrap result.
type Bootstrap struct {
	Library         *domain.Library
	InboxCollection *domain.Collection
	IsNewLibrary    bool
}

// ProvideBootstrap ensures the library exists and returns bootstrap info.
// If no audiobook path is configured, returns nil Library to allow setup via API.
func ProvideBootstrap(i do.Injector) (*Bootstrap, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)
	storeHandle := do.MustInvoke[*StoreHandle](i)

	ctx := context.Background()

	// If no audiobook path is configured, check if library already exists
	if cfg.Library.AudiobookPath == "" {
		library, err := storeHandle.GetDefaultLibrary(ctx)
		if err != nil {
			// No library exists and none configured - server will start without library
			// Admin can set it up via POST /api/v1/library/setup
			log.Info("No library configured - setup required via API")
			return &Bootstrap{
				Library:         nil,
				InboxCollection: nil,
				IsNewLibrary:    false,
			}, nil
		}

		// Library exists from previous setup
		inbox, _ := storeHandle.GetInboxForLibrary(ctx, library.ID)
		log.Info("Using existing library",
			"library_id", library.ID,
			"library_name", library.Name,
			"scan_paths", len(library.ScanPaths),
		)
		return &Bootstrap{
			Library:         library,
			InboxCollection: inbox,
			IsNewLibrary:    false,
		}, nil
	}

	// Audiobook path is configured - ensure library exists
	result, err := storeHandle.EnsureLibrary(ctx, cfg.Library.AudiobookPath, "system")
	if err != nil {
		return nil, err
	}

	log.Info("Library ready",
		"library_id", result.Library.ID,
		"library_name", result.Library.Name,
		"owner_id", result.Library.OwnerID,
		"scan_paths", len(result.Library.ScanPaths),
		"is_new", result.IsNewLibrary,
		"inbox_collection", result.InboxCollection.ID,
	)

	return &Bootstrap{
		Library:         result.Library,
		InboxCollection: result.InboxCollection,
		IsNewLibrary:    result.IsNewLibrary,
	}, nil
}

// ProvideSlogLogger provides access to the underlying slog.Logger for packages that need it.
func ProvideSlogLogger(i do.Injector) (*slog.Logger, error) {
	log := do.MustInvoke[*logger.Logger](i)
	return log.Logger, nil
}

// ProvideRegistrationBroadcaster provides the registration status broadcaster for pending users.
func ProvideRegistrationBroadcaster(i do.Injector) (*sse.RegistrationBroadcaster, error) {
	log := do.MustInvoke[*logger.Logger](i)
	return sse.NewRegistrationBroadcaster(log.Logger), nil
}
