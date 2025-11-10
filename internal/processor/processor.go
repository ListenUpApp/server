package processor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/watcher"
)

// EventProcessor processes file system events and orchestrates incremental scanning.
//
// Key design principles:
//   - Processes each event immediately (no batching)
//   - Uses per-folder locking to deduplicate concurrent events
//   - All file types trigger a rescan of the affected folder (simple, idempotent)
//   - Non-blocking (TryLock prevents queueing)
type EventProcessor struct {
	scanner *scanner.Scanner
	logger  *slog.Logger

	// folderLocks provides per-folder mutexes to prevent concurrent scans
	// of the same folder. Type-safe concurrent map using generics.
	folderLocks *SyncMap[string, *sync.Mutex]
}

// NewEventProcessor creates a new EventProcessor instance
func NewEventProcessor(scanner *scanner.Scanner, logger *slog.Logger) *EventProcessor {
	return &EventProcessor{
		scanner:     scanner,
		logger:      logger,
		folderLocks: NewSyncMap[string, *sync.Mutex](),
	}
}

// ProcessEvent processes a file system event.
//
// Processing flow:
//  1. Classify the file (audio, cover, metadata, or ignored)
//  2. Determine which folder (book) the file belongs to
//  3. Acquire per-folder lock with TryLock (deduplicate concurrent events)
//  4. Call appropriate handler based on file type
//
// If the folder is already being scanned, the event is skipped (non-blocking).
// The next event for that folder will catch any changes.
func (ep *EventProcessor) ProcessEvent(ctx context.Context, event watcher.Event) error {
	// Log the event
	ep.logger.Debug("processing event",
		"type", event.Type.String(),
		"path", event.Path,
	)

	// Classify file type
	fileType := classifyFile(event.Path)

	// Skip ignored files
	if fileType == FileTypeIgnored {
		ep.logger.Debug("ignoring file",
			"path", event.Path,
			"type", fileType.String(),
		)
		return nil
	}

	// Determine which book folder this file belongs to
	bookFolder := determineBookFolder(event.Path)

	// Acquire folder lock (deduplicate concurrent events)
	lock := ep.getFolderLock(bookFolder)
	if !lock.TryLock() {
		// Already scanning this folder, skip
		ep.logger.Debug("folder already being scanned, skipping",
			"folder", bookFolder,
			"path", event.Path,
		)
		return nil
	}
	defer lock.Unlock()

	// Process based on event type and file type
	switch event.Type {
	case watcher.EventAdded, watcher.EventModified:
		return ep.handleFileChange(ctx, bookFolder, event.Path, fileType)
	case watcher.EventRemoved:
		return ep.handleRemovedFile(ctx, bookFolder, event.Path, fileType)
	default:
		ep.logger.Warn("unknown event type",
			"type", event.Type,
			"path", event.Path,
		)
		return nil
	}
}

// handleFileChange handles added or modified files.
// All file types trigger a rescan of the folder (simple, idempotent approach).
func (ep *EventProcessor) handleFileChange(ctx context.Context, bookFolder, filePath string, fileType FileType) error {
	ep.logger.Info("processing file change",
		"folder", bookFolder,
		"path", filePath,
		"type", fileType.String(),
	)

	// Delegate to specific handlers based on file type
	switch fileType {
	case FileTypeAudio:
		return ep.handleAudioFile(ctx, bookFolder, filePath)
	case FileTypeCover:
		return ep.handleCoverFile(ctx, bookFolder, filePath)
	case FileTypeMetadata:
		return ep.handleMetadataFile(ctx, bookFolder, filePath)
	default:
		return nil
	}
}

// handleAudioFile handles added or modified audio files.
// Scans the folder to discover or update the book.
func (ep *EventProcessor) handleAudioFile(ctx context.Context, bookFolder, filePath string) error {
	ep.logger.Info("handling audio file",
		"folder", bookFolder,
		"file", filePath,
	)

	// Scan the folder to get the current state
	item, err := ep.scanner.ScanFolder(ctx, bookFolder, scanner.ScanOptions{
		Workers: 0, // Use default (runtime.NumCPU)
	})
	if err != nil {
		ep.logger.Error("failed to scan folder",
			"folder", bookFolder,
			"error", err,
		)
		return fmt.Errorf("scan folder: %w", err)
	}

	// For now, just log the results
	// TODO: Database integration - create or update book
	ep.logger.Info("scanned audio file folder",
		"folder", bookFolder,
		"audioFiles", len(item.AudioFiles),
		"imageFiles", len(item.ImageFiles),
		"metadataFiles", len(item.MetadataFiles),
	)

	return nil
}

