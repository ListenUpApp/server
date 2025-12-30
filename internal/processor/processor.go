package processor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/watcher"
)

// BookStore defines the interface for book database operations needed by the event processor.
type BookStore interface {
	GetBookByPath(ctx context.Context, path string) (*domain.Book, error)
	CreateBook(ctx context.Context, book *domain.Book) error
	UpdateBook(ctx context.Context, book *domain.Book) error
	DeleteBook(ctx context.Context, id string) error
	BroadcastBookCreated(ctx context.Context, book *domain.Book) error
	GetOrCreateContributorByName(ctx context.Context, name string) (*domain.Contributor, error)
	GetOrCreateSeriesByName(ctx context.Context, name string) (*domain.Series, error)

	// Genre methods for normalization during scanning.
	GetGenreBySlug(ctx context.Context, slug string) (*domain.Genre, error)
	GetGenreAliasByRaw(ctx context.Context, raw string) (*domain.GenreAlias, error)
	TrackUnmappedGenre(ctx context.Context, raw string, bookID string) error
	AddBookGenre(ctx context.Context, bookID, genreID string) error

	// Inbox workflow methods.
	GetServerSettings(ctx context.Context) (*domain.ServerSettings, error)
	GetDefaultLibrary(ctx context.Context) (*domain.Library, error)
	GetInboxForLibrary(ctx context.Context, libraryID string) (*domain.Collection, error)
	AdminAddBookToCollection(ctx context.Context, bookID, collectionID string) error
}

// EventProcessor processes file system events and orchestrates incremental scanning.
//
// Life before death. Strength before weakness. Journey before destination.
// We protect the library through vigilance.
//
// Key design principles:
//   - Processes each event immediately (no batching).
//   - Uses per-folder locking to deduplicate concurrent events.
//   - All file types trigger a rescan of the affected folder (simple, idempotent).
//   - Non-blocking (TryLock prevents queueing).
type EventProcessor struct {
	scanner      *scanner.Scanner
	store        BookStore
	enricher     *dto.Enricher
	eventEmitter *sse.Manager
	logger       *slog.Logger

	// folderLocks provides per-folder mutexes to prevent concurrent scans.
	// of the same folder. Type-safe concurrent map using generics.
	folderLocks *SyncMap[string, *sync.Mutex]
}

// NewEventProcessor creates a new EventProcessor instance.
func NewEventProcessor(scanner *scanner.Scanner, store BookStore, enricher *dto.Enricher, eventEmitter *sse.Manager, logger *slog.Logger) *EventProcessor {
	return &EventProcessor{
		scanner:      scanner,
		store:        store,
		enricher:     enricher,
		eventEmitter: eventEmitter,
		logger:       logger,
		folderLocks:  NewSyncMap[string, *sync.Mutex](),
	}
}

