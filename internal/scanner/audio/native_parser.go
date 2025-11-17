package audio

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/metadata"
	"github.com/simonhull/audiometa"
)

// NativeParser parses audio file metadata using the native audiometa library.
type NativeParser struct{}

// NewNativeParser creates a new NativeParser instance.
func NewNativeParser() *NativeParser {
	return &NativeParser{}
}

// Parse extracts metadata from a single audio file.
func (p *NativeParser) Parse(ctx context.Context, path string) (*Metadata, error) {
	// Open and parse audio file.
	file, err := audiometa.OpenContext(ctx, path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // Defer close, nothing we can do about errors here

	return convertMetadata(file), nil
}

// ParseMultiFile extracts combined metadata from multiple audio files.
func (p *NativeParser) ParseMultiFile(ctx context.Context, paths []string) (*Metadata, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files provided")
	}

	// Detect format from first file.
	ext := strings.ToLower(filepath.Ext(paths[0]))

	switch ext {
	case ".mp3":
		// For MP3, open all files and combine metadata.
		files, err := audiometa.OpenMany(ctx, paths...)
		if err != nil {
			return nil, err
		}
		defer func() {
			for _, f := range files {
				_ = f.Close() //nolint:errcheck // Defer close in cleanup, nothing we can do about errors
			}
		}()

		// Use metadata from first file, but sum durations.
		if len(files) == 0 {
			return nil, fmt.Errorf("no files opened")
		}

		result := convertMetadata(files[0])

		// Create chapters from each file and sum up total duration.
		var totalDuration time.Duration
		result.Chapters = make([]Chapter, 0, len(files))

		for i, f := range files {
			startTime := totalDuration
			totalDuration += f.Audio.Duration

			// Use the file's title if available, otherwise use filename.
			title := f.Tags.Title
			if title == "" {
				title = filepath.Base(paths[i])
			}

			result.Chapters = append(result.Chapters, Chapter{
				Index:     i + 1,
				Title:     title,
				StartTime: startTime,
				EndTime:   totalDuration,
			})
		}

		result.Duration = totalDuration

		return result, nil

	case ".m4b", ".m4a":
		// M4B/M4A files shouldn't be multi-file audiobooks.
		// If multiple M4B files, just parse the first one.
		file, err := audiometa.OpenContext(ctx, paths[0])
		if err != nil {
			return nil, err
		}
		defer file.Close() //nolint:errcheck // Defer close, nothing we can do about errors here

		return convertMetadata(file), nil

	default:
		return nil, fmt.Errorf("unsupported format for multi-file: %s", ext)
	}
}

// convertMetadata converts audiometa.File to scanner's Metadata format.
func convertMetadata(file *audiometa.File) *Metadata {
	result := &Metadata{
		Format:     file.Format.String(),
		Duration:   file.Audio.Duration,
		Bitrate:    file.Audio.Bitrate,
		SampleRate: file.Audio.SampleRate,
		Channels:   file.Audio.Channels,
		Codec:      file.Audio.Codec,

		// Standard tags.
		Title:  file.Tags.Title,
		Album:  file.Tags.Album,
		Artist: file.Tags.Artist,
		// Use AlbumArtist if set, otherwise fall back to Artist.
		AlbumArtist: file.Tags.AlbumArtist,
		// For multi-value fields, join with comma or use first value.
		Composer:   joinOrFirst(file.Tags.Composers),
		Genre:      joinOrFirst(file.Tags.Genres),
		Year:       file.Tags.Year,
		Track:      file.Tags.TrackNumber,
		TrackTotal: file.Tags.TrackTotal,
		Disc:       file.Tags.DiscNumber,
		DiscTotal:  file.Tags.DiscTotal,

		// Extended metadata.
		Narrator:   file.Tags.Narrator,
		Publisher:  file.Tags.Publisher,
		Series:     file.Tags.Series,
		SeriesPart: metadata.InferSeriesPosition(file),
		ISBN:       file.Tags.ISBN,
		ASIN:       file.Tags.ASIN,

		// Description from comment.
		Description: file.Tags.Comment,
	}

	// Fall back to Artist if AlbumArtist is not set.
	if result.AlbumArtist == "" {
		result.AlbumArtist = file.Tags.Artist
	}

	// Convert chapters.
	for _, ch := range file.Chapters {
		result.Chapters = append(result.Chapters, Chapter{
			Index:     ch.Index,
			Title:     ch.Title,
			StartTime: ch.StartTime,
			EndTime:   ch.EndTime,
		})
	}

	return result
}

// joinOrFirst joins a string slice with ", " or returns the first element if only one.
// Returns empty string if slice is empty.
func joinOrFirst(values []string) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return values[0]
	}
	return strings.Join(values, ", ")
}
