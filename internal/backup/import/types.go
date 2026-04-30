// Package backupimport provides backup import and restore functionality.
package backupimport

import "time"

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

// Errors.
var (
	ErrInvalidManifest = invalidManifestError{}
	ErrVersionMismatch = versionMismatchError{}
)

type invalidManifestError struct{}

func (invalidManifestError) Error() string { return "invalid or missing manifest" }

type versionMismatchError struct{}

func (versionMismatchError) Error() string { return "backup version not supported" }
