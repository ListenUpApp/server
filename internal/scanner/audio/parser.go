package audio

import (
	"context"
	"time"
)

// Parser parses audio file metadata
type Parser interface {

	// Parse extracts metadata from an audio file
	Parse(ctx context.Context, path string) (*Metadata, error)

	// ParseMultiFile aggregates metadata from multiple audio files
	// Used for multi-file audiobooks/albums (e.g., one MP3 per chapter)
	ParseMultiFile(ctx context.Context, paths []string) (*Metadata, error)
}

type Metadata struct {
	// Format info
	Format     string
	Duration   time.Duration
	Bitrate    int // bits per second
	SampleRate int // Hz
	Channels   int
	Codec      string

	// Standard tags
	Title       string
	Album       string
	Artist      string
	AlbumArtist string
	Composer    string // Often narrator
	Genre       string
	Year        int
	Track       int
	TrackTotal  int
	Disc        int
	DiscTotal   int

	// Extended metadata
	Narrator    string
	Publisher   string
	Description string
	Subtitle    string
	Series      string
	SeriesPart  string
	ISBN        string
	ASIN        string
	Language    string

	// Chapters
	Chapters []Chapter

	// Embedded artwork
	HasCover  bool
	CoverMIME string
}

// Chapter represents a chapter marker
type Chapter struct {
	Index     int
	Title     string
	StartTime time.Duration
	EndTime   time.Duration
}
