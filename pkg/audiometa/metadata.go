package audiometa

import "time"

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

	// File info
	FileSize int64  // File size in bytes
	Format   Format // Detected format (M4B, M4A)
}
