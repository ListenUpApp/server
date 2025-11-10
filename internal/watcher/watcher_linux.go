//go:build linux

package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// linuxBackend implements WatcherBackend using Linux inotify with IN_CLOSE_WRITE
type linuxBackend struct {
	logger *slog.Logger
	opts   Options

	fd      int            // inotify file descriptor
	watches map[string]int // path -> watch descriptor
	wdPaths map[int]string // watch descriptor -> path
	mu      sync.RWMutex   // protects watches and wdPaths

	events chan Event
	errors chan error
	done   chan struct{}
	wg     sync.WaitGroup
}

// newLinuxBackend creates a new Linux-specific file watcher backend
func newLinuxBackend(logger *slog.Logger, opts Options) (*linuxBackend, error) {
	// Initialize inotify
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize inotify: %w", err)
	}

	return &linuxBackend{
		logger:  logger,
		opts:    opts,
		fd:      fd,
		watches: make(map[string]int),
		wdPaths: make(map[int]string),
		events:  make(chan Event, 100),
		errors:  make(chan error, 10),
		done:    make(chan struct{}),
	}, nil
}

// Watch adds a path to be monitored
func (b *linuxBackend) Watch(path string) error {
	// Clean the path
	path = filepath.Clean(path)

	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	// Add watch for this path
	if info.IsDir() {
		return b.watchDir(path)
	}
	return b.watchFile(path)
}

// watchDir recursively watches a directory
func (b *linuxBackend) watchDir(path string) error {
	// Walk the directory tree
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			b.logger.Warn("failed to access path", "path", p, "error", err)
			return nil // Continue walking
		}

		// Skip ignored paths
		if b.opts.shouldIgnore(p) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only watch directories
		if !info.IsDir() {
			return nil
		}

		// Add inotify watch
		if err := b.addWatch(p); err != nil {
			b.logger.Error("failed to add watch", "path", p, "error", err)
			return nil // Continue walking
		}

		return nil
	})
}

// watchFile watches a single file by watching its parent directory
func (b *linuxBackend) watchFile(path string) error {
	dir := filepath.Dir(path)
	return b.addWatch(dir)
}

// addWatch adds an inotify watch for a path
func (b *linuxBackend) addWatch(path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if already watching
	if _, exists := b.watches[path]; exists {
		return nil
	}

	// Add inotify watch
	// IN_CLOSE_WRITE: File closed after writing (perfect for our use case!)
	// IN_MOVED_TO: File moved into watched directory
	// IN_CREATE: Directory created (we need to watch new directories)
	// IN_DELETE: File/directory deleted
	// IN_ONLYDIR: Only watch if path is a directory
	mask := unix.IN_CLOSE_WRITE | unix.IN_MOVED_TO | unix.IN_CREATE | unix.IN_DELETE

	wd, err := unix.InotifyAddWatch(b.fd, path, uint32(mask))
	if err != nil {
		return fmt.Errorf("inotify_add_watch failed: %w", err)
	}

	b.watches[path] = wd
	b.wdPaths[wd] = path
	b.logger.Debug("added watch", "path", path, "wd", wd)

	return nil
}

// Start begins watching for events
func (b *linuxBackend) Start(ctx context.Context) error {
	b.wg.Add(1)
	go b.readEvents(ctx)

	// Wait for context cancellation or done signal
	<-ctx.Done()
	return nil
}

// readEvents reads events from inotify
func (b *linuxBackend) readEvents(ctx context.Context) {
	defer b.wg.Done()

	buf := make([]byte, unix.SizeofInotifyEvent*100)

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.done:
			return
		default:
			// Read events (this blocks until events are available)
			n, err := unix.Read(b.fd, buf)
			if err != nil {
				if err == unix.EINTR {
					continue // Interrupted, try again
				}
				if err == unix.EAGAIN {
					continue // No data available, try again
				}
				b.errors <- fmt.Errorf("failed to read inotify events: %w", err)
				return
			}

			if n < unix.SizeofInotifyEvent {
				continue // Not enough data
			}

			b.parseEvents(buf[:n])
		}
	}
}

// parseEvents parses raw inotify events
func (b *linuxBackend) parseEvents(buf []byte) {
	offset := 0
	for offset < len(buf) {
		event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
		offset += unix.SizeofInotifyEvent + int(event.Len)

		// Get the path for this watch descriptor
		b.mu.RLock()
		dir, ok := b.wdPaths[int(event.Wd)]
		b.mu.RUnlock()

		if !ok {
			continue
		}

		// Get the full path
		name := ""
		if event.Len > 0 {
			nameBytes := buf[offset-int(event.Len) : offset]
			name = string(nameBytes[:clen(nameBytes)])
		}

		path := filepath.Join(dir, name)

		// Process the event
		b.processEvent(path, event.Mask)
	}
}

// processEvent processes a single inotify event
func (b *linuxBackend) processEvent(path string, mask uint32) {
	// Skip ignored paths
	if b.opts.shouldIgnore(path) {
		return
	}

	// Handle directory creation
	if mask&unix.IN_CREATE != 0 {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			// New directory created, watch it
			b.watchDir(path)
			return
		}
	}

	// Handle file deletion
	if mask&unix.IN_DELETE != 0 {
		b.emitEvent(Event{
			Type: EventRemoved,
			Path: path,
		})
		return
	}

	// Handle file close after write (file is ready!)
	if mask&unix.IN_CLOSE_WRITE != 0 {
		b.handleFileReady(path)
		return
	}

	// Handle file moved into directory
	if mask&unix.IN_MOVED_TO != 0 {
		b.handleFileReady(path)
		return
	}
}

// handleFileReady handles a file that is ready to be processed
func (b *linuxBackend) handleFileReady(path string) {
	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		b.logger.Warn("failed to stat file", "path", path, "error", err)
		return
	}

	// Skip directories
	if info.IsDir() {
		return
	}

	// Create event
	event := Event{
		Type:    EventAdded, // TODO: Track files to determine if added or modified
		Path:    path,
		Inode:   getInode(info.Sys()),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}

	b.emitEvent(event)
}

// emitEvent sends an event to the events channel
func (b *linuxBackend) emitEvent(event Event) {
	select {
	case b.events <- event:
	case <-b.done:
	}
}

// Events returns the events channel
func (b *linuxBackend) Events() <-chan Event {
	return b.events
}

// Errors returns the errors channel
func (b *linuxBackend) Errors() <-chan error {
	return b.errors
}

// Stop stops the watcher
func (b *linuxBackend) Stop() error {
	close(b.done)

	// Wait for goroutines to finish
	b.wg.Wait()

	// Close inotify
	if b.fd >= 0 {
		unix.Close(b.fd)
	}

	close(b.events)
	close(b.errors)

	return nil
}

// clen returns the length of a null-terminated byte slice
func clen(n []byte) int {
	for i := 0; i < len(n); i++ {
		if n[i] == 0 {
			return i
		}
	}
	return len(n)
}

// newFallbackBackend is a stub that should never be called on Linux
// It exists only to satisfy the compiler when watcher.go references it
func newFallbackBackend(logger *slog.Logger, opts Options) (WatcherBackend, error) {
	return nil, fmt.Errorf("fallback backend not available on Linux")
}
