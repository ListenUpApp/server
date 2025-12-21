// Package scanner provides functionality for discovering, analyzing, and cataloging audiobook files.
package scanner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/listenupapp/listenup-server/internal/normalize"
	"github.com/listenupapp/listenup-server/internal/scanner/audio"
)

// Analyzer analyzes audio files and extracts metadata.
type Analyzer struct {
	logger *slog.Logger
	parser audio.Parser
}

// NewAnalyzer creates a new analyzer.
func NewAnalyzer(logger *slog.Logger) *Analyzer {
	return &Analyzer{
		logger: logger,
		parser: audio.NewNativeParser(),
	}
}

// AnalyzeOptions configures analysis behavior.
type AnalyzeOptions struct {
	// Number of concurrent workers.
	Workers int

	// Skip files that haven't changed (based on modtime/size.
	UseCache bool
}

// Analyze analyzes audio files and extracts metadata concurrently.
func (a *Analyzer) Analyze(ctx context.Context, files []AudioFileData, opts AnalyzeOptions) ([]AudioFileData, error) {
	// Handle empty input.
	if len(files) == 0 {
		return []AudioFileData{}, nil
	}

	// Default workers to runtime.NumCPU() if not specified.
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Create job and result channels.
	type job struct {
		file  AudioFileData
		index int
	}

	type result struct {
		err   error
		file  AudioFileData
		index int
	}

	jobs := make(chan job, len(files))
	results := make(chan result, len(files))

	// Start workers.
	for range workers {
		go func() {
			for j := range jobs {
				// Check context cancellation.
				select {
				case <-ctx.Done():
					results <- result{file: j.file, index: j.index, err: ctx.Err()}
					return
				default:
				}

				// Parse metadata.
				metadata, err := a.parser.Parse(ctx, j.file.Path)
				if err != nil {
					a.logger.Error("failed to parse file", "path", j.file.Path, "error", err)
					// Continue without metadata rather than failing.
					results <- result{file: j.file, index: j.index, err: err}
					continue
				}

				// Convert audio.Metadata to AudioMetadata.
				j.file.Metadata = convertMetadata(metadata)
				results <- result{file: j.file, index: j.index}
			}
		}()
	}

	// Send jobs.
	for i, file := range files {
		select {
		case jobs <- job{file: file, index: i}:
		case <-ctx.Done():
			close(jobs)
			return nil, ctx.Err()
		}
	}
	close(jobs)

	// Collect results.
	parsedFiles := make([]AudioFileData, len(files))

	for range len(files) {
		select {
		case r := <-results:
			parsedFiles[r.index] = r.file
			// Check if it's a context error (should fail fast).
			if r.err != nil && (errors.Is(r.err, context.Canceled) || errors.Is(r.err, context.DeadlineExceeded)) {
				return nil, r.err
			}
			// Otherwise individual file errors are logged by workers and gracefully ignored.
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return parsedFiles, nil
}

// ItemType represents the type of library item.
type ItemType int

// ItemType constants define the different types of audiobook items.
const (
	// ItemTypeSingleFile represents a single audio file (M4B or single MP3).
	ItemTypeSingleFile ItemType = iota
	// ItemTypeMultiFile represents multiple files (MP3 album/audiobook).
	ItemTypeMultiFile
)

// AnalyzeItems analyzes library items with multi-file classification.
func (a *Analyzer) AnalyzeItems(ctx context.Context, items []LibraryItemData) ([]LibraryItemData, error) {
	if len(items) == 0 {
		return []LibraryItemData{}, nil
	}

	// Process each item.
	results := make([]LibraryItemData, len(items))

	for i := range items {
		item := &items[i]
		// Classify item.
		itemType := classifyItem(*item)

		// Analyze based on type.
		switch itemType {
		case ItemTypeSingleFile:
			// Single file - parse it.
			if len(item.AudioFiles) > 0 {
				metadata, parseErr := a.parser.Parse(ctx, item.AudioFiles[0].Path)
				if parseErr != nil {
					a.logger.Error("failed to parse single file",
						"path", item.AudioFiles[0].Path,
						"error", parseErr)
				} else {
					item.AudioFiles[0].Metadata = convertMetadata(metadata)
					// Build item-level BookMetadata from audio metadata.
					item.Metadata = buildBookMetadata(item.AudioFiles[0].Metadata)
				}
			}

		case ItemTypeMultiFile:
			// Multiple files - aggregate them.
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
			} else if len(item.AudioFiles) > 0 {
				// Store aggregated metadata in first file.
				item.AudioFiles[0].Metadata = convertMetadata(metadata)
				// Build item-level BookMetadata from aggregated audio metadata.
				item.Metadata = buildBookMetadata(item.AudioFiles[0].Metadata)
			}
		}

		results[i] = *item

		// Check for context cancellation.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	return results, nil
}

// classifyItem determines if an item is single-file or multi-file.
func classifyItem(item LibraryItemData) ItemType {
	audioCount := len(item.AudioFiles)

	// No audio files or single file.
	if audioCount <= 1 {
		return ItemTypeSingleFile
	}

	// Check file types.
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

	// Multiple MP3 files = multi-file audiobook/album.
	if hasMP3 && audioCount > 1 {
		return ItemTypeMultiFile
	}

	// Multiple M4B files = error condition, but treat as single file.
	if hasM4B && audioCount > 1 {
		// Log warning?
		return ItemTypeSingleFile
	}

	return ItemTypeSingleFile
}

// convertMetadata converts audio.Metadata to AudioMetadata.
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

	// Convert chapters.
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

// splitContributors splits a contributor string by semicolons.
// Handles patterns like "Homer; Emily Wilson - translator".
func splitContributors(input string) []string {
	var result []string
	parts := strings.Split(input, ";")
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// resolveBookTitle determines the correct book title based on format and available metadata.
//
// For M4B/M4A/MP4 files (single-file audiobooks):
//   - The Title tag contains the book title
//   - Chapter information is stored in chapter atoms, not the title
//
// For MP3 albums (multi-file audiobooks):
//   - The Title tag contains the track/chapter name
//   - The Album tag contains the book title
//
// Falls back to Album if Title is empty, or Title if Album is empty.
func resolveBookTitle(audioMeta *AudioMetadata) string {
	format := strings.ToLower(audioMeta.Format)

	// M4B, M4A, MP4 formats: prefer Title tag
	if format == "m4b" || format == "m4a" || format == "mp4" {
		if audioMeta.Title != "" {
			return audioMeta.Title
		}
		return audioMeta.Album
	}

	// MP3 and other formats: prefer Album tag (Title is track name)
	if audioMeta.Album != "" {
		return audioMeta.Album
	}
	return audioMeta.Title
}

// sanitizeString removes null bytes and other control characters that can cause
// issues in databases and JSON parsing. Some audio metadata parsers include
// null terminators in strings.
func sanitizeString(s string) string {
	return strings.Map(func(r rune) rune {
		if r == 0 { // null byte
			return -1 // drop it
		}
		return r
	}, s)
}

// parseAbridgedFromTitle detects "(Abridged)" or "(Unabridged)" in title text
// and returns the cleaned title along with the abridged status.
// Returns (cleanedTitle, isAbridged).
func parseAbridgedFromTitle(title string) (string, bool) {
	lower := strings.ToLower(title)

	// Check for explicit abridged indicator
	if strings.Contains(lower, "(abridged)") {
		// Remove the indicator (case-insensitive)
		cleaned := regexp.MustCompile(`(?i)\s*\(abridged\)\s*`).ReplaceAllString(title, " ")
		return strings.TrimSpace(cleaned), true
	}

	// Check for unabridged indicator (strip it, book is not abridged)
	if strings.Contains(lower, "(unabridged)") {
		cleaned := regexp.MustCompile(`(?i)\s*\(unabridged\)\s*`).ReplaceAllString(title, " ")
		return strings.TrimSpace(cleaned), false
	}

	// Also check without parentheses (some titles have "- Unabridged" or similar)
	if strings.Contains(lower, "unabridged") {
		cleaned := regexp.MustCompile(`(?i)\s*[-:]\s*unabridged\s*`).ReplaceAllString(title, " ")
		return strings.TrimSpace(cleaned), false
	}

	if strings.Contains(lower, "abridged") {
		cleaned := regexp.MustCompile(`(?i)\s*[-:]\s*abridged\s*`).ReplaceAllString(title, " ")
		return strings.TrimSpace(cleaned), true
	}

	// No indicator found - default to unabridged (most audiobooks are)
	return title, false
}

// buildBookMetadata converts AudioMetadata to BookMetadata.
// This aggregates audio file metadata into item-level metadata for book creation.
func buildBookMetadata(audioMeta *AudioMetadata) *BookMetadata {
	if audioMeta == nil {
		return nil
	}

	// Determine book title based on format.
	// For M4B/M4A files, the Title tag is the book title (chapters are in chapter atoms).
	// For MP3 albums, the Title tag is the track/chapter name, so use Album instead.
	rawTitle := sanitizeString(resolveBookTitle(audioMeta))

	// Parse abridged status from title (e.g., "Book Name (Unabridged)")
	// This also cleans the title by removing the indicator.
	title, abridged := parseAbridgedFromTitle(rawTitle)

	bookMeta := &BookMetadata{
		Title:       title,
		Subtitle:    sanitizeString(audioMeta.Subtitle),
		Description: htmlToMarkdown(sanitizeString(audioMeta.Description)),
		Publisher:   sanitizeString(audioMeta.Publisher),
		Language:    normalize.LanguageCode(audioMeta.Language),
		ISBN:        sanitizeString(audioMeta.ISBN),
		ASIN:        sanitizeString(audioMeta.ASIN),
		Abridged:    abridged,
		Chapters:    audioMeta.Chapters,
	}

	// Convert year to string.
	if audioMeta.Year > 0 {
		bookMeta.PublishYear = fmt.Sprintf("%d", audioMeta.Year)
	}

	// Convert Artist to Authors array (split by semicolon for multiple).
	if audioMeta.Artist != "" {
		bookMeta.Authors = splitContributors(sanitizeString(audioMeta.Artist))
	}

	// Convert Narrator to Narrators array (split by semicolon for multiple).
	if audioMeta.Narrator != "" {
		bookMeta.Narrators = splitContributors(sanitizeString(audioMeta.Narrator))
	}

	// Convert Genre to Genres array (split by semicolon or comma for multiple).
	// Audio files use various separators: "Fantasy; Science Fiction" or "Fantasy, Science Fiction"
	if audioMeta.Genre != "" {
		genreStr := sanitizeString(audioMeta.Genre)
		// Normalize separators: replace semicolons with commas, then split by comma
		genreStr = strings.ReplaceAll(genreStr, ";", ",")
		genres := strings.Split(genreStr, ",")
		for _, genre := range genres {
			if trimmed := strings.TrimSpace(genre); trimmed != "" {
				bookMeta.Genres = append(bookMeta.Genres, trimmed)
			}
		}
	}

	// Convert Series to SeriesInfo array.
	if audioMeta.Series != "" {
		bookMeta.Series = []SeriesInfo{
			{
				Name:     sanitizeString(audioMeta.Series),
				Sequence: sanitizeString(audioMeta.SeriesPart),
			},
		}
	}

	return bookMeta
}

// BuildBookMetadataExported is an exported version of buildBookMetadata for diagnostic tools.
func BuildBookMetadataExported(audioMeta *AudioMetadata) *BookMetadata {
	return buildBookMetadata(audioMeta)
}
