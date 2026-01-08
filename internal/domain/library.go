package domain

import (
	"slices"
	"time"
)

// AccessMode determines the default visibility of books in a library.
type AccessMode string

const (
	// AccessModeOpen means uncollected books are visible to all users.
	// Collections act as RESTRICTION zones - adding a book to a collection
	// limits its visibility to collection members only.
	// This is the default mode and matches existing behavior.
	AccessModeOpen AccessMode = "open"

	// AccessModeRestricted means users only see books they're explicitly granted.
	// Collections act as GRANT zones - users must have access to at least one
	// collection containing a book to see it.
	AccessModeRestricted AccessMode = "restricted"
)

// Library represents a physical audiobook collection rooted at a filesystem path.
// A library can scan multiple filesystem paths and presents them as a single.
// unified collection.
type Library struct {
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ID         string     `json:"id"`
	OwnerID    string     `json:"owner_id"` // User who owns this library
	Name       string     `json:"name"`
	ScanPaths  []string   `json:"scan_paths"`
	SkipInbox  bool       `json:"skip_inbox"`  // If true, new books bypass Inbox and are immediately public
	AccessMode AccessMode `json:"access_mode"` // Empty = "open" for backward compat
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

// GetAccessMode returns the effective access mode, defaulting to open.
// This ensures backward compatibility - existing libraries without an
// explicit access_mode behave as open.
func (l *Library) GetAccessMode() AccessMode {
	if l.AccessMode == "" {
		return AccessModeOpen
	}
	return l.AccessMode
}

// IsOpen returns true if the library uses open access mode.
func (l *Library) IsOpen() bool {
	return l.GetAccessMode() == AccessModeOpen
}

// IsRestricted returns true if the library uses restricted access mode.
func (l *Library) IsRestricted() bool {
	return l.GetAccessMode() == AccessModeRestricted
}
