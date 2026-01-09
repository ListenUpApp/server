package backup

import "time"

// BackupOptions configures backup creation.
type BackupOptions struct {
	IncludeImages bool   // Include cover images and avatars
	IncludeEvents bool   // Include listening events
	OutputPath    string // Where to write the backup file
}

// DefaultBackupOptions returns sensible defaults.
func DefaultBackupOptions() BackupOptions {
	return BackupOptions{
		IncludeImages: false,
		IncludeEvents: true,
	}
}

// RestoreOptions configures restoration.
type RestoreOptions struct {
	Mode          RestoreMode
	MergeStrategy MergeStrategy
	DryRun        bool // Validate without writing
}

// RestoreMode determines how to handle existing data.
type RestoreMode string

const (
	// RestoreModeFull wipes existing data and restores from backup.
	RestoreModeFull RestoreMode = "full"

	// RestoreModeMerge adds backup data to existing data.
	RestoreModeMerge RestoreMode = "merge"

	// RestoreModeEventsOnly only restores listening events.
	RestoreModeEventsOnly RestoreMode = "events_only"
)

// Valid returns true if the restore mode is recognized.
func (m RestoreMode) Valid() bool {
	switch m {
	case RestoreModeFull, RestoreModeMerge, RestoreModeEventsOnly:
		return true
	default:
		return false
	}
}

// MergeStrategy determines conflict resolution in merge mode.
type MergeStrategy string

const (
	// MergeKeepLocal keeps local version on conflict.
	MergeKeepLocal MergeStrategy = "keep_local"

	// MergeKeepBackup uses backup version on conflict.
	MergeKeepBackup MergeStrategy = "keep_backup"

	// MergeNewest uses whichever has newer UpdatedAt.
	MergeNewest MergeStrategy = "newest"
)

// Valid returns true if the merge strategy is recognized.
func (s MergeStrategy) Valid() bool {
	switch s {
	case MergeKeepLocal, MergeKeepBackup, MergeNewest:
		return true
	case "": // Empty is valid (not needed for non-merge modes)
		return true
	default:
		return false
	}
}

// BackupResult contains the outcome of a backup operation.
type BackupResult struct {
	Path     string        `json:"path"`
	Size     int64         `json:"size"`
	Counts   EntityCounts  `json:"counts"`
	Duration time.Duration `json:"duration"`
	Checksum string        `json:"checksum"`
}

// BackupInfo describes an existing backup.
type BackupInfo struct {
	ID        string       `json:"id"`
	Path      string       `json:"path"`
	Size      int64        `json:"size"`
	CreatedAt time.Time    `json:"created_at"`
	Counts    EntityCounts `json:"counts,omitempty"`
}

// RestoreResult contains the outcome of a restore operation.
type RestoreResult struct {
	Imported map[string]int  `json:"imported"`
	Skipped  map[string]int  `json:"skipped"`
	Errors   []RestoreError  `json:"errors,omitempty"`
	Duration time.Duration   `json:"duration"`
}

// RestoreError describes a non-fatal error during restore.
type RestoreError struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id,omitempty"`
	Error      string `json:"error"`
}

// ValidationResult describes backup validity.
type ValidationResult struct {
	Valid          bool         `json:"valid"`
	Manifest       *Manifest    `json:"manifest,omitempty"`
	ExpectedCounts EntityCounts `json:"expected_counts"`
	Errors         []string     `json:"errors,omitempty"`
	Warnings       []string     `json:"warnings,omitempty"`
}
