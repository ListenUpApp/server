package scanner

import (
	"log/slog"
	"time"

	"context"
)

type Differ struct {
	logger *slog.Logger
}

func NewDiffer(logger *slog.Logger) *Differ {
	return &Differ{
		logger: logger,
	}
}

func (d *Differ) ComputeDiff(ctx context.Context, scanned []LibraryItemData, existing []ExistingItem) (*ScanDiff, error) {
	// TODO: implement
	//
	// Algorithm:
	// 1. Match scanned items to existing items by:
	//    - Exact path match (preferred)
	//    - Inode match (handles moves)
	//    - Fuzzy match on title/author (handles renames)
	//
	// 2. For matched items, compute field-level changes
	//    - Only include fields that actually changed
	//    - Track what changed (old -> new value)
	//
	// 3. Items in scanned but not existing -> Added
	// 4. Items in existing but not scanned -> Removed
	// 5. Items matched with changes -> Updated

	return nil, nil
}

type ExistingItem struct {
	ID       string
	Path     string
	Inode    uint64
	ModTime  time.Time
	Metadata *ItemMetadata
}
