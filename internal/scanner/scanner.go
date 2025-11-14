package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Scanner orchestrates the library scanning process
type Scanner struct {
	store        *store.Store
	eventEmitter store.EventEmitter
	logger       *slog.Logger

	walker   *Walker
	grouper  *Grouper
	analyzer *Analyzer
	differ   *Differ
}

// NewScanner creates a new scanner instance
func NewScanner(store *store.Store, emitter store.EventEmitter, logger *slog.Logger) *Scanner {
	return &Scanner{
		store:        store,
		eventEmitter: emitter,
		logger:       logger,
		walker:       NewWalker(logger),
		grouper:      NewGrouper(logger),
		analyzer:     NewAnalyzer(logger),
		differ:       NewDiffer(logger),
	}
}

// ScanOptions configures a scan
type ScanOptions struct {
	LibraryID  string // The library being scanned (for event emission)
	Force      bool
	DryRun     bool
	Workers    int
	OnProgress func(*Progress)
}

func (s *Scanner) Scan(ctx context.Context, folderPath string, opts ScanOptions) (*ScanResult, error) {
	// Verify path exists
	if _, err := os.Stat(folderPath); err != nil {
		return nil, fmt.Errorf("folder path not accessible: %w", err)
	}

	// Initialize result
	result := &ScanResult{
		LibraryID: opts.LibraryID,
		StartedAt: time.Now(),
	}

	// Emit scan started event if library ID is provided
	if opts.LibraryID != "" {
		s.eventEmitter.Emit(sse.NewScanStartedEvent(opts.LibraryID))
	}

	// Setup progress tracking
	tracker := NewProgressTracker(opts.OnProgress)
	result.Progress = &tracker.progress

	// Default workers if not specified
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}

	// Phase 1: Walk filesystem
	tracker.SetPhase(PhaseWalking)
	s.logger.Info("starting walk", "path", folderPath)

	walkResults := s.walker.Walk(ctx, folderPath)

	var files []WalkResult
	for wr := range walkResults {
		if wr.Error != nil {
			tracker.AddError(ScanError{
				Path:  wr.Path,
				Phase: PhaseWalking,
				Error: wr.Error,
				Time:  time.Now(),
			})
			result.Errors++
			continue
		}
		files = append(files, wr)
		tracker.Increment(wr.Path)
	}

	s.logger.Info("walk complete", "files", len(files))

	// Phase 2: Group files into library items
	tracker.SetPhase(PhaseGrouping)
	s.logger.Info("grouping files")

	grouped := s.grouper.Group(ctx, files, GroupOptions{})

	s.logger.Info("grouping complete", "items", len(grouped))

	// Phase 3: Build LibraryItemData for each item
	tracker.SetPhase(PhaseAnalyzing)
	tracker.SetTotal(len(grouped))
	s.logger.Info("building library items", "count", len(grouped))

	var items []*LibraryItemData
	for itemPath, itemFiles := range grouped {
		// Check context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Classify files by type
		var audioFiles []AudioFileData
		var imageFiles []ImageFileData
		var metadataFiles []MetadataFileData

		for _, f := range itemFiles {
			ext := strings.ToLower(filepath.Ext(f.Path))

			if isAudioExt(ext) {
				audioFiles = append(audioFiles, AudioFileData{
					Path:     f.Path,
					RelPath:  f.RelPath,
					Filename: filepath.Base(f.Path),
					Ext:      ext,
					Size:     f.Size,
					ModTime:  time.UnixMilli(f.ModTime),
					Inode:    f.Inode,
				})
			} else if isImageExt(ext) {
				imageFiles = append(imageFiles, ImageFileData{
					Path:     f.Path,
					RelPath:  f.RelPath,
					Filename: filepath.Base(f.Path),
					Ext:      ext,
					Size:     f.Size,
					ModTime:  time.UnixMilli(f.ModTime),
					Inode:    f.Inode,
				})
			} else if metadataType := classifyMetadataFile(f.Path); metadataType != MetadataTypeUnknown {
				metadataFiles = append(metadataFiles, MetadataFileData{
					Path:     f.Path,
					RelPath:  f.RelPath,
					Filename: filepath.Base(f.Path),
					Ext:      ext,
					Type:     metadataType,
					Size:     f.Size,
					ModTime:  time.UnixMilli(f.ModTime),
					Inode:    f.Inode,
				})
			}
		}

		// Skip items with no audio files
		if len(audioFiles) == 0 {
			s.logger.Info("skipping item with no audio files", "path", itemPath)
			tracker.Increment(itemPath)
			continue
		}

		// Analyze audio files
		analyzed, err := s.analyzer.Analyze(ctx, audioFiles, AnalyzeOptions{
			Workers: opts.Workers,
		})
		if err != nil {
			tracker.AddError(ScanError{
				Path:  itemPath,
				Phase: PhaseAnalyzing,
				Error: err,
				Time:  time.Now(),
			})
			result.Errors++
			// Continue with unanalyzed files rather than failing completely
			analyzed = audioFiles
		}

		// Build library item data
		item := &LibraryItemData{
			Path:          itemPath,
			AudioFiles:    analyzed,
			ImageFiles:    imageFiles,
			MetadataFiles: metadataFiles,
		}

		// Determine if this is a file or directory
		if len(itemFiles) == 1 && itemPath == itemFiles[0].Path {
			item.IsFile = true
			item.Size = itemFiles[0].Size
			item.ModTime = time.UnixMilli(itemFiles[0].ModTime)
			item.Inode = itemFiles[0].Inode
		} else {
			if stat, err := os.Stat(itemPath); err == nil {
				item.IsFile = false
				item.ModTime = stat.ModTime()
			}
		}

		items = append(items, item)
		tracker.Increment(itemPath)
	}

	s.logger.Info("library items built", "count", len(items))

	// Phase 4: Convert to domain.Book and save to database
	if !opts.DryRun && len(items) > 0 {
		tracker.SetPhase(PhaseApplying)
		s.logger.Info("saving books to database", "count", len(items))

		// Use batch writer for efficient bulk inserts
		batchWriter := s.store.NewBatchWriter(100)
		defer func() {
			if err := batchWriter.Flush(); err != nil {
				s.logger.Error("failed to flush final batch", "error", err)
			}
		}()

		var errs []error

		for _, item := range items {
			// Check context
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			// Convert to domain.Book
			book, err := ConvertToBook(item)
			if err != nil {
				convertErr := fmt.Errorf("convert %s: %w", item.Path, err)
				errs = append(errs, convertErr)
				tracker.AddError(ScanError{
					Path:  item.Path,
					Phase: PhaseApplying,
					Error: convertErr,
					Time:  time.Now(),
				})
				result.Errors++
				continue
			}

			// Try to create the book using batch writer
			err = batchWriter.CreateBook(ctx, book)
			if err != nil {
				createErr := fmt.Errorf("save %s (%s): %w", book.Title, book.Path, err)
				errs = append(errs, createErr)
				tracker.AddError(ScanError{
					Path:  item.Path,
					Phase: PhaseApplying,
					Error: createErr,
					Time:  time.Now(),
				})
				result.Errors++
				continue
			}

			result.Added++
		}

		// Log aggregated errors if any
		if len(errs) > 0 {
			s.logger.Warn("scan completed with errors",
				"error_count", len(errs),
				"first_error", errs[0],
			)
		}
	} else if opts.DryRun {
		s.logger.Info("dry run mode - skipping database updates")
	} else if len(items) == 0 {
		s.logger.Info("no items to save")
	}

	// Complete
	result.CompletedAt = time.Now()
	tracker.SetPhase(PhaseComplete)
	s.logger.Info("scan complete",
		"duration", result.CompletedAt.Sub(result.StartedAt),
		"files", len(files),
		"items", len(grouped),
		"errors", result.Errors,
	)

	// Emit scan complete event if library ID is provided
	if opts.LibraryID != "" {
		s.eventEmitter.Emit(sse.NewScanCompleteEvent(
			opts.LibraryID,
			result.Added,
			result.Updated,
			result.Removed,
		))
	}

	return result, nil
}

