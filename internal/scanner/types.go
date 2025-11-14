package scanner

import (
	"time"
)

// ScanResult represents the outcome of scanning a library.
type ScanResult struct {
	StartedAt   time.Time
	CompletedAt time.Time
	Progress    *Progress
	LibraryID   string
	Added       int
	Updated     int
	Removed     int
	Unchanged   int
	Errors      int
}

// LibraryItemData represents a discovered library item.
type LibraryItemData struct {
	ModTime       time.Time
	Metadata      *BookMetadata
	Path          string
	RelPath       string
	AudioFiles    []AudioFileData
	ImageFiles    []ImageFileData
	MetadataFiles []MetadataFileData
	Inode         uint64
	Size          int64
	IsFile        bool
}

// AudioFileData represents a discovered audio file.
type AudioFileData struct {
	ModTime  time.Time
	Metadata *AudioMetadata
	Path     string
	RelPath  string
	Filename string
	Ext      string
	Size     int64
	Inode    uint64
}

// BookMetadata is the metadata for an audiobook.
type BookMetadata struct {
	Sources     map[string]string
	ASIN        string
	ISBN        string
	Subtitle    string
	Title       string
	Description string
	Publisher   string
	PublishYear string
	Language    string
	Genres      []string
	Tags        []string
	Authors     []string
	Series      []SeriesInfo
	Chapters    []Chapter
	Narrators   []string
	Explicit    bool
	Abridged    bool
}

// SeriesInfo represents series metadata.
type SeriesInfo struct {
	Name     string
	Sequence string
}

// Chapter represents a chapter marker.
type Chapter struct {
	Title     string
	ID        int
	StartTime time.Duration
	EndTime   time.Duration
}

// ImageFileData represents a discovered image file (cover art).
type ImageFileData struct {
	ModTime  time.Time
	Path     string
	RelPath  string
	Filename string
	Ext      string
	Size     int64
	Inode    uint64
}

// MetadataFileData represents a discovered metadata file.
// (metadata.json, metadata.abs, .opf, .nfo, desc.txt, reader.txt).
type MetadataFileData struct {
	ModTime  time.Time
	Path     string
	RelPath  string
	Filename string
	Ext      string
	Type     MetadataFileType
	Size     int64
	Inode    uint64
}

// MetadataFileType identifies the type of metadata file.
type MetadataFileType string

// MetadataFileType constants define the different types of metadata files.
const (
	// MetadataTypeJSON represents JSON metadata files.
	MetadataTypeJSON MetadataFileType = "metadata.json"
	// MetadataTypeABS represents AudioBookShelf metadata files.
	MetadataTypeABS MetadataFileType = "metadata.abs"
	// MetadataTypeOPF represents OPF (Open Packaging Format) metadata files.
	MetadataTypeOPF MetadataFileType = "opf"
	// MetadataTypeNFO represents NFO metadata files.
	MetadataTypeNFO MetadataFileType = "nfo"
	// MetadataTypeDesc represents description text files.
	MetadataTypeDesc MetadataFileType = "desc.txt"
	// MetadataTypeReader represents reader text files.
	MetadataTypeReader MetadataFileType = "reader.txt"
	// MetadataTypeUnknown represents unknown metadata file types.
	MetadataTypeUnknown MetadataFileType = "unknown"
)

// AudioMetadata represents parsed metadata from an audio file.
// This is populated by the analyzer after parsing with ffprobe/native parser.
type AudioMetadata struct {
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

// Progress tracks scan progress.
type Progress struct {
	Phase       ScanPhase
	CurrentItem string
	Errors      []ScanError
	Current     int
	Total       int
	Added       int
	Updated     int
	Removed     int
}

// ScanPhase represents the current scan phase.
type ScanPhase string

// ScanPhase constants define the different phases of a library scan.
const (
	// PhaseWalking represents the file system walking phase.
	PhaseWalking ScanPhase = "walking"
	// PhaseGrouping represents the file grouping phase.
	PhaseGrouping ScanPhase = "grouping"
	// PhaseAnalyzing represents the metadata analysis phase.
	PhaseAnalyzing ScanPhase = "analyzing"
	// PhaseResolving represents the dependency resolution phase.
	PhaseResolving ScanPhase = "resolving"
	// PhaseDiffing represents the diff computation phase.
	PhaseDiffing ScanPhase = "diffing"
	// PhaseApplying represents the database update phase.
	PhaseApplying ScanPhase = "applying"
	// PhaseComplete represents the completion phase.
	PhaseComplete ScanPhase = "complete"
)

// ScanError represents an error during scanning.
type ScanError struct {
	Time  time.Time
	Error error
	Path  string
	Phase ScanPhase
}

// ScanDiff  represents changes detected during scanning.
type ScanDiff struct {
	Added   []LibraryItemData
	Updated []ItemUpdate
	Removed []string // item id
}

// ItemUpdate represents an update to an existing item.
type ItemUpdate struct {
	Changes map[string]FieldChange
	ID      string
}

// FieldChange represents a field changescan.
type FieldChange struct {
	OldValue any
	NewValue any
	Field    string
}
