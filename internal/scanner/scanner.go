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

	"github.com/listenupapp/listenup-server/internal/store"
)

// Scanner orchestrates the library scanning process
type Scanner struct {
	store  *store.Store
	logger *slog.Logger

	walker   *Walker
	grouper  *Grouper
	analyzer *Analyzer
	differ   *Differ
}

// NewScanner creates a new scanner instance
func NewScanner(store *store.Store, logger *slog.Logger) *Scanner {
	return &Scanner{
		store:    store,
		logger:   logger,
		walker:   NewWalker(logger),
		grouper:  NewGrouper(logger),
		analyzer: NewAnalyzer(logger),
		differ:   NewDiffer(logger),
	}
}

// ScanOptions configures a scan
type ScanOptions struct {
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
		StartedAt: time.Now(),
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

	// Phase 3: Analyze audio files
	tracker.SetPhase(PhaseAnalyzing)
	tracker.SetTotal(len(grouped))
	s.logger.Info("analyzing items", "count", len(grouped))

	for itemPath, itemFiles := range grouped {
		// Check context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Extract audio files from the group
		var audioFiles []AudioFileData
		for _, f := range itemFiles {
			// Check if it's an audio file by extension
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
			}
		}

		// Analyze audio files
		if len(audioFiles) > 0 {
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
				continue
			}

			// For now, just log the analyzed files
			// TODO: Build LibraryItemData and process further
			s.logger.Info("analyzed item", "path", itemPath, "audioFiles", len(analyzed))
		}

		tracker.Increment(itemPath)
	}

	s.logger.Info("analysis complete")

	// Phase 4: Resolve metadata (TODO: implement resolver)
	tracker.SetPhase(PhaseResolving)
	s.logger.Info("resolving metadata")
	// TODO: Implement metadata resolution from multiple sources

	// Phase 5: Diff (TODO: implement differ)
	tracker.SetPhase(PhaseDiffing)
	s.logger.Info("computing diff")
	// TODO: Implement diff computation

	// Phase 6: Apply changes (TODO: implement database updates)
	if !opts.DryRun {
		tracker.SetPhase(PhaseApplying)
		s.logger.Info("applying changes")
		// TODO: Implement database updates
	} else {
		s.logger.Info("dry run mode - skipping database updates")
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

	return result, nil
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