// ScanFolder scans a specific folder incrementally
// Only scans the given folder (and disc subdirectories if present), not the entire library
// Returns a LibraryItemData structure that can be used to create or update a library item
func (s *Scanner) ScanFolder(ctx context.Context, folderPath string, opts ScanOptions) (*LibraryItemData, error) {
	// Verify path exists
	if _, err := os.Stat(folderPath); err != nil {
		return nil, fmt.Errorf("folder path not accessible: %w", err)
	}

	// Default workers if not specified
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}

	s.logger.Info("scanning folder", "path", folderPath)

	// Phase 1: Walk just this folder (non-recursive, but includes disc subdirs)
	walkResults := s.walker.WalkFolder(ctx, folderPath)

	var files []WalkResult
	for wr := range walkResults {
		if wr.Error != nil {
			s.logger.Error("walk error", "path", wr.Path, "error", wr.Error)
			continue
		}
		files = append(files, wr)
	}

	if len(files) == 0 {
		s.logger.Info("no files found in folder", "path", folderPath)
		return &LibraryItemData{
			Path:       folderPath,
			AudioFiles: []AudioFileData{},
		}, nil
	}

	s.logger.Info("walk complete", "files", len(files))

	// Phase 2: For ScanFolder, all files belong to the same item (the folder we're scanning)
	// We don't need the complex grouper logic here since we're only scanning one folder
	itemPath := folderPath
	itemFiles := files

	s.logger.Info("grouping complete", "path", itemPath, "files", len(itemFiles))

	// Phase 3: Extract and classify files
	var audioFiles []AudioFileData
	var imageFiles []ImageFileData
	var metadataFiles []MetadataFileData

	for _, f := range itemFiles {
		ext := strings.ToLower(filepath.Ext(f.Path))

		// Check if it's an audio file
		if isAudioExt(ext) {
			audioFiles = append(audioFiles, AudioFileData{
				Path:     f.Path,
				RelPath:  f.RelPath,
				Filename: filepath.Base(f.Path),
				Ext:      ext,
				Size:     f.Size,
				ModTime:  time.UnixMilli(f.ModTime),
				Inode:    f.Inode,
			})
			continue
		}

		// Check if it's an image file
		if isImageExt(ext) {
			imageFiles = append(imageFiles, ImageFileData{
				Path:     f.Path,
				RelPath:  f.RelPath,
				Filename: filepath.Base(f.Path),
				Ext:      ext,
				Size:     f.Size,
				ModTime:  time.UnixMilli(f.ModTime),
				Inode:    f.Inode,
			})
			continue
		}

		// Check if it's a metadata file
		if metadataType := classifyMetadataFile(f.Path); metadataType != MetadataTypeUnknown {
			metadataFiles = append(metadataFiles, MetadataFileData{
				Path:     f.Path,
				RelPath:  f.RelPath,
				Filename: filepath.Base(f.Path),
				Ext:      ext,
				Type:     metadataType,
				Size:     f.Size,
				ModTime:  time.UnixMilli(f.ModTime),
				Inode:    f.Inode,
			})
		}
	}

	s.logger.Info("files classified",
		"audio", len(audioFiles),
		"images", len(imageFiles),
		"metadata", len(metadataFiles),
	)

	// Phase 4: Analyze audio files
	var analyzed []AudioFileData
	if len(audioFiles) > 0 {
		var err error
		analyzed, err = s.analyzer.Analyze(ctx, audioFiles, AnalyzeOptions{
			Workers: opts.Workers,
		})
		if err != nil {
			s.logger.Error("analysis failed", "path", itemPath, "error", err)
			// Continue with unanalyzed files rather than failing
			analyzed = audioFiles
		}
	}

	s.logger.Info("analysis complete", "path", itemPath, "audioFiles", len(analyzed))

	// Build library item data
	item := &LibraryItemData{
		Path:          itemPath,
		AudioFiles:    analyzed,
		ImageFiles:    imageFiles,
		MetadataFiles: metadataFiles,
	}

	// Determine if this is a file or directory
	if len(itemFiles) == 1 && itemPath == itemFiles[0].Path {
		// Single file (e.g., single M4B in library root)
		item.IsFile = true
		item.Size = itemFiles[0].Size
		item.ModTime = time.UnixMilli(itemFiles[0].ModTime)
		item.Inode = itemFiles[0].Inode
	} else {
		// Directory - use directory stats
		if stat, err := os.Stat(itemPath); err == nil {
			item.IsFile = false
			item.ModTime = stat.ModTime()
			// Note: Size for directories is not meaningful, leave as 0
		}
	}

	return item, nil
}

