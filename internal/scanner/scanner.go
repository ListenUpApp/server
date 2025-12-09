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

	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Extension sets for file classification (package-level to avoid allocations).
var (
	audioExtensions = map[string]bool{
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

	imageExtensions = map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".webp": true,
		".gif":  true,
		".bmp":  true,
	}
)

// Scanner orchestrates the library scanning process.
type Scanner struct {
	store          *store.Store
	eventEmitter   store.EventEmitter
	logger         *slog.Logger
	imageProcessor *images.Processor

	walker   *Walker
	grouper  *Grouper
	analyzer *Analyzer
	differ   *Differ
}

// NewScanner creates a new scanner instance.
func NewScanner(store *store.Store, emitter store.EventEmitter, imageProcessor *images.Processor, logger *slog.Logger) *Scanner {
	return &Scanner{
		store:          store,
		eventEmitter:   emitter,
		logger:         logger,
		imageProcessor: imageProcessor,
		walker:         NewWalker(logger),
		grouper:        NewGrouper(logger),
		analyzer:       NewAnalyzer(logger),
		differ:         NewDiffer(logger),
	}
}

// ScanOptions configures a scan.
type ScanOptions struct {
	OnProgress func(*Progress)
	LibraryID  string
	Workers    int
	Force      bool
	DryRun     bool
}

// Scan performs a full library scan of the given folder path.
// It walks the filesystem, groups files into library items, analyzes audio metadata,
// and saves the results to the database (unless DryRun is set).
func (s *Scanner) Scan(ctx context.Context, folderPath string, opts ScanOptions) (*ScanResult, error) {
	// Verify path exists.
	if _, err := os.Stat(folderPath); err != nil {
		return nil, fmt.Errorf("folder path not accessible: %w", err)
	}

	// Initialize result and progress tracking.
	result := &ScanResult{
		LibraryID: opts.LibraryID,
		StartedAt: time.Now(),
	}

	if opts.LibraryID != "" {
		s.eventEmitter.Emit(sse.NewScanStartedEvent(opts.LibraryID))
	}

	tracker := NewProgressTracker(opts.OnProgress)
	defer tracker.Close() // Ensure cleanup of background goroutine

	// Get initial progress snapshot
	progress := tracker.Get()
	result.Progress = &progress

	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}

	// Execute scan phases.
	files := s.walkFilesystem(ctx, folderPath, tracker, result)

	grouped := s.groupFiles(ctx, files, tracker)

	items, err := s.buildLibraryItems(ctx, grouped, tracker, result, opts)
	if err != nil {
		return nil, err
	}

	if err := s.saveToDatabase(ctx, items, tracker, result, opts); err != nil {
		return nil, err
	}

	// Complete scan.
	result.CompletedAt = time.Now()
	tracker.SetPhase(PhaseComplete)
	s.logger.Info("scan complete",
		"duration", result.CompletedAt.Sub(result.StartedAt),
		"files", len(files),
		"items", len(grouped),
		"errors", result.Errors,
	)

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

// walkFilesystem walks the directory tree and collects all files.
func (s *Scanner) walkFilesystem(ctx context.Context, folderPath string, tracker *ProgressTracker, result *ScanResult) []WalkResult {
	tracker.SetPhase(PhaseWalking)
	s.logger.Info("starting walk", "path", folderPath)

	walkResults := s.walker.Walk(ctx, folderPath)
	files := make([]WalkResult, 0, 100)

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
	return files
}

// groupFiles groups files into library items (books).
func (s *Scanner) groupFiles(ctx context.Context, files []WalkResult, tracker *ProgressTracker) map[string][]WalkResult {
	tracker.SetPhase(PhaseGrouping)
	s.logger.Info("grouping files")

	grouped := s.grouper.Group(ctx, files, GroupOptions{})

	s.logger.Info("grouping complete", "items", len(grouped))
	return grouped
}

// buildLibraryItems builds LibraryItemData structures from grouped files.
func (s *Scanner) buildLibraryItems(ctx context.Context, grouped map[string][]WalkResult, tracker *ProgressTracker, result *ScanResult, opts ScanOptions) ([]*LibraryItemData, error) {
	tracker.SetPhase(PhaseAnalyzing)
	tracker.SetTotal(len(grouped))
	s.logger.Info("building library items", "count", len(grouped))

	items := make([]*LibraryItemData, 0, len(grouped))

	for itemPath, itemFiles := range grouped {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Classify files by type.
		audioFiles, imageFiles, metadataFiles := s.classifyFiles(itemFiles)

		// Skip items with no audio files.
		if len(audioFiles) == 0 {
			s.logger.Info("skipping item with no audio files", "path", itemPath)
			tracker.Increment(itemPath)
			continue
		}

		// Analyze audio files.
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
			analyzed = audioFiles // Continue with unanalyzed files.
		}

		// Build item.
		item := s.buildItemData(itemPath, itemFiles, analyzed, imageFiles, metadataFiles)
		items = append(items, item)
		tracker.Increment(itemPath)
	}

	s.logger.Info("library items built", "count", len(items))
	return items, nil
}