// ProcessEvent processes a file system event.
//
// Processing flow:
//  1. Classify the file (audio, cover, metadata, or ignored).
//  2. Determine which folder (book) the file belongs to.
//  3. Acquire per-folder lock with TryLock (deduplicate concurrent events).
//  4. Call appropriate handler based on file type.
//
// If the folder is already being scanned, the event is skipped (non-blocking).
// The next event for that folder will catch any changes.
func (ep *EventProcessor) ProcessEvent(ctx context.Context, event watcher.Event) error {
	// Log the event.
	ep.logger.Debug("processing event",
		"type", event.Type.String(),
		"path", event.Path,
	)

	// Classify file type.
	fileType := classifyFile(event.Path)

	// Skip ignored files, but NOT for removal events.
	// Removal events might be for directories (book folders) which have no extension.
	if fileType == FileTypeIgnored && event.Type != watcher.EventRemoved {
		ep.logger.Debug("ignoring file",
			"path", event.Path,
			"type", fileType.String(),
		)
		return nil
	}

	// Determine which book folder this file belongs to.
	bookFolder := determineBookFolder(event.Path)

	// Acquire folder lock (deduplicate concurrent events).
	lock := ep.getFolderLock(bookFolder)
	if !lock.TryLock() {
		// Already scanning this folder, skip.
		ep.logger.Debug("folder already being scanned, skipping",
			"folder", bookFolder,
			"path", event.Path,
		)
		return nil
	}
	defer lock.Unlock()

	// Process based on event type and file type.
	switch event.Type {
	case watcher.EventAdded, watcher.EventModified, watcher.EventMoved:
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

	// Delegate to specific handlers based on file type.
	switch fileType {
	case FileTypeAudio:
		return ep.handleAudioFile(ctx, bookFolder, filePath)
	case FileTypeCover:
		return ep.handleCoverFile(ctx, bookFolder, filePath)
	case FileTypeMetadata:
		return ep.handleMetadataFile(ctx, bookFolder, filePath)
	case FileTypeIgnored:
		return nil // Ignored files are skipped
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

	// Scan the folder to get the current state.
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

	ep.logger.Info("scanned audio file folder",
		"folder", bookFolder,
		"audioFiles", len(item.AudioFiles),
		"imageFiles", len(item.ImageFiles),
		"metadataFiles", len(item.MetadataFiles),
	)

	// Check if book already exists at this path
	existingBook, err := ep.store.GetBookByPath(ctx, item.Path)
	if err != nil {
		// Book doesn't exist - create new one
		book, convertErr := scanner.ConvertToBook(ctx, item, ep.store)
		if convertErr != nil {
			ep.logger.Error("failed to convert scanned item to book",
				"folder", bookFolder,
				"error", convertErr,
			)
			return fmt.Errorf("convert to book: %w", convertErr)
		}

		if createErr := ep.store.CreateBook(ctx, book); createErr != nil {
			ep.logger.Error("failed to create book",
				"folder", bookFolder,
				"title", book.Title,
				"error", createErr,
			)
			return fmt.Errorf("create book: %w", createErr)
		}

		ep.logger.Info("created new book",
			"id", book.ID,
			"title", book.Title,
			"path", book.Path,
		)

		// Extract embedded cover art if present
		if len(item.AudioFiles) > 0 {
			firstAudioFile := item.AudioFiles[0].Path
			if coverInfo, extractErr := ep.scanner.ExtractCoverArt(ctx, firstAudioFile, book.ID); extractErr != nil {
				ep.logger.Warn("failed to extract embedded cover art",
					"book_id", book.ID,
					"path", firstAudioFile,
					"error", extractErr,
				)
			} else if coverInfo != nil {
				ep.logger.Info("extracted embedded cover art",
					"book_id", book.ID,
					"cover_hash", coverInfo.Hash,
				)

				// Update book with cover image info
				book.CoverImage = &domain.ImageFileInfo{
					Filename: "cover.jpg",
					Format:   coverInfo.Format,
					Size:     coverInfo.Size,
				}

				// Save updated book with cover info
				if updateErr := ep.store.UpdateBook(ctx, book); updateErr != nil {
					ep.logger.Warn("failed to update book with cover info",
						"book_id", book.ID,
						"error", updateErr,
					)
				}
			}
		}

		// Broadcast SSE event AFTER cover extraction to avoid race condition
		// where clients try to download covers before they're ready
		if broadcastErr := ep.store.BroadcastBookCreated(ctx, book); broadcastErr != nil {
			ep.logger.Warn("failed to broadcast book.created event",
				"book_id", book.ID,
				"error", broadcastErr,
			)
		}

		// Add to inbox if workflow is enabled
		ep.addBookToInboxIfEnabled(ctx, book)
	} else {
		// Book exists - update it with new scan data
		if updateErr := scanner.UpdateBookFromScan(ctx, existingBook, item, ep.store); updateErr != nil {
			ep.logger.Error("failed to update book from scan",
				"folder", bookFolder,
				"book_id", existingBook.ID,
				"error", updateErr,
			)
			return fmt.Errorf("update book: %w", updateErr)
		}

		if saveErr := ep.store.UpdateBook(ctx, existingBook); saveErr != nil {
			ep.logger.Error("failed to save updated book",
				"folder", bookFolder,
				"book_id", existingBook.ID,
				"error", saveErr,
			)
			return fmt.Errorf("save book: %w", saveErr)
		}

		ep.logger.Info("updated existing book",
			"id", existingBook.ID,
			"title", existingBook.Title,
			"path", existingBook.Path,
		)

		// Extract embedded cover art if present (in case cover changed)
		if len(item.AudioFiles) > 0 {
			firstAudioFile := item.AudioFiles[0].Path
			if coverInfo, extractErr := ep.scanner.ExtractCoverArt(ctx, firstAudioFile, existingBook.ID); extractErr != nil {
				ep.logger.Warn("failed to extract embedded cover art",
					"book_id", existingBook.ID,
					"path", firstAudioFile,
					"error", extractErr,
				)
			} else if coverInfo != nil {
				ep.logger.Info("extracted embedded cover art",
					"book_id", existingBook.ID,
					"cover_hash", coverInfo.Hash,
				)

				// Update book with cover image info
				existingBook.CoverImage = &domain.ImageFileInfo{
					Filename: "cover.jpg",
					Format:   coverInfo.Format,
					Size:     coverInfo.Size,
				}

				// Save updated book with cover info
				if updateErr := ep.store.UpdateBook(ctx, existingBook); updateErr != nil {
					ep.logger.Warn("failed to update book with cover info",
						"book_id", existingBook.ID,
						"error", updateErr,
					)
				}
			}
		}
	}

	return nil
}

// handleCoverFile handles added or modified cover art files.
// Scans the folder to update the book with the new cover.
func (ep *EventProcessor) handleCoverFile(ctx context.Context, bookFolder, filePath string) error {
	ep.logger.Info("handling cover file",
		"folder", bookFolder,
		"file", filePath,
	)

	// Scan the folder to get the current state (including the new cover).
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

	// For now, just log the results.
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

	// Scan the folder to get the current state (including the new metadata).
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

	// For now, just log the results.
	// TODO: Database integration - update book metadata
	ep.logger.Info("scanned metadata file folder",
		"folder", bookFolder,
		"audioFiles", len(item.AudioFiles),
		"imageFiles", len(item.ImageFiles),
		"metadataFiles", len(item.MetadataFiles),
	)

	return nil
}

// handleRemovedFile handles removed files or folders.
// For files: Rescans the folder to see what remains. If no audio files remain,
// the book is deleted from the database.
// For folders: If the folder was a book folder, deletes the book.
func (ep *EventProcessor) handleRemovedFile(ctx context.Context, bookFolder, filePath string, fileType FileType) error {
	ep.logger.Info("handling removed file/folder",
		"folder", bookFolder,
		"path", filePath,
		"type", fileType.String(),
	)

	// Check if the removed path itself was a book folder (folder deletion).
	// This happens when fileType is Ignored (no extension = directory).
	if fileType == FileTypeIgnored {
		// The path might be the book folder itself.
		existingBook, err := ep.store.GetBookByPath(ctx, filePath)
		if err == nil && existingBook != nil {
			// Found a book at this exact path - it was a book folder deletion.
			ep.logger.Info("book folder deleted, removing book",
				"path", filePath,
				"book_id", existingBook.ID,
				"title", existingBook.Title,
			)
			if deleteErr := ep.store.DeleteBook(ctx, existingBook.ID); deleteErr != nil {
				ep.logger.Error("failed to delete book",
					"book_id", existingBook.ID,
					"error", deleteErr,
				)
				return fmt.Errorf("delete book: %w", deleteErr)
			}
			return nil
		}
		// Not a book folder, ignore.
		ep.logger.Debug("removed path is not a known book folder",
			"path", filePath,
		)
		return nil
	}

	// Regular file removal - check if book folder still has audio files.
	existingBook, err := ep.store.GetBookByPath(ctx, bookFolder)
	if err != nil {
		ep.logger.Debug("no existing book found for folder",
			"folder", bookFolder,
			"error", err,
		)
		// No book exists for this folder, nothing to do.
		return nil
	}

	// Rescan folder to see what remains.
	item, err := ep.scanner.ScanFolder(ctx, bookFolder, scanner.ScanOptions{
		Workers: 0, // Use default (runtime.NumCPU)
	})
	if err != nil {
		// If folder doesn't exist anymore, delete the book.
		ep.logger.Info("folder no longer exists, deleting book",
			"folder", bookFolder,
			"book_id", existingBook.ID,
			"title", existingBook.Title,
		)
		if deleteErr := ep.store.DeleteBook(ctx, existingBook.ID); deleteErr != nil {
			ep.logger.Error("failed to delete book",
				"book_id", existingBook.ID,
				"error", deleteErr,
			)
			return fmt.Errorf("delete book: %w", deleteErr)
		}
		return nil
	}

	// Check if any audio files remain.
	if len(item.AudioFiles) == 0 {
		// No audio files left - delete the book.
		ep.logger.Info("no audio files remain, deleting book",
			"folder", bookFolder,
			"book_id", existingBook.ID,
			"title", existingBook.Title,
		)
		if err := ep.store.DeleteBook(ctx, existingBook.ID); err != nil {
			ep.logger.Error("failed to delete book",
				"book_id", existingBook.ID,
				"error", err,
			)
			return fmt.Errorf("delete book: %w", err)
		}
	} else {
		// Some files remain - update the book with new file list.
		ep.logger.Info("audio files remain after removal, updating book",
			"folder", bookFolder,
			"book_id", existingBook.ID,
			"audioFiles", len(item.AudioFiles),
		)
		// Trigger a re-process of the folder to update the book.
		return ep.handleFileChange(ctx, bookFolder, filePath, fileType)
	}

	return nil
}

// getFolderLock gets or creates a mutex for the given folder.
// This enables per-folder locking to prevent concurrent scans of the same folder.
func (ep *EventProcessor) getFolderLock(folderPath string) *sync.Mutex {
	// Try to load existing lock.
	if lock, ok := ep.folderLocks.Load(folderPath); ok {
		return lock
	}

	// Create new lock.
	newLock := &sync.Mutex{}

	// Store it (LoadOrStore handles race condition if multiple goroutines.
	// try to create a lock for the same folder simultaneously).
	actual, _ := ep.folderLocks.LoadOrStore(folderPath, newLock)

	return actual
}

// addBookToInboxIfEnabled adds a newly created book to the inbox if the workflow is enabled.
// This allows admins to review new books before they become visible to users.
func (ep *EventProcessor) addBookToInboxIfEnabled(ctx context.Context, book *domain.Book) {
	// Check if inbox workflow is enabled
	settings, err := ep.store.GetServerSettings(ctx)
	if err != nil || !settings.InboxEnabled {
		return
	}

	// Get the inbox collection for the default library
	library, err := ep.store.GetDefaultLibrary(ctx)
	if err != nil || library == nil {
		ep.logger.Warn("failed to get default library for inbox",
			"error", err,
		)
		return
	}

	inboxCollection, err := ep.store.GetInboxForLibrary(ctx, library.ID)
	if err != nil || inboxCollection == nil {
		ep.logger.Warn("failed to get inbox collection",
			"library_id", library.ID,
			"error", err,
		)
		return
	}

	// Add book to inbox collection
	if err := ep.store.AdminAddBookToCollection(ctx, book.ID, inboxCollection.ID); err != nil {
		ep.logger.Warn("failed to add book to inbox",
			"book_id", book.ID,
			"error", err,
		)
		return
	}

	ep.logger.Info("added book to inbox",
		"book_id", book.ID,
		"title", book.Title,
	)

	// Emit SSE event for admin clients
	if ep.enricher != nil && ep.eventEmitter != nil {
		enrichedBook, err := ep.enricher.EnrichBook(ctx, book)
		if err == nil {
			ep.eventEmitter.Emit(sse.NewInboxBookAddedEvent(enrichedBook))
		}
	}
}