// handleCoverFile handles added or modified cover art files.
// Scans the folder to update the book with the new cover.
func (ep *EventProcessor) handleCoverFile(ctx context.Context, bookFolder, filePath string) error {
	ep.logger.Info("handling cover file",
		"folder", bookFolder,
		"file", filePath,
	)

	// Scan the folder to get the current state (including the new cover)
	item, err := ep.scanner.ScanFolder(ctx, bookFolder, scanner.ScanOptions{
		Workers: 0, // Use default (runtime.NumCPU)
	})
	if err != nil {
		ep.logger.Error("failed to scan folder",
			"folder", bookFolder,
			"error", err,
		)
		return fmt.Errorf("scan folder: %w", err)
	}

	// For now, just log the results
	// TODO: Database integration - update book cover
	ep.logger.Info("scanned cover file folder",
		"folder", bookFolder,
		"audioFiles", len(item.AudioFiles),
		"imageFiles", len(item.ImageFiles),
		"metadataFiles", len(item.MetadataFiles),
	)

	return nil
}

// handleMetadataFile handles added or modified metadata files.
// Scans the folder to update the book with the new metadata.
func (ep *EventProcessor) handleMetadataFile(ctx context.Context, bookFolder, filePath string) error {
	ep.logger.Info("handling metadata file",
		"folder", bookFolder,
		"file", filePath,
	)

	// Scan the folder to get the current state (including the new metadata)
	item, err := ep.scanner.ScanFolder(ctx, bookFolder, scanner.ScanOptions{
		Workers: 0, // Use default (runtime.NumCPU)
	})
	if err != nil {
		ep.logger.Error("failed to scan folder",
			"folder", bookFolder,
			"error", err,
		)
		return fmt.Errorf("scan folder: %w", err)
	}

	// For now, just log the results
	// TODO: Database integration - update book metadata
	ep.logger.Info("scanned metadata file folder",
		"folder", bookFolder,
		"audioFiles", len(item.AudioFiles),
		"imageFiles", len(item.ImageFiles),
		"metadataFiles", len(item.MetadataFiles),
	)

	return nil
}

// handleRemovedFile handles removed files.
// Rescans the folder to see what remains. If no audio files remain,
// the book should be marked as missing.
func (ep *EventProcessor) handleRemovedFile(ctx context.Context, bookFolder, filePath string, fileType FileType) error {
	ep.logger.Info("handling removed file",
		"folder", bookFolder,
		"file", filePath,
		"type", fileType.String(),
	)

	// Rescan folder to see what remains
	item, err := ep.scanner.ScanFolder(ctx, bookFolder, scanner.ScanOptions{
		Workers: 0, // Use default (runtime.NumCPU)
	})
	if err != nil {
		ep.logger.Error("failed to scan folder after removal",
			"folder", bookFolder,
			"error", err,
		)
		return fmt.Errorf("scan folder: %w", err)
	}

	// Check if any audio files remain
	if len(item.AudioFiles) == 0 {
		// No audio files left - book should be marked as missing
		// For now, just log
		// TODO: Database integration - mark book as missing
		ep.logger.Info("no audio files remain, book should be marked missing",
			"folder", bookFolder,
		)
	} else {
		// Some files remain - update book
		// For now, just log
		// TODO: Database integration - update book
		ep.logger.Info("audio files remain after removal",
			"folder", bookFolder,
			"audioFiles", len(item.AudioFiles),
			"imageFiles", len(item.ImageFiles),
			"metadataFiles", len(item.MetadataFiles),
		)
	}

	return nil
}

// getFolderLock gets or creates a mutex for the given folder.
// This enables per-folder locking to prevent concurrent scans of the same folder.
func (ep *EventProcessor) getFolderLock(folderPath string) *sync.Mutex {
	// Try to load existing lock
	if lock, ok := ep.folderLocks.Load(folderPath); ok {
		return lock
	}

	// Create new lock
	newLock := &sync.Mutex{}

	// Store it (LoadOrStore handles race condition if multiple goroutines
	// try to create a lock for the same folder simultaneously)
	actual, _ := ep.folderLocks.LoadOrStore(folderPath, newLock)

	return actual
}
