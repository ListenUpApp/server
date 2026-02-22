package abs

import "time"

// MatchConfidence indicates how certain we are about a match.
// Higher confidence = auto-import. Lower confidence = admin review.
type MatchConfidence int

const (
	// MatchNone means no match was found.
	MatchNone MatchConfidence = iota

	// MatchWeak means the match is uncertain - needs admin review.
	// Examples: title similar but author different, duration mismatch.
	MatchWeak

	// MatchStrong means the match is likely correct.
	// Examples: same path, or (same title + same author + duration within 2%).
	MatchStrong

	// MatchDefinitive means the match is certain.
	// Examples: ASIN match, ISBN match, or admin-specified mapping.
	MatchDefinitive
)

func (c MatchConfidence) String() string {
	switch c {
	case MatchNone:
		return "none"
	case MatchWeak:
		return "weak"
	case MatchStrong:
		return "strong"
	case MatchDefinitive:
		return "definitive"
	default:
		return "unknown"
	}
}

// ShouldAutoImport returns true if this confidence level should be auto-imported.
func (c MatchConfidence) ShouldAutoImport() bool {
	return c >= MatchStrong
}

// UserMatch represents an attempt to match an ABS user to a ListenUp user.
type UserMatch struct {
	ABSUser     *User            // Original ABS user
	ListenUpID  string           // Matched ListenUp user ID (empty if no match)
	Confidence  MatchConfidence  // How confident we are
	MatchReason string           // Human-readable explanation
	Suggestions []UserSuggestion // Possible matches for admin review
}

// UserSuggestion is a suggested ListenUp user for an unmatched ABS user.
type UserSuggestion struct {
	UserID      string  // ListenUp user ID
	Email       string  // For display
	DisplayName string  // For display
	Score       float64 // Similarity score 0.0-1.0
	Reason      string  // Why this is suggested
}

// BookMatch represents an attempt to match an ABS item to a ListenUp book.
type BookMatch struct {
	ABSItem     *LibraryItem     // Original ABS item
	ListenUpID  string           // Matched ListenUp book ID (empty if no match)
	Confidence  MatchConfidence  // How confident we are
	MatchReason string           // Human-readable explanation
	Suggestions []BookSuggestion // Possible matches for admin review
}

// BookSuggestion is a suggested ListenUp book for an unmatched ABS item.
type BookSuggestion struct {
	BookID     string  // ListenUp book ID
	Title      string  // For display
	Author     string  // For display
	DurationMs int64   // For comparison
	Score      float64 // Similarity score 0.0-1.0
	Reason     string  // Why this is suggested
}

// AnalysisOptions configures the backup analysis.
type AnalysisOptions struct {
	// UserMappings are admin-specified ABS user ID → ListenUp user ID mappings.
	// These take precedence over auto-matching.
	UserMappings map[string]string

	// BookMappings are admin-specified ABS item ID → ListenUp book ID mappings.
	BookMappings map[string]string

	// MatchByEmail enables automatic user matching by email address.
	// Default: true
	MatchByEmail bool

	// MatchByPath enables automatic book matching by filesystem path.
	// Only useful if library paths are the same between ABS and ListenUp.
	// Default: true
	MatchByPath bool

	// FuzzyMatchBooks enables fuzzy title/author/duration matching for books.
	// Default: true
	FuzzyMatchBooks bool

	// FuzzyThreshold is the minimum similarity score for fuzzy matches.
	// Default: 0.85 (85% similar)
	FuzzyThreshold float64
}

// DefaultAnalysisOptions returns sensible defaults.
func DefaultAnalysisOptions() AnalysisOptions {
	return AnalysisOptions{
		UserMappings:    make(map[string]string),
		BookMappings:    make(map[string]string),
		MatchByEmail:    true,
		MatchByPath:     true,
		FuzzyMatchBooks: true,
		FuzzyThreshold:  0.85,
	}
}

// AnalysisResult contains the results of analyzing an ABS backup.
// This shows what can be imported and what needs admin attention.
type AnalysisResult struct {
	// Source backup info
	BackupPath string    `json:"backup_path"`
	AnalyzedAt time.Time `json:"analyzed_at"`

	// Summary counts
	TotalUsers    int `json:"total_users"`
	TotalBooks    int `json:"total_books"`
	TotalSessions int `json:"total_sessions"`

	// User matching results
	UserMatches  []UserMatch `json:"user_matches"`
	UsersMatched int         `json:"users_matched"` // Strong or definitive
	UsersPending int         `json:"users_pending"` // None or weak

	// Book matching results
	BookMatches  []BookMatch `json:"book_matches"`
	BooksMatched int         `json:"books_matched"` // Strong or definitive
	BooksPending int         `json:"books_pending"` // None or weak

	// Sessions that can be imported (both user and book matched)
	SessionsReady   int `json:"sessions_ready"`
	SessionsPending int `json:"sessions_pending"`

	// Progress records that can be imported
	ProgressReady   int `json:"progress_ready"`
	ProgressPending int `json:"progress_pending"`

	// Issues found during analysis
	Warnings []string `json:"warnings,omitempty"`
}

// ImportOptions configures the import execution.
type ImportOptions struct {
	// UserMappings are finalized ABS user ID → ListenUp user ID mappings.
	UserMappings map[string]string

	// BookMappings are finalized ABS item ID → ListenUp book ID mappings.
	BookMappings map[string]string

	// SkipUnmatched skips items without matches instead of failing.
	// Default: true (we import what we can)
	SkipUnmatched bool

	// ImportSessions imports historical listening sessions.
	// Default: true
	ImportSessions bool

	// ImportProgress imports current progress state.
	// Default: true
	ImportProgress bool
}

// DefaultImportOptions returns sensible defaults.
func DefaultImportOptions() ImportOptions {
	return ImportOptions{
		UserMappings:   make(map[string]string),
		BookMappings:   make(map[string]string),
		SkipUnmatched:  true,
		ImportSessions: true,
		ImportProgress: true,
	}
}

// ImportResult contains the results of executing an ABS import.
type ImportResult struct {
	// What was imported
	SessionsImported         int `json:"sessions_imported"`
	SessionsSkipped          int `json:"sessions_skipped"`
	ProgressImported         int `json:"progress_imported"`
	ProgressSkipped          int `json:"progress_skipped"`
	EventsCreated            int `json:"events_created"`             // Total ListeningEvents created
	ReadingSessionsCreated   int `json:"reading_sessions_created"`   // BookReadingSession records for readers section
	ProgressOverridesApplied int `json:"progress_overrides_applied"` // Authoritative MediaProgress overrides applied

	// Users whose progress was affected (for rebuild)
	AffectedUserIDs []string `json:"affected_user_ids"`

	// Duration
	Duration time.Duration `json:"duration"`

	// Issues
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}
