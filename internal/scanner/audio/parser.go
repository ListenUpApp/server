package audio

import (
	"context"
	"time"
)

// Parser parses audio file metadata.
type Parser interface {

	// Parse extracts metadata from an audio file.
	Parse(ctx context.Context, path string) (*Metadata, error)

	// ParseMultiFile aggregates metadata from multiple audio files.
	// Used for multi-file audiobooks/albums (e.g., one MP3 per chapter).
	ParseMultiFile(ctx context.Context, paths []string) (*Metadata, error)
}

// Metadata contains audio file metadata extracted by parsers.
type Metadata struct {
	Series      string
	Description string
	CoverMIME   string
	Language    string
	ASIN        string
	Codec       string
	Title       string
	Album       string
	Artist      string
	AlbumArtist string
	Composer    string
	Genre       string
	ISBN        string
	SeriesPart  string
	Format      string
	Subtitle    string
	Publisher   string
	Narrator    string
	Chapters    []Chapter
	TrackTotal  int
	Disc        int
	DiscTotal   int
	Duration    time.Duration
	Track       int
	Year        int
	Channels    int
	SampleRate  int
	Bitrate     int
	HasCover    bool
}

// Chapter represents a chapter marker.
type Chapter struct {
	Title     string
	Index     int
	StartTime time.Duration
	EndTime   time.Duration
}
