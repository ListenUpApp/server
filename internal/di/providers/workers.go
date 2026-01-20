package providers

import (
	"context"
	"time"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/processor"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/watcher"
)

// TranscodeServiceHandle wraps the transcode service with shutdown capability.
type TranscodeServiceHandle struct {
	*service.TranscodeService
}

// Shutdown implements do.Shutdownable.
func (h *TranscodeServiceHandle) Shutdown() error {
	h.Stop()
	return nil
}

// ProvideTranscodeService provides the audio transcoding service.
func ProvideTranscodeService(i do.Injector) (*TranscodeServiceHandle, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)
	fileScanner := do.MustInvoke[*scanner.Scanner](i)

	svc, err := service.NewTranscodeService(storeHandle.Store, sseHandle.Manager, cfg.Transcode, log.Logger)
	if err != nil {
		return nil, err
	}

	// Wire to scanner and store
	fileScanner.SetTranscodeQueuer(svc)
	storeHandle.SetTranscodeDeleter(svc)

	// Start workers
	svc.Start()

	log.Info("Transcode service started")

	return &TranscodeServiceHandle{TranscodeService: svc}, nil
}

// FileWatcherHandle wraps the file watcher with shutdown capability.
type FileWatcherHandle struct {
	*watcher.Watcher
	cancel context.CancelFunc
}

// Shutdown implements do.Shutdownable.
func (h *FileWatcherHandle) Shutdown() error {
	h.cancel()
	if h.Watcher != nil {
		return h.Stop()
	}
	return nil
}

// ProvideFileWatcher provides the file system watcher.
func ProvideFileWatcher(i do.Injector) (*FileWatcherHandle, error) {
	log := do.MustInvoke[*logger.Logger](i)
	bootstrap := do.MustInvoke[*Bootstrap](i)
	eventProcessor := do.MustInvoke[*processor.EventProcessor](i)

	// If no library exists yet, return a no-op watcher
	if bootstrap.Library == nil {
		log.Info("No library configured - file watcher disabled until setup")
		ctx, cancel := context.WithCancel(context.Background())
		return &FileWatcherHandle{
			Watcher: nil,
			cancel:  cancel,
		}, ctx.Err() // This will be nil
	}

	w, err := watcher.New(log.Logger, watcher.Options{IgnoreHidden: true})
	if err != nil {
		return nil, err
	}

	// Watch library paths (skip non-existent paths gracefully)
	watchedPaths := 0
	for _, scanPath := range bootstrap.Library.ScanPaths {
		if err := w.Watch(scanPath); err != nil {
			// Log warning but continue - path may not exist yet or may have been removed
			log.Warn("Cannot watch scan path", "path", scanPath, "error", err)
			continue
		}
		log.Info("Watching scan path", "path", scanPath)
		watchedPaths++
	}

	if watchedPaths == 0 {
		log.Warn("No valid scan paths to watch - library paths may not exist")
	}

	// Start in background
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if err := w.Start(ctx); err != nil {
			log.Error("File watcher error", "error", err)
		}
	}()

	// Process events in background
	go func() {
		for {
			select {
			case event := <-w.Events():
				if err := eventProcessor.ProcessEvent(ctx, event); err != nil {
					log.Warn("failed to process event",
						"error", err,
						"type", event.Type,
						"path", event.Path,
					)
				}
			case err := <-w.Errors():
				log.Warn("file watcher error", "error", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Info("File watcher started", "scan_paths", len(bootstrap.Library.ScanPaths))

	return &FileWatcherHandle{
		Watcher: w,
		cancel:  cancel,
	}, nil
}

// SessionCleanupJob runs periodic session cleanup.
type SessionCleanupJob struct {
	cancel context.CancelFunc
}

// Shutdown implements do.Shutdownable.
func (j *SessionCleanupJob) Shutdown() error {
	j.cancel()
	return nil
}

// ProvideSessionCleanupJob provides the periodic session cleanup job.
func ProvideSessionCleanupJob(i do.Injector) (*SessionCleanupJob, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Initial cleanup on startup
		if count, err := storeHandle.DeleteExpiredSessions(ctx); err != nil {
			log.Warn("Initial session cleanup failed", "error", err)
		} else if count > 0 {
			log.Info("Initial session cleanup completed", "deleted", count)
		}

		for {
			select {
			case <-ticker.C:
				if count, err := storeHandle.DeleteExpiredSessions(ctx); err != nil {
					log.Warn("Session cleanup failed", "error", err)
				} else if count > 0 {
					log.Info("Session cleanup completed", "deleted", count)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Info("Session cleanup job started")

	return &SessionCleanupJob{cancel: cancel}, nil
}