// classifyFiles separates files into audio, image, and metadata categories.
// Uses a single-pass algorithm for optimal performance.
func (s *Scanner) classifyFiles(itemFiles []WalkResult) ([]AudioFileData, []ImageFileData, []MetadataFileData) {
	// Preallocate with reasonable capacities based on expected ratios.
	audioFiles := make([]AudioFileData, 0, len(itemFiles)/3)
	imageFiles := make([]ImageFileData, 0, len(itemFiles)/10)
	metadataFiles := make([]MetadataFileData, 0, len(itemFiles)/10)

	for _, f := range itemFiles {
		ext := strings.ToLower(filepath.Ext(f.Path))

		// Check audio first (most common).
		if IsAudioExt(ext) {
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

		// Check image (second most common).
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

		// Check metadata (least common).
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

	return audioFiles, imageFiles, metadataFiles
}

// buildItemData constructs a LibraryItemData from classified files.
func (s *Scanner) buildItemData(itemPath string, itemFiles []WalkResult, audioFiles []AudioFileData, imageFiles []ImageFileData, metadataFiles []MetadataFileData) *LibraryItemData {
	item := &LibraryItemData{
		Path:          itemPath,
		AudioFiles:    audioFiles,
		ImageFiles:    imageFiles,
		MetadataFiles: metadataFiles,
	}

	// Build item-level BookMetadata from first audio file's metadata.
	if len(audioFiles) > 0 && audioFiles[0].Metadata != nil {
		item.Metadata = buildBookMetadata(audioFiles[0].Metadata)
	}

	// Determine if this is a file or directory.
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

	return item
}

// saveToDatabase converts library items to domain.Book and saves them.
func (s *Scanner) saveToDatabase(ctx context.Context, items []*LibraryItemData, tracker *ProgressTracker, result *ScanResult, opts ScanOptions) error {
	switch {
	case opts.DryRun:
		s.logger.Info("dry run mode - skipping database updates")
		return nil
	case len(items) == 0:
		s.logger.Info("no items to save")
		return nil
	}

	tracker.SetPhase(PhaseApplying)
	s.logger.Info("saving books to database", "count", len(items))

	batchWriter := s.store.NewBatchWriter(100)
	defer func() {
		if err := batchWriter.Flush(ctx); err != nil {
			s.logger.Error("failed to flush final batch", "error", err)
		}
	}()

	var errs []error

	for _, item := range items {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Convert to domain.Book.
		book, err := ConvertToBook(ctx, item, s.store)
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

		// Extract and process cover art.
		// Priority: 1) embedded artwork, 2) external cover image files
		if s.imageProcessor != nil {
			coverFound := false

			// Try embedded artwork first.
			if len(item.AudioFiles) > 0 {
				firstAudioFile := item.AudioFiles[0].Path
				hash, extractErr := s.imageProcessor.ExtractAndProcess(ctx, firstAudioFile, book.ID)
				if extractErr != nil {
					s.logger.Debug("no embedded cover extracted",
						"book_id", book.ID,
						"path", firstAudioFile,
						"error", extractErr,
					)
				} else if hash != "" {
					coverFound = true
				}
			}

			// Fallback to external cover image if no embedded artwork.
			if !coverFound && len(item.ImageFiles) > 0 {
				// Use the first image file (typically cover.jpg)
				coverPath := item.ImageFiles[0].Path
				if _, extractErr := s.imageProcessor.ProcessExternalCover(coverPath, book.ID); extractErr != nil {
					s.logger.Warn("failed to process external cover",
						"book_id", book.ID,
						"path", coverPath,
						"error", extractErr,
					)
				} else {
					s.logger.Debug("used external cover",
						"book_id", book.ID,
						"path", coverPath,
					)
				}
			}
		}

		// Save to database.
		if err := batchWriter.CreateBook(ctx, book); err != nil {
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

	if len(errs) > 0 {
		s.logger.Warn("scan completed with errors",
			"error_count", len(errs),
			"first_error", errs[0],
		)
	}

	return nil
}

// ExtractCoverArt extracts embedded cover art from an audio file and processes it.
// Returns the path to the processed cover image, or empty string if no cover found.
func (s *Scanner) ExtractCoverArt(ctx context.Context, audioFilePath, bookID string) (string, error) {
	if s.imageProcessor == nil {
		return "", nil // No image processor configured
	}

	coverPath, err := s.imageProcessor.ExtractAndProcess(ctx, audioFilePath, bookID)
	if err != nil {
		return "", fmt.Errorf("extract and process cover: %w", err)
	}

	return coverPath, nil
}

// ScanFolder scans a specific folder incrementally.
// Only scans the given folder (and disc subdirectories if present), not the entire library.
// Returns a LibraryItemData structure that can be used to create or update a library item.
func (s *Scanner) ScanFolder(ctx context.Context, folderPath string, opts ScanOptions) (*LibraryItemData, error) {
	// Verify path exists.
	if _, err := os.Stat(folderPath); err != nil {
		return nil, fmt.Errorf("folder path not accessible: %w", err)
	}

	// Default workers if not specified.
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}

	s.logger.Info("scanning folder", "path", folderPath)

	// Phase 1: Walk just this folder (non-recursive, but includes disc subdirs).
	walkResults := s.walker.WalkFolder(ctx, folderPath)

	files := make([]WalkResult, 0, 50) // Preallocate with reasonable buffer for single folder
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

	// Phase 2: For ScanFolder, all files belong to the same item (the folder we're scanning).
	// We don't need the complex grouper logic here since we're only scanning one folder.
	itemPath := folderPath
	itemFiles := files

	s.logger.Info("grouping complete", "path", itemPath, "files", len(itemFiles))

	// Phase 3: Extract and classify files.
	var audioFiles []AudioFileData
	var imageFiles []ImageFileData
	var metadataFiles []MetadataFileData

	for _, f := range itemFiles {
		ext := strings.ToLower(filepath.Ext(f.Path))

		// Check if it's an audio file.
		if IsAudioExt(ext) {
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

		// Check if it's an image file.
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

		// Check if it's a metadata file.
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

	// Phase 4: Analyze audio files.
	var analyzed []AudioFileData
	if len(audioFiles) > 0 {
		var err error
		analyzed, err = s.analyzer.Analyze(ctx, audioFiles, AnalyzeOptions{
			Workers: opts.Workers,
		})
		if err != nil {
			s.logger.Error("analysis failed", "path", itemPath, "error", err)
			// Continue with unanalyzed files rather than failing.
			analyzed = audioFiles
		}
	}

	s.logger.Info("analysis complete", "path", itemPath, "audioFiles", len(analyzed))

	// Build library item data.
	item := &LibraryItemData{
		Path:          itemPath,
		AudioFiles:    analyzed,
		ImageFiles:    imageFiles,
		MetadataFiles: metadataFiles,
	}

	// Build item-level BookMetadata from first audio file's metadata.
	// This ensures contributors are extracted properly in ConvertToBook().
	if len(analyzed) > 0 && analyzed[0].Metadata != nil {
		item.Metadata = buildBookMetadata(analyzed[0].Metadata)
	}

	// Determine if this is a file or directory.
	if len(itemFiles) == 1 && itemPath == itemFiles[0].Path {
		// Single file (e.g., single M4B in library root).
		item.IsFile = true
		item.Size = itemFiles[0].Size
		item.ModTime = time.UnixMilli(itemFiles[0].ModTime)
		item.Inode = itemFiles[0].Inode
	} else {
		// Directory - use directory stats.
		if stat, err := os.Stat(itemPath); err == nil {
			item.IsFile = false
			item.ModTime = stat.ModTime()
			// Note: Size for directories is not meaningful, leave as 0.
		}
	}

	return item, nil
}

// isImageExt checks if a file extension is for an image file.
func isImageExt(ext string) bool {
	return imageExtensions[ext]
}

// classifyMetadataFile determines the type of metadata file.
func classifyMetadataFile(path string) MetadataFileType {
	filename := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))

	// Check specific filenames first.
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

	// Check extensions.
	switch ext {
	case ".opf":
		return MetadataTypeOPF
	case ".nfo":
		return MetadataTypeNFO
	}

	return MetadataTypeUnknown
}

// IsAudioExt checks if a file extension is for an audio file.
func IsAudioExt(ext string) bool {
	return audioExtensions[ext]
}
