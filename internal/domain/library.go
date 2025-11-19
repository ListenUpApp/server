package domain

import (
	"slices"
	"time"
)

// Library represents a physical audiobook collection rooted at a filesystem path.
// A library can scan multiple filesystem paths and presents them as a single.
// unified collection.
type Library struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ID        string    `json:"id"`
	OwnerID   string    `json:"owner_id"` // User who owns this library
	Name      string    `json:"name"`
	ScanPaths []string  `json:"scan_paths"`
	SkipInbox bool      `json:"skip_inbox"` // If true, new books bypass Inbox and are immediately public
}

// AddScanPath adds a path to the library's scan paths if not already present.
func (l *Library) AddScanPath(path string) {
	// Check for dupes.
	if slices.Contains(l.ScanPaths, path) {
		return
	}
	l.ScanPaths = append(l.ScanPaths, path)
}

// RemoveScanPath removes a path from the library's scan paths.
func (l *Library) RemoveScanPath(path string) {
	l.ScanPaths = slices.DeleteFunc(l.ScanPaths, func(existing string) bool {
		return existing == path
	})
}

