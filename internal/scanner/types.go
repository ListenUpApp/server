package scanner

import (
	"time"
)

// ScanResult represents the outcome of scanning a library
type ScanResult struct {
	LibraryID   string
	StartedAt   time.Time
	CompletedAt time.Time

	Added     int
	Updated   int
	Removed   int
	Unchanged int
	Errors    int

	Progress *Progress
}

// LibraryItemData represents a discovered library item
type LibraryItemData struct {
	Path    string
	RelPath string
	IsFile  bool // true if single audio file, false if directory
	ModTime time.Time
	Inode   uint64
	Size    int64

	AudioFiles    []AudioFileData
	ImageFiles    []ImageFileData
	MetadataFiles []MetadataFileData

	Metadata *BookMetadata
}

// AudioFileData represents a discovered audio file
type AudioFileData struct {
	Path     string
	RelPath  string
	Filename string
	Ext      string
	Size     int64
	ModTime  time.Time
	Inode    uint64

	Metadata *AudioMetadata
}

// BookMetadata is the metadata for an audiobook
type BookMetadata struct {
	Title       string
	Subtitle    string
	Authors     []string
	Narrators   []string
	Series      []SeriesInfo
	Description string
	Publisher   string
	PublishYear string
	Language    string
	Genres      []string
	Tags        []string
	ISBN        string
	ASIN        string
	Explicit    bool
	Abridged    bool
	Chapters    []Chapter

	// Source tracking (for debugging)
	Sources map[string]string // field -> source
}

// SeriesInfo represents series metadata
type SeriesInfo struct {
	Name     string
	Sequence string
}

// Chapter represents a chapter marker
type Chapter struct {
	ID        int
	Title     string
	StartTime time.Duration
	EndTime   time.Duration
}

// ImageFileData represents a discovered image file (cover art)
type ImageFileData struct {
	Path     string
	RelPath  string
	Filename string
	Ext      string
	Size     int64
	ModTime  time.Time
	Inode    uint64
}

// MetadataFileData represents a discovered metadata file
// (metadata.json, metadata.abs, .opf, .nfo, desc.txt, reader.txt)
type MetadataFileData struct {
	Path     string
	RelPath  string
	Filename string
	Ext      string
	Type     MetadataFileType
	Size     int64
	ModTime  time.Time
	Inode    uint64
}

// MetadataFileType identifies the type of metadata file
type MetadataFileType string

const (
	MetadataTypeJSON    MetadataFileType = "metadata.json"
	MetadataTypeABS     MetadataFileType = "metadata.abs"
	MetadataTypeOPF     MetadataFileType = "opf"
	MetadataTypeNFO     MetadataFileType = "nfo"
	MetadataTypeDesc    MetadataFileType = "desc.txt"
	MetadataTypeReader  MetadataFileType = "reader.txt"
	MetadataTypeUnknown MetadataFileType = "unknown"
)

// AudioMetadata represents parsed metadata from an audio file
// This is populated by the analyzer after parsing with ffprobe/native parser
type AudioMetadata struct {
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

	// Audiobook-specific tags
	Narrator    string
	Publisher   string
	Description string
	Subtitle    string
	Series      string
	SeriesPart  string
	ISBN        string
	ASIN        string
	Language    string

	// Chapters (if embedded in file)
	Chapters []Chapter

	// Embedded artwork
	HasCover  bool
	CoverMIME string
}

// Progress tracks scan progress
type Progress struct {
	Phase       ScanPhase
	Current     int
	Total       int
	CurrentItem string

	Added   int
	Updated int
	Removed int
	Errors  []ScanError
}

// ScanPhase represents the current scan phase
type ScanPhase string

const (
	PhaseWalking   ScanPhase = "walking"
	PhaseGrouping  ScanPhase = "grouping"
	PhaseAnalyzing ScanPhase = "analyzing"
	PhaseResolving ScanPhase = "resolving"
	PhaseDiffing   ScanPhase = "diffing"
	PhaseApplying  ScanPhase = "applying"
	PhaseComplete  ScanPhase = "complete"
)

// ScanError represents an error during scanning
type ScanError struct {
	Path  string
	Phase ScanPhase
	Error error
	Time  time.Time
}

// ScanDiff  represents changes detected during scanning
type ScanDiff struct {
	Added   []LibraryItemData
	Updated []ItemUpdate
	Removed []string //item id
}

// ItemUpdate represents an update to an existing item
type ItemUpdate struct {
	ID      string
	Changes map[string]FieldChange
}

// FieldChange represents a field changescan
type FieldChange struct {
	Field    string
	OldValue any
	NewValue any
}
