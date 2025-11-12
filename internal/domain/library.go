package domain

import (
	"slices"
	"time"
)

// Library represents a physical audiobook collection rooted at a filesystem path.
// A library can scan multiple filesystem paths and presents them as a single
// unified collection
type Library struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	ScanPaths []string `json:"scan_paths"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (l *Library) AddScanPath(path string) {
	// Check for dupes
	if slices.Contains(l.ScanPaths, path) {
		return
	}
	l.ScanPaths = append(l.ScanPaths, path)
}

func (l *Library) RemoveScanPath(path string) {
	l.ScanPaths = slices.DeleteFunc(l.ScanPaths, func(existing string) bool {
		return existing == path
	})
}

// Collection represents a logical grouping of books within a library.
// Primarily, collections define access control but can also be used for organization.
type Collection struct {
	ID        string         `json:"id"`
	LibraryID string         `json:"library_id"`
	Name      string         `json:"name"`
	Type      CollectionType `json:"type"`

	BookIDs []string `json:"book_ids"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CollectionType defines special behavior for certain collections
type CollectionType int

const (
	// CollectionTypeDefault is the main collection where all approved books live.
	// Every library has exactly one of these.
	CollectionTypeDefault CollectionType = iota

	// CollectionTypeInbox is an optional (on by default) special collection that defines a staging area for books.
	// Books land here first which allows a user to review the book (and especially who can access it) before it
	// gets pushed out to the users of the application. Just like the default collection, we only have one of these.
	CollectionTypeInbox

	// CollectionTypeCustom is a catch-all for user created collections. Designed for organization and ACL
	// Some examples would be: "Kids Books", "Wednesday Book Club", "John's Smutty Monster Romance Books" (I see you John!), etc.
	// Libraries can have zero or many custom collections.
	CollectionTypeCustom
)

func (ct CollectionType) String() string {
	switch ct {
	case CollectionTypeDefault:
		return "default"
	case CollectionTypeInbox:
		return "inbox"
	case CollectionTypeCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// IsSystemCollection returns true for inbox and default collections.
// System collections are automatically created and cannot be deleted.
func (ct CollectionType) IsSystemCollection() bool {
	return ct == CollectionTypeDefault || ct == CollectionTypeInbox
}
