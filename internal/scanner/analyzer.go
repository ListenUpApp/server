package scanner

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/listenupapp/listenup-server/internal/scanner/audio"
)

// Analyzer analyzes audio files and extracts metadata
type Analyzer struct {
	logger *slog.Logger
	parser audio.Parser
}

// NewAnalyzer creates a new analyzer
func NewAnalyzer(logger *slog.Logger) *Analyzer {
	return &Analyzer{
		logger: logger,
		parser: audio.NewNativeParser(),
	}
}

// AnalyzeOptions configures analysis behavior
type AnalyzeOptions struct {
	// Number of concurrent workers
	Workers int

	// Skip files that haven't changed (based on modtime/size
	UseCache bool
}

func (a *Analyzer) Analyze(ctx context.Context, files []AudioFileData, opts AnalyzeOptions) ([]AudioFileData, error) {
	// Handle empty input
	if len(files) == 0 {
		return []AudioFileData{}, nil
	}

	// Default workers to runtime.NumCPU() if not specified
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Create job and result channels
	type job struct {
		file  AudioFileData
		index int
	}

	type result struct {
		file  AudioFileData
		index int
		err   error
	}

	jobs := make(chan job, len(files))
	results := make(chan result, len(files))

	// Start workers
	for range workers {
		go func() {
			for j := range jobs {
				// Check context cancellation
				select {
				case <-ctx.Done():
					results <- result{file: j.file, index: j.index, err: ctx.Err()}
					return
				default:
				}

				// Parse metadata
				metadata, err := a.parser.Parse(ctx, j.file.Path)
				if err != nil {
					a.logger.Error("failed to parse file", "path", j.file.Path, "error", err)
					// Continue without metadata rather than failing
					results <- result{file: j.file, index: j.index, err: err}
					continue
				}

				// Convert audio.Metadata to AudioMetadata
				j.file.Metadata = convertMetadata(metadata)
				results <- result{file: j.file, index: j.index}
			}
		}()
	}

	// Send jobs
	for i, file := range files {
		select {
		case jobs <- job{file: file, index: i}:
		case <-ctx.Done():
			close(jobs)
			return nil, ctx.Err()
		}
	}
	close(jobs)

	// Collect results
	parsedFiles := make([]AudioFileData, len(files))
	var firstErr error

	for range len(files) {
		select {
		case r := <-results:
			parsedFiles[r.index] = r.file
			if r.err != nil && firstErr == nil {
				// Check if it's a context error (should fail fast)
				if r.err == context.Canceled || r.err == context.DeadlineExceeded {
					return nil, r.err
				}
				// Otherwise just log and continue
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return parsedFiles, firstErr
}

// ItemType represents the type of library item
type ItemType int

const (
	ItemTypeSingleFile ItemType = iota // Single audio file (M4B or single MP3)
	ItemTypeMultiFile                  // Multiple files (MP3 album/audiobook)
)

// AnalyzeItems analyzes library items with multi-file classification
func (a *Analyzer) AnalyzeItems(ctx context.Context, items []LibraryItemData) ([]LibraryItemData, error) {
	if len(items) == 0 {
		return []LibraryItemData{}, nil
	}

	// Process each item
	results := make([]LibraryItemData, len(items))

	for i, item := range items {
		// Classify item
		itemType := classifyItem(item)

		// Analyze based on type
		switch itemType {
		case ItemTypeSingleFile:
			// Single file - parse it
			if len(item.AudioFiles) > 0 {
				metadata, parseErr := a.parser.Parse(ctx, item.AudioFiles[0].Path)
				if parseErr != nil {
					a.logger.Error("failed to parse single file",
						"path", item.AudioFiles[0].Path,
						"error", parseErr)
				} else {
					item.AudioFiles[0].Metadata = convertMetadata(metadata)
				}
			}

		case ItemTypeMultiFile:
			// Multiple files - aggregate them
			paths := make([]string, len(item.AudioFiles))
			for j, file := range item.AudioFiles {
				paths[j] = file.Path
			}

			metadata, parseErr := a.parser.ParseMultiFile(ctx, paths)
			if parseErr != nil {
				a.logger.Error("failed to parse multi-file item",
					"path", item.Path,
					"files", len(paths),
					"error", parseErr)
			} else {
				// Store aggregated metadata in first file
				// (The item-level BookMetadata will be built later)
				if len(item.AudioFiles) > 0 {
					item.AudioFiles[0].Metadata = convertMetadata(metadata)
				}
			}
		}

		results[i] = item

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	return results, nil
}

// classifyItem determines if an item is single-file or multi-file
func classifyItem(item LibraryItemData) ItemType {
	audioCount := len(item.AudioFiles)

	// No audio files or single file
	if audioCount <= 1 {
		return ItemTypeSingleFile
	}

	// Check file types
	hasMP3 := false
	hasM4B := false

	for _, file := range item.AudioFiles {
		ext := strings.ToLower(filepath.Ext(file.Path))
		switch ext {
		case ".mp3":
			hasMP3 = true
		case ".m4b", ".m4a":
			hasM4B = true
		}
	}

	// Multiple MP3 files = multi-file audiobook/album
	if hasMP3 && audioCount > 1 {
		return ItemTypeMultiFile
	}

	// Multiple M4B files = error condition, but treat as single file
	if hasM4B && audioCount > 1 {
		// Log warning?
		return ItemTypeSingleFile
	}

	return ItemTypeSingleFile
}

// convertMetadata converts audio.Metadata to AudioMetadata
func convertMetadata(src *audio.Metadata) *AudioMetadata {
	if src == nil {
		return nil
	}

	dst := &AudioMetadata{
		Format:      src.Format,
		Duration:    src.Duration,
		Bitrate:     src.Bitrate,
		SampleRate:  src.SampleRate,
		Channels:    src.Channels,
		Codec:       src.Codec,
		Title:       src.Title,
		Album:       src.Album,
		Artist:      src.Artist,
		AlbumArtist: src.AlbumArtist,
		Composer:    src.Composer,
		Genre:       src.Genre,
		Year:        src.Year,
		Track:       src.Track,
		TrackTotal:  src.TrackTotal,
		Disc:        src.Disc,
		DiscTotal:   src.DiscTotal,
		Narrator:    src.Narrator,
		Publisher:   src.Publisher,
		Description: src.Description,
		Subtitle:    src.Subtitle,
		Series:      src.Series,
		SeriesPart:  src.SeriesPart,
		ISBN:        src.ISBN,
		ASIN:        src.ASIN,
		Language:    src.Language,
		HasCover:    src.HasCover,
		CoverMIME:   src.CoverMIME,
	}

	// Convert chapters
	for _, ch := range src.Chapters {
		dst.Chapters = append(dst.Chapters, Chapter{
			ID:        ch.Index,
			Title:     ch.Title,
			StartTime: ch.StartTime,
			EndTime:   ch.EndTime,
		})
	}

	return dst
}
