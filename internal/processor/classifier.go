// Package processor handles file system event processing and incremental audiobook library scanning.
package processor

import (
	"path/filepath"
	"strings"

	"github.com/listenupapp/listenup-server/internal/scanner"
)

// FileType represents the type of file detected by the classifier.
type FileType int

const (
	// FileTypeAudio represents audio files (.mp3, .m4b, .m4a, .flac, .opus, .ogg, .aac, .wma, .wav).
	FileTypeAudio FileType = iota
	// FileTypeCover represents cover art files (.jpg, .jpeg, .png, .webp).
	FileTypeCover
	// FileTypeMetadata represents metadata files (.nfo, .txt, .json).
	FileTypeMetadata
	// FileTypeIgnored represents files that should be ignored (.cue, .log, temp files, hidden files).
	FileTypeIgnored
)

// String returns the string representation of a FileType.
func (ft FileType) String() string {
	switch ft {
	case FileTypeAudio:
		return "audio"
	case FileTypeCover:
		return "cover"
	case FileTypeMetadata:
		return "metadata"
	case FileTypeIgnored:
		return "ignored"
	default:
		return "unknown"
	}
}

// classifyFile determines the type of file based on its extension.
// This is a fast, deterministic, extension-based classification that.
// supports the event processor's need for instant file categorization.
//
// Classification rules:
//   - Audio: .mp3, .m4b, .m4a, .flac, .opus, .ogg, .aac, .wma, .wav.
//   - Cover: .jpg, .jpeg, .png, .webp.
//   - Metadata: .nfo, .txt, .json.
//   - Ignored: Everything else (including .cue, .log, temp files, hidden files).
//
// The function is case-insensitive and handles paths with various separators.
func classifyFile(path string) FileType {
	// Handle empty path.
	if path == "" {
		return FileTypeIgnored
	}

	// Extract extension and normalize to lowercase.
	ext := strings.ToLower(filepath.Ext(path))

	// Check audio files using scanner's exported function.
	if scanner.IsAudioExt(ext) {
		return FileTypeAudio
	}

	// Check cover art files.
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return FileTypeCover
	}

	// Check metadata files.
	switch ext {
	case ".nfo", ".txt", ".json":
		return FileTypeMetadata
	}

	// Everything else is ignored.
	return FileTypeIgnored
}
