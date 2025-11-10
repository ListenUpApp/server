package watcher

import "context"

// WatcherBackend defines the platform-specific file watching implementation
type WatcherBackend interface {
	// Watch adds a path to be monitored. The path can be a file or directory.
	// If the path is a directory, it will be watched recursively.
	Watch(path string) error

	// Start begins watching for events. This method should block until
	// Stop is called or an error occurs.
	Start(ctx context.Context) error

	// Stop stops the watcher and releases all resources
	Stop() error

	// Events returns the channel for receiving file system events
	Events() <-chan Event

	// Errors returns the channel for receiving errors
	Errors() <-chan error
}
