package audiometa

import (
	"fmt"
	"time"
)

// Metadata represents audio file metadata
type Metadata struct {
	// Basic info
	Title    string
	Artist   string
	Album    string
	Year     int
	Genre    string
	Composer string
	Comment  string

	// Track info
	TrackNumber int
	TrackTotal  int
	DiscNumber  int
	DiscTotal   int

	// Technical info
	Duration   time.Duration // Total duration
	BitRate    int           // Bits per second
	SampleRate int           // Samples per second
	Channels   int           // Number of audio channels
	Codec      string        // Audio codec (e.g., "AAC", "ALAC")

	// Audiobook-specific
	Narrator   string `json:"narrator,omitempty"`
	Publisher  string `json:"publisher,omitempty"`
	Series     string `json:"series,omitempty"`
	SeriesPart string `json:"series_part,omitempty"`
	ISBN       string `json:"isbn,omitempty"`
	ASIN       string `json:"asin,omitempty"`

	// File info
	FileSize int64  // File size in bytes
	Format   Format // Detected format (M4B, M4A)

	// Chapters
	Chapters []Chapter `json:"chapters,omitempty"`

	// Warnings contains non-fatal errors encountered during parsing
	// These indicate partial data loss but don't prevent metadata extraction
	Warnings []string `json:"warnings,omitempty"`
}

// AddWarning adds a non-fatal warning to the metadata
func (m *Metadata) AddWarning(format string, args ...any) {
	if m.Warnings == nil {
		m.Warnings = make([]string, 0)
	}
	m.Warnings = append(m.Warnings, fmt.Sprintf(format, args...))
}

// Chapter represents a chapter marker
type Chapter struct {
	Index     int           `json:"index"`
	Title     string        `json:"title"`
	StartTime time.Duration `json:"start_time"`
	EndTime   time.Duration `json:"end_time"`
}
