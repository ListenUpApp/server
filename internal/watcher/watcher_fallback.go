//go:build !linux

package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// fallbackBackend implements WatcherBackend using fsnotify with debouncing
type fallbackBackend struct {
	logger  *slog.Logger
	opts    Options
	watcher *fsnotify.Watcher

	pending map[string]*pendingEvent // path -> pending event info
	mu      sync.RWMutex             // protects pending map

	events chan Event
	errors chan error
	done   chan struct{}
	wg     sync.WaitGroup
}

// pendingEvent tracks a file that may still be changing
type pendingEvent struct {
	path    string
	size    int64
	modTime time.Time
	timer   *time.Timer
}

// newFallbackBackend creates a fallback backend using fsnotify
func newFallbackBackend(logger *slog.Logger, opts Options) (*fallbackBackend, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	return &fallbackBackend{
		logger:  logger,
		opts:    opts,
		watcher: watcher,
		pending: make(map[string]*pendingEvent),
		events:  make(chan Event, 100),
		errors:  make(chan error, 10),
		done:    make(chan struct{}),
	}, nil
}

// Watch adds a path to be monitored
func (b *fallbackBackend) Watch(path string) error {
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		return b.watchDir(path)
	}
	return b.watchFile(path)
}

// watchDir recursively watches a directory
func (b *fallbackBackend) watchDir(path string) error {
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			b.logger.Warn("failed to access path", "path", p, "error", err)
			return nil
		}

		if b.opts.shouldIgnore(p) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		if err := b.watcher.Add(p); err != nil {
			b.logger.Error("failed to add watch", "path", p, "error", err)
			return nil
		}

		b.logger.Debug("added watch", "path", p)
		return nil
	})
}

// watchFile watches a single file by watching its parent directory
func (b *fallbackBackend) watchFile(path string) error {
	dir := filepath.Dir(path)
	return b.watcher.Add(dir)
}

// Start begins watching for events
func (b *fallbackBackend) Start(ctx context.Context) error {
	b.wg.Add(1)
	go b.processEvents(ctx)

	<-ctx.Done()
	return nil
}

// processEvents processes fsnotify events
func (b *fallbackBackend) processEvents(ctx context.Context) {
	defer b.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		case event, ok := <-b.watcher.Events:
			if !ok {
				return
			}
			b.handleFsnotifyEvent(event)
		case err, ok := <-b.watcher.Errors:
			if !ok {
				return
			}
			b.errors <- err
		}
	}
}

// handleFsnotifyEvent handles an fsnotify event with debouncing
func (b *fallbackBackend) handleFsnotifyEvent(event fsnotify.Event) {
	path := event.Name

	// Skip ignored paths
	if b.opts.shouldIgnore(path) {
		return
	}

	// Handle directory creation
	if event.Op&fsnotify.Create != 0 {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			b.watchDir(path)
			return
		}
	}

	// Handle deletion
	if event.Op&fsnotify.Remove != 0 {
		b.cancelPending(path)
		b.emitEvent(Event{
			Type: EventRemoved,
			Path: path,
		})
		return
	}

	// Handle write/create events (need debouncing)
	if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
		b.startSettling(path)
	}
}

// startSettling begins the settling process for a file
func (b *fallbackBackend) startSettling(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Cancel existing timer if any
	if pending, exists := b.pending[path]; exists {
		pending.timer.Stop()
	}

	// Get current file info
	info, err := os.Stat(path)
	if err != nil {
		b.logger.Warn("failed to stat file", "path", path, "error", err)
		delete(b.pending, path)
		return
	}

	// Skip directories
	if info.IsDir() {
		return
	}

	// Create pending event
	pending := &pendingEvent{
		path:    path,
		size:    info.Size(),
		modTime: info.ModTime(),
	}

	// Start settle timer
	pending.timer = time.AfterFunc(b.opts.SettleDelay, func() {
		b.checkSettled(path)
	})

	b.pending[path] = pending
}

// checkSettled checks if a file has finished settling
func (b *fallbackBackend) checkSettled(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	pending, exists := b.pending[path]
	if !exists {
		return
	}

	// Check current file state
	info, err := os.Stat(path)
	if err != nil {
		// File was deleted
		delete(b.pending, path)
		b.emitEvent(Event{
			Type: EventRemoved,
			Path: path,
		})
		return
	}

	// Check if size/mtime changed
	if info.Size() != pending.size || info.ModTime() != pending.modTime {
		// Still changing, restart timer
		pending.size = info.Size()
		pending.modTime = info.ModTime()
		pending.timer = time.AfterFunc(b.opts.SettleDelay, func() {
			b.checkSettled(path)
		})
		return
	}

	// File has settled, emit event
	delete(b.pending, path)

	event := Event{
		Type:    EventAdded, // TODO: Track to determine if added or modified
		Path:    path,
		Inode:   getInode(info.Sys()),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}

	b.emitEvent(event)
}

// cancelPending cancels a pending event
func (b *fallbackBackend) cancelPending(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if pending, exists := b.pending[path]; exists {
		pending.timer.Stop()
		delete(b.pending, path)
	}
}

// emitEvent sends an event to the events channel
func (b *fallbackBackend) emitEvent(event Event) {
	select {
	case b.events <- event:
	case <-b.done:
	}
}

// Events returns the events channel
func (b *fallbackBackend) Events() <-chan Event {
	return b.events
}

// Errors returns the errors channel
func (b *fallbackBackend) Errors() <-chan error {
	return b.errors
}

// Stop stops the watcher
func (b *fallbackBackend) Stop() error {
	close(b.done)

	// Cancel all pending timers
	b.mu.Lock()
	for _, pending := range b.pending {
		pending.timer.Stop()
	}
	clear(b.pending) // Go 1.21+ - more idiomatic than make()
	b.mu.Unlock()

	// Close fsnotify watcher
	b.watcher.Close()

	// Wait for goroutines
	b.wg.Wait()

	close(b.events)
	close(b.errors)

	return nil
}

// newLinuxBackend is a stub that should never be called on non-Linux platforms
// It exists only to satisfy the compiler when watcher.go references it
func newLinuxBackend(logger *slog.Logger, opts Options) (WatcherBackend, error) {
	return nil, fmt.Errorf("Linux backend not available on this platform")
}
