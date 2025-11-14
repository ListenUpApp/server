package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
)

// Watcher monitors file system changes
// Life before death - we watch to protect your library
type Watcher struct {
	backend WatcherBackend
	logger  *slog.Logger
}

// New creates a new file watcher
// The watcher automatically selects the best backend for the current platform:
// - Linux: Uses inotify with IN_CLOSE_WRITE for instant detection, Likely going to be used in 99% of production cases
// - Others: Uses fsnotify with debouncing for reliable detection, great for development.
func New(logger *slog.Logger, opts Options) (*Watcher, error) {
	// Apply defaults
	opts.setDefaults()

	// Create platform-specific backend
	var backend WatcherBackend
	var err error

	if runtime.GOOS == "linux" {
		backend, err = newLinuxBackend(logger, opts)
		logger.Info("using Linux inotify backend with IN_CLOSE_WRITE")
	} else {
		backend, err = newFallbackBackend(logger, opts)
		logger.Info("using fsnotify fallback backend", "platform", runtime.GOOS)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create backend: %w", err)
	}

	return &Watcher{
		backend: backend,
		logger:  logger,
	}, nil
}

// Watch adds a path to be monitored
// The path can be a file or directory. Directories are watched recursively.
func (w *Watcher) Watch(path string) error {
	return w.backend.Watch(path)
}

// Start begins watching for events
// This method blocks until the context is cancelled
func (w *Watcher) Start(ctx context.Context) error {
	return w.backend.Start(ctx)
}

// Stop stops the watcher and releases resources
func (w *Watcher) Stop() error {
	return w.backend.Stop()
}

// Events returns the channel for receiving file system events
func (w *Watcher) Events() <-chan Event {
	return w.backend.Events()
}

// Errors returns the channel for receiving errors
func (w *Watcher) Errors() <-chan error {
	return w.backend.Errors()
}
