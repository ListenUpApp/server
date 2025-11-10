package watcher

import "time"

// EventType represents the type of file system event
type EventType int

const (
	// EventAdded is emitted when a new file is detected (after settling)
	EventAdded EventType = iota
	// EventModified is emitted when an existing file changes (after settling)
	EventModified
	// EventRemoved is emitted when a file is deleted
	EventRemoved
	// EventMoved is emitted when a file is moved (future enhancement)
	EventMoved
)

// String returns the string representation of the event type
func (t EventType) String() string {
	switch t {
	case EventAdded:
		return "added"
	case EventModified:
		return "modified"
	case EventRemoved:
		return "removed"
	case EventMoved:
		return "moved"
	default:
		return "unknown"
	}
}

// Event represents a file system event
type Event struct {
	// Type is the kind of event (added, modified, removed, moved)
	Type EventType

	// Path is the current file path
	Path string

	// OldPath is the previous path (only for move events)
	OldPath string

	// Inode is the file's inode number (for tracking file identity)
	Inode uint64

	// Size is the file size in bytes
	Size int64

	// ModTime is the file's last modification time
	ModTime time.Time
}
