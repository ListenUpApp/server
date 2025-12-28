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
	return h.Store.Close()
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

	return &StoreHandle{Store: db}, nil
}

// Bootstrap contains the library bootstrap result.
type Bootstrap struct {
	Library         *domain.Library
	InboxCollection *domain.Collection
	IsNewLibrary    bool
}

// ProvideBootstrap ensures the library exists and returns bootstrap info.
func ProvideBootstrap(i do.Injector) (*Bootstrap, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)
	storeHandle := do.MustInvoke[*StoreHandle](i)

	ctx := context.Background()
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
