package providers

import (
	"context"
	"expvar"
	"sync"
	"sync/atomic"
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

	// Wire transcode queueing into the scanner. Transcode-deletion cascades
	// (e.g. when a book is removed) live at the service layer, not on the
	// store, so there is no longer a SetTranscodeDeleter hook to invoke here.
	fileScanner.SetTranscodeQueuer(svc)

	// Start workers
	svc.Start()

	log.Info("Transcode service started")

	return &TranscodeServiceHandle{TranscodeService: svc}, nil
}

var fileWatcherExpvarOnce sync.Once

// FileWatcherHandle wraps the file watcher with shutdown capability.
type FileWatcherHandle struct {
	*watcher.Watcher
	cancel       context.CancelFunc
	lastTickUnix int64
}

// Shutdown implements do.Shutdownable.
func (h *FileWatcherHandle) Shutdown() error {
	h.cancel()
	if h.Watcher != nil {
		return h.Stop()
	}
	return nil
}

// LastTick returns the wall-clock time of the most recent loop iteration.
func (h *FileWatcherHandle) LastTick() time.Time {
	return time.Unix(atomic.LoadInt64(&h.lastTickUnix), 0)
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
		handle := &FileWatcherHandle{
			Watcher: nil,
			cancel:  cancel,
		}
		atomic.StoreInt64(&handle.lastTickUnix, time.Now().Unix())
		fileWatcherExpvarOnce.Do(func() {
			pinned := handle
			expvar.Publish("file_watcher_last_tick_unix", expvar.Func(func() any {
				return atomic.LoadInt64(&pinned.lastTickUnix)
			}))
		})
		return handle, ctx.Err() // This will be nil
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

	handle := &FileWatcherHandle{
		Watcher: w,
		cancel:  cancel,
	}
	atomic.StoreInt64(&handle.lastTickUnix, time.Now().Unix())

	fileWatcherExpvarOnce.Do(func() {
		pinned := handle
		expvar.Publish("file_watcher_last_tick_unix", expvar.Func(func() any {
			return atomic.LoadInt64(&pinned.lastTickUnix)
		}))
	})

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
				atomic.StoreInt64(&handle.lastTickUnix, time.Now().Unix())
				if err := eventProcessor.ProcessEvent(ctx, event); err != nil {
					log.Warn("failed to process event",
						"error", err,
						"type", event.Type,
						"path", event.Path,
					)
				}
			case err := <-w.Errors():
				atomic.StoreInt64(&handle.lastTickUnix, time.Now().Unix())
				log.Warn("file watcher error", "error", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Info("File watcher started", "scan_paths", len(bootstrap.Library.ScanPaths))

	return handle, nil
}

var sessionCleanupExpvarOnce sync.Once

// SessionCleanupJob runs periodic session cleanup.
type SessionCleanupJob struct {
	cancel       context.CancelFunc
	lastTickUnix int64
}

// Shutdown implements do.Shutdownable.
func (j *SessionCleanupJob) Shutdown() error {
	j.cancel()
	return nil
}

// LastTick returns the wall-clock time of the most recent loop iteration.
func (j *SessionCleanupJob) LastTick() time.Time {
	return time.Unix(atomic.LoadInt64(&j.lastTickUnix), 0)
}

// ProvideSessionCleanupJob provides the periodic session cleanup job.
func ProvideSessionCleanupJob(i do.Injector) (*SessionCleanupJob, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	ctx, cancel := context.WithCancel(context.Background())

	job := &SessionCleanupJob{cancel: cancel}
	atomic.StoreInt64(&job.lastTickUnix, time.Now().Unix())

	sessionCleanupExpvarOnce.Do(func() {
		pinned := job
		expvar.Publish("session_cleanup_last_tick_unix", expvar.Func(func() any {
			return atomic.LoadInt64(&pinned.lastTickUnix)
		}))
	})

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Initial cleanup on startup
		if count, err := storeHandle.DeleteExpiredSessions(ctx); err != nil {
			log.Warn("Initial session cleanup failed", "error", err)
		} else if count > 0 {
			log.Info("Initial session cleanup completed", "deleted", count)
		}

		// Stale reading session cleanup ticker (every 15 minutes)
		staleTicker := time.NewTicker(15 * time.Minute)
		defer staleTicker.Stop()

		for {
			select {
			case <-ticker.C:
				atomic.StoreInt64(&job.lastTickUnix, time.Now().Unix())
				if count, err := storeHandle.DeleteExpiredSessions(ctx); err != nil {
					log.Warn("Session cleanup failed", "error", err)
				} else if count > 0 {
					log.Info("Session cleanup completed", "deleted", count)
				}
			case <-staleTicker.C:
				atomic.StoreInt64(&job.lastTickUnix, time.Now().Unix())
				if count, err := storeHandle.CleanupStaleSessions(ctx, 30*time.Minute); err != nil {
					log.Warn("Stale reading session cleanup failed", "error", err)
				} else if count > 0 {
					log.Info("Stale reading session cleanup completed", "cleaned", count)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Info("Session cleanup job started")

	return job, nil
}

var eventLogCleanupExpvarOnce sync.Once

// EventLogCleanupJob runs periodic SSE event log cleanup.
type EventLogCleanupJob struct {
	cancel       context.CancelFunc
	lastTickUnix int64
}

// Shutdown cancels the cleanup loop's context and stops the job.
func (j *EventLogCleanupJob) Shutdown() error {
	j.cancel()
	return nil
}

// LastTick returns the wall-clock time of the most recent loop iteration.
func (j *EventLogCleanupJob) LastTick() time.Time {
	return time.Unix(atomic.LoadInt64(&j.lastTickUnix), 0)
}

// ProvideEventLogCleanupJob provides periodic cleanup of the SSE event log.
func ProvideEventLogCleanupJob(i do.Injector) (*EventLogCleanupJob, error) {
	log := do.MustInvoke[*logger.Logger](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)

	ctx, cancel := context.WithCancel(context.Background())

	job := &EventLogCleanupJob{cancel: cancel}
	atomic.StoreInt64(&job.lastTickUnix, time.Now().Unix())

	eventLogCleanupExpvarOnce.Do(func() {
		pinned := job
		expvar.Publish("event_log_cleanup_last_tick_unix", expvar.Func(func() any {
			return atomic.LoadInt64(&pinned.lastTickUnix)
		}))
	})

	go func() {
		eventLogger := sseHandle.GetEventLogger()
		if eventLogger == nil {
			return
		}

		// Initial cleanup on startup.
		if count, err := eventLogger.CleanupEventLog(ctx, 24*time.Hour); err != nil {
			log.Warn("Initial event log cleanup failed", "error", err)
		} else if count > 0 {
			log.Info("Initial event log cleanup completed", "deleted", count)
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				atomic.StoreInt64(&job.lastTickUnix, time.Now().Unix())
				if count, err := eventLogger.CleanupEventLog(ctx, 24*time.Hour); err != nil {
					log.Warn("Event log cleanup failed", "error", err)
				} else if count > 0 {
					log.Info("Event log cleanup completed", "deleted", count)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Info("Event log cleanup job started")

	return job, nil
}
