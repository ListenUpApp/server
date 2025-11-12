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

// ComputeDiff compares scanned items against existing items to determine what changed
func (d *Differ) ComputeDiff(ctx context.Context, scanned []LibraryItemData, existing []ExistingItem) (*ScanDiff, error) {
	diff := &ScanDiff{
		Added:   make([]LibraryItemData, 0),
		Updated: make([]ItemUpdate, 0),
		Removed: make([]string, 0),
	}

	// Create lookup maps for efficient matching
	existingByPath := make(map[string]*ExistingItem)
	existingByInode := make(map[uint64]*ExistingItem)
	matchedExisting := make(map[string]bool)

	for i := range existing {
		item := &existing[i]
		existingByPath[item.Path] = item
		if item.Inode > 0 {
			existingByInode[item.Inode] = item
		}
	}

	// Process scanned items
	for i := range scanned {
		scannedItem := &scanned[i]

		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var matchedItem *ExistingItem

		// 1. Try exact path match (most common case)
		if existing, ok := existingByPath[scannedItem.Path]; ok {
			matchedItem = existing
		}

		// 2. Try inode match (handles moves/renames)
		if matchedItem == nil && scannedItem.Inode > 0 {
			if existing, ok := existingByInode[scannedItem.Inode]; ok {
				matchedItem = existing
			}
		}

		if matchedItem != nil {
			// Mark as matched
			matchedExisting[matchedItem.ID] = true

			// Check if item has changed
			if d.hasChanged(scannedItem, matchedItem) {
				changes := d.buildFieldChanges(scannedItem, matchedItem)
				diff.Updated = append(diff.Updated, ItemUpdate{
					ID:      matchedItem.ID,
					Changes: changes,
				})
			}
			// else: unchanged, no action needed
		} else {
			// New item
			diff.Added = append(diff.Added, *scannedItem)
		}
	}

	// Find removed items (in existing but not scanned)
	for i := range existing {
		item := &existing[i]
		if !matchedExisting[item.ID] {
			diff.Removed = append(diff.Removed, item.ID)
		}
	}

	d.logger.Info("diff computed",
		"added", len(diff.Added),
		"updated", len(diff.Updated),
		"removed", len(diff.Removed),
	)

	return diff, nil
}

// hasChanged determines if a scanned item differs from an existing item
func (d *Differ) hasChanged(scanned *LibraryItemData, existing *ExistingItem) bool {
	// Check if path changed (file/folder moved)
	if scanned.Path != existing.Path {
		return true
	}

	// Check if modification time changed
	if !scanned.ModTime.Equal(existing.ModTime) {
		return true
	}

	// Check if number of audio files changed
	if len(scanned.AudioFiles) != existing.NumAudioFiles {
		return true
	}

	// Check if number of image files changed
	if len(scanned.ImageFiles) != existing.NumImageFiles {
		return true
	}

	return false
}

// buildFieldChanges creates a map of field-level changes
func (d *Differ) buildFieldChanges(scanned *LibraryItemData, existing *ExistingItem) map[string]FieldChange {
	changes := make(map[string]FieldChange)

	if scanned.Path != existing.Path {
		changes["path"] = FieldChange{
			Field:    "path",
			OldValue: existing.Path,
			NewValue: scanned.Path,
		}
	}

	if !scanned.ModTime.Equal(existing.ModTime) {
		changes["mod_time"] = FieldChange{
			Field:    "mod_time",
			OldValue: existing.ModTime,
			NewValue: scanned.ModTime,
		}
	}

	if len(scanned.AudioFiles) != existing.NumAudioFiles {
		changes["audio_files"] = FieldChange{
			Field:    "audio_files",
			OldValue: existing.NumAudioFiles,
			NewValue: len(scanned.AudioFiles),
		}
	}

	if len(scanned.ImageFiles) != existing.NumImageFiles {
		changes["image_files"] = FieldChange{
			Field:    "image_files",
			OldValue: existing.NumImageFiles,
			NewValue: len(scanned.ImageFiles),
		}
	}

	return changes
}

type ExistingItem struct {
	ID       string
	Path     string
	Inode    uint64
	ModTime  time.Time
	Metadata *BookMetadata
	NumAudioFiles int
	NumImageFiles int
}
