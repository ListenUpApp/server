package audio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	"github.com/listenupapp/listenup-server/pkg/audiometa/m4a"
	"github.com/listenupapp/listenup-server/pkg/audiometa/mp3"
)

type NativeParser struct{}

func NewNativeParser() *NativeParser {
	return &NativeParser{}
}

func (p *NativeParser) Parse(ctx context.Context, path string) (*Metadata, error) {
	// Auto-detect format and parse
	meta, err := parseAudioFile(path)
	if err != nil {
		return nil, err
	}

	return convertMetadata(meta), nil
}

func (p *NativeParser) ParseMultiFile(ctx context.Context, paths []string) (*Metadata, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files provided")
	}

	// Detect format from first file
	ext := strings.ToLower(filepath.Ext(paths[0]))

	var meta *audiometa.Metadata
	var err error

	switch ext {
	case ".mp3":
		meta, err = mp3.ParseMultiFile(paths)
	case ".m4b", ".m4a":
		// M4B/M4A files shouldn't be multi-file audiobooks
		// If multiple M4B files, just parse the first one
		meta, err = m4a.Parse(paths[0])
	default:
		return nil, fmt.Errorf("unsupported format for multi-file: %s", ext)
	}

	if err != nil {
		return nil, err
	}

	return convertMetadata(meta), nil
}

// parseAudioFile auto-detects format and parses a single audio file
func parseAudioFile(path string) (*audiometa.Metadata, error) {
	// Open file to detect format
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Detect format
	format, err := audiometa.DetectFormat(file, stat.Size(), path)
	if err != nil {
		return nil, err
	}

	// Parse based on format
	switch format {
	case audiometa.FormatM4B, audiometa.FormatM4A:
		return m4a.Parse(path)
	case audiometa.FormatMP3:
		return mp3.Parse(path)
	default:
		return nil, &audiometa.UnsupportedFormatError{
			Path:   path,
			Reason: fmt.Sprintf("unsupported format: %s", format),
		}
	}
}

// convertMetadata converts audiometa.Metadata to scanner's Metadata format
func convertMetadata(meta *audiometa.Metadata) *Metadata {
	result := &Metadata{
		Format:     meta.Format.String(),
		Duration:   meta.Duration,
		Bitrate:    meta.BitRate,
		SampleRate: meta.SampleRate,
		Channels:   meta.Channels,
		Codec:      meta.Codec,

		// Standard tags
		Title:       meta.Title,
		Album:       meta.Album,
		Artist:      meta.Artist,
		AlbumArtist: meta.Artist,
		Composer:    meta.Composer,
		Genre:       meta.Genre,
		Year:        meta.Year,
		Track:       meta.TrackNumber,
		TrackTotal:  meta.TrackTotal,
		Disc:        meta.DiscNumber,
		DiscTotal:   meta.DiscTotal,

		// Audiobook-specific
		Narrator:   meta.Narrator,
		Publisher:  meta.Publisher,
		Series:     meta.Series,
		SeriesPart: meta.SeriesPart,
		ISBN:       meta.ISBN,
		ASIN:       meta.ASIN,

		// Description from comment
		Description: meta.Comment,
	}

	// Convert chapters
	for _, ch := range meta.Chapters {
		result.Chapters = append(result.Chapters, Chapter{
			Index:     ch.Index,
			Title:     ch.Title,
			StartTime: ch.StartTime,
			EndTime:   ch.EndTime,
		})
	}

	return result
}
