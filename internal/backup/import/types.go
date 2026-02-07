// Package backupimport provides backup import and restore functionality.
package backupimport

import "time"

// FormatVersion is the backup format version.
const FormatVersion = "1.0"

// RestoreMode defines how restore handles existing data.
type RestoreMode string

const (
	// RestoreModeFull wipes existing data and restores everything.
	RestoreModeFull RestoreMode = "full"
	// RestoreModeMerge adds new data, resolves conflicts with strategy.
	RestoreModeMerge RestoreMode = "merge"
	// RestoreModeEventsOnly imports only listening events + sessions.
	RestoreModeEventsOnly RestoreMode = "events_only"
)

// MergeStrategy defines conflict resolution for merge mode.
type MergeStrategy string

const (
	// MergeKeepLocal keeps local version on conflict.
	MergeKeepLocal MergeStrategy = "keep_local"
	// MergeKeepBackup keeps backup version on conflict.
	MergeKeepBackup MergeStrategy = "keep_backup"
	// MergeNewest keeps whichever version is newer.
	MergeNewest MergeStrategy = "newest"
)

// RestoreOptions configures restore behavior.
type RestoreOptions struct {
	Mode          RestoreMode
	MergeStrategy MergeStrategy
	DryRun        bool
}

// RestoreResult reports what was restored.
type RestoreResult struct {
	Imported map[string]int
	Skipped  map[string]int
	Errors   []RestoreError
	Duration time.Duration
}

// RestoreError represents a non-fatal restore error.
type RestoreError struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id,omitempty"`
	Error      string `json:"error"`
}

// Manifest describes backup contents and metadata.
type Manifest struct {
	Version          string       `json:"version"`
	CreatedAt        time.Time    `json:"created_at"`
	ServerID         string       `json:"server_id"`
	ServerName       string       `json:"server_name"`
	ListenUpVersion  string       `json:"listenup_version"`
	Counts           EntityCounts `json:"counts"`
	IncludesImages   bool         `json:"includes_images"`
	IncludesEvents   bool         `json:"includes_events"`
	IncludesSettings bool         `json:"includes_settings"`
}

// EntityCounts tracks entity counts for validation and progress reporting.
type EntityCounts struct {
	Users            int `json:"users"`
	Libraries        int `json:"libraries"`
	Books            int `json:"books"`
	Contributors     int `json:"contributors"`
	Series           int `json:"series"`
	Genres           int `json:"genres"`
	Tags             int `json:"tags"`
	Collections      int `json:"collections"`
	CollectionShares int `json:"collection_shares"`
	Shelves           int `json:"shelves"`
	Activities       int `json:"activities"`
	ListeningEvents  int `json:"listening_events"`
	ReadingSessions  int `json:"reading_sessions"`
	Images           int `json:"images,omitempty"`
}

// Errors.
var (
	ErrInvalidManifest = errInvalidManifest{}
	ErrVersionMismatch = errVersionMismatch{}
)

type errInvalidManifest struct{}

func (errInvalidManifest) Error() string { return "invalid or missing manifest" }

type errVersionMismatch struct{}

func (errVersionMismatch) Error() string { return "backup version not supported" }
