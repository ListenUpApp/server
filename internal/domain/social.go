package domain

// LeaderboardCategory defines the ranking metric.
type LeaderboardCategory string

const (
	LeaderboardCategoryTime   LeaderboardCategory = "time"
	LeaderboardCategoryBooks  LeaderboardCategory = "books"
	LeaderboardCategoryStreak LeaderboardCategory = "streak"
)

// Valid checks if the category is valid.
func (c LeaderboardCategory) Valid() bool {
	switch c {
	case LeaderboardCategoryTime, LeaderboardCategoryBooks, LeaderboardCategoryStreak:
		return true
	default:
		return false
	}
}

// LeaderboardEntry represents a single user's ranking.
type LeaderboardEntry struct {
	Rank        int     `json:"rank"`
	UserID      string  `json:"user_id"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Value       int64   `json:"value"`       // Time in ms, book count, or streak days
	ValueLabel  string  `json:"value_label"` // Human-readable value (e.g., "12h 30m")
	IsCurrentUser bool  `json:"is_current_user"`
}

// Leaderboard contains the ranked entries and aggregate stats.
type Leaderboard struct {
	Category    LeaderboardCategory
	Period      StatsPeriod
	Entries     []LeaderboardEntry
	TotalUsers  int

	// Community aggregate stats
	CommunityTotalTimeMs    int64
	CommunityTotalBooks     int
	CommunityAverageStreak  float64
}