// isImageExt checks if a file extension is for an image file
func isImageExt(ext string) bool {
	imageExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".webp": true,
		".gif":  true,
		".bmp":  true,
	}
	return imageExts[ext]
}

// classifyMetadataFile determines the type of metadata file
func classifyMetadataFile(path string) MetadataFileType {
	filename := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))

	// Check specific filenames first
	switch filename {
	case "metadata.json":
		return MetadataTypeJSON
	case "metadata.abs":
		return MetadataTypeABS
	case "desc.txt", "description.txt":
		return MetadataTypeDesc
	case "reader.txt", "narrator.txt":
		return MetadataTypeReader
	}

	// Check extensions
	switch ext {
	case ".opf":
		return MetadataTypeOPF
	case ".nfo":
		return MetadataTypeNFO
	}

	return MetadataTypeUnknown
}

// IsAudioExt checks if a file extension is for an audio file
func IsAudioExt(ext string) bool {
	audioExts := map[string]bool{
		".mp3":  true,
		".m4a":  true,
		".m4b":  true,
		".flac": true,
		".ogg":  true,
		".opus": true,
		".aac":  true,
		".wma":  true,
		".wav":  true,
	}
	return audioExts[ext]
}

// isAudioExt is the internal version that calls the exported function
// Kept for backward compatibility with existing code
func isAudioExt(ext string) bool {
	return IsAudioExt(ext)
}
