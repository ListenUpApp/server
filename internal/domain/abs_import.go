package domain

import "time"

// ABSImportStatus represents the lifecycle state of an ABS import.
type ABSImportStatus string

const (
	ABSImportStatusActive    ABSImportStatus = "active"
	ABSImportStatusCompleted ABSImportStatus = "completed"
	ABSImportStatusArchived  ABSImportStatus = "archived"
)

// SessionImportStatus represents the import state of an individual session.
type SessionImportStatus string

const (
	SessionStatusPendingUser SessionImportStatus = "pending_user" // Waiting for user mapping
	SessionStatusPendingBook SessionImportStatus = "pending_book" // Waiting for book mapping
	SessionStatusReady       SessionImportStatus = "ready"        // Both mapped, ready to import
	SessionStatusImported    SessionImportStatus = "imported"     // Successfully imported
	SessionStatusSkipped     SessionImportStatus = "skipped"      // Explicitly skipped by admin
)

// ABSImport represents a connected ABS backup that can be processed incrementally.
// The import persists across sessions, allowing admins to leave and return later.
type ABSImport struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`        // User-friendly name (e.g., "ABS Backup 2024")
	BackupPath  string          `json:"backup_path"` // Original file path for reference
	Status      ABSImportStatus `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`

	// Summary stats (denormalized for quick display)
	TotalUsers       int `json:"total_users"`
	TotalBooks       int `json:"total_books"`
	TotalSessions    int `json:"total_sessions"`
	UsersMapped      int `json:"users_mapped"`
	BooksMapped      int `json:"books_mapped"`
	SessionsImported int `json:"sessions_imported"`
}

// ABSImportUser represents a parsed ABS user with mapping state.
type ABSImportUser struct {
	ImportID    string `json:"import_id"`
	ABSUserID   string `json:"abs_user_id"`
	ABSUsername string `json:"abs_username"`
	ABSEmail    string `json:"abs_email"`

	// Mapping to ListenUp user
	ListenUpID          *string    `json:"listenup_id,omitempty"`
	ListenUpEmail       *string    `json:"listenup_email,omitempty"`        // Resolved display info
	ListenUpDisplayName *string    `json:"listenup_display_name,omitempty"` // Resolved display info
	MappedAt            *time.Time `json:"mapped_at,omitempty"`

	// Stats for display (helps admin decide on mappings)
	SessionCount  int   `json:"session_count"`
	TotalListenMs int64 `json:"total_listen_ms"`

	// Matching hints from analysis
	Confidence  string   `json:"confidence"`   // none, weak, strong, definitive
	MatchReason string   `json:"match_reason"` // Human-readable explanation
	Suggestions []string `json:"suggestions"`  // ListenUp user IDs to suggest
}

// IsMapped returns true if this ABS user is mapped to a ListenUp user.
func (u *ABSImportUser) IsMapped() bool {
	return u.ListenUpID != nil && *u.ListenUpID != ""
}

// ABSImportBook represents a parsed ABS book with mapping state.
type ABSImportBook struct {
	ImportID      string `json:"import_id"`
	ABSMediaID    string `json:"abs_media_id"` // The actual matching key (books.id in ABS)
	ABSTitle      string `json:"abs_title"`
	ABSAuthor     string `json:"abs_author"`
	ABSDurationMs int64  `json:"abs_duration_ms"`
	ABSASIN       string `json:"abs_asin,omitempty"`
	ABSISBN       string `json:"abs_isbn,omitempty"`

	// Mapping to ListenUp book
	ListenUpID     *string    `json:"listenup_id,omitempty"`
	ListenUpTitle  *string    `json:"listenup_title,omitempty"`  // Resolved display info
	ListenUpAuthor *string    `json:"listenup_author,omitempty"` // Resolved display info
	MappedAt       *time.Time `json:"mapped_at,omitempty"`

	// Stats for display
	SessionCount int `json:"session_count"`

	// Matching hints from analysis
	Confidence  string   `json:"confidence"`
	MatchReason string   `json:"match_reason"`
	Suggestions []string `json:"suggestions"` // ListenUp book IDs to suggest
}

// IsMapped returns true if this ABS book is mapped to a ListenUp book.
func (b *ABSImportBook) IsMapped() bool {
	return b.ListenUpID != nil && *b.ListenUpID != ""
}

// ABSImportSession represents a parsed ABS listening session with import status.
type ABSImportSession struct {
	ImportID     string `json:"import_id"`
	ABSSessionID string `json:"abs_session_id"`
	ABSUserID    string `json:"abs_user_id"`
	ABSMediaID   string `json:"abs_media_id"`

	// Session data
	StartTime     time.Time `json:"start_time"`
	Duration      int64     `json:"duration"`       // milliseconds
	StartPosition int64     `json:"start_position"` // milliseconds
	EndPosition   int64     `json:"end_position"`   // milliseconds

	// Import status
	Status     SessionImportStatus `json:"status"`
	ImportedAt *time.Time          `json:"imported_at,omitempty"`
	SkipReason *string             `json:"skip_reason,omitempty"`
}

// CanImport returns true if the session is ready to be imported.
func (s *ABSImportSession) CanImport() bool {
	return s.Status == SessionStatusReady
}

// ABSImportProgress represents a parsed ABS media progress entry.
// This is used when no session history exists for a book.
type ABSImportProgress struct {
	ImportID   string `json:"import_id"`
	ABSUserID  string `json:"abs_user_id"`
	ABSMediaID string `json:"abs_media_id"`

	// Progress data
	CurrentTime int64      `json:"current_time"` // Position in milliseconds
	Duration    int64      `json:"duration"`     // Total duration in milliseconds
	Progress    float64    `json:"progress"`     // 0.0 to 1.0
	IsFinished  bool       `json:"is_finished"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"` // When the book was finished
	LastUpdate  time.Time  `json:"last_update"`

	// Import status
	Status     SessionImportStatus `json:"status"`
	ImportedAt *time.Time          `json:"imported_at,omitempty"`
}

// MappingFilter specifies which items to return when listing.
type MappingFilter string

const (
	MappingFilterAll      MappingFilter = "all"
	MappingFilterMapped   MappingFilter = "mapped"
	MappingFilterUnmapped MappingFilter = "unmapped"
)

// SessionStatusFilter specifies which sessions to return when listing.
type SessionStatusFilter string

const (
	SessionFilterAll      SessionStatusFilter = "all"
	SessionFilterPending  SessionStatusFilter = "pending" // pending_user or pending_book
	SessionFilterReady    SessionStatusFilter = "ready"
	SessionFilterImported SessionStatusFilter = "imported"
	SessionFilterSkipped  SessionStatusFilter = "skipped"
)
