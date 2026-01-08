package domain

import (
	"slices"
	"time"
)

// ActivityType represents the type of social activity.
type ActivityType string

const (
	// ActivityStartedBook is recorded when a user starts reading a book.
	// IsReread distinguishes first read from re-reads after completion.
	ActivityStartedBook ActivityType = "started_book"

	// ActivityFinishedBook is recorded when a user completes a book (99%+ progress).
	ActivityFinishedBook ActivityType = "finished_book"

	// ActivityStreakMilestone is recorded when a user hits a streak milestone.
	ActivityStreakMilestone ActivityType = "streak_milestone"

	// ActivityListeningMilestone is recorded when a user crosses a listening hours milestone.
	ActivityListeningMilestone ActivityType = "listening_milestone"

	// ActivityLensCreated is recorded when a user creates a new lens.
	ActivityLensCreated ActivityType = "lens_created"

	// ActivityListeningSession is recorded when a user completes a listening session.
	// Shows duration listened (e.g., "John listened to 16 minutes of Book Title").
	// Only recorded for sessions >= MinListeningSessionMinutes to avoid spam.
	ActivityListeningSession ActivityType = "listening_session"
)

// MinListeningSessionMs is the minimum duration in milliseconds for a listening session
// to generate an activity. Prevents spam from very short sessions.
// Set to 30 seconds for testing - events come every 30 seconds while playing.
const MinListeningSessionMs = 30_000 // 30 seconds

// Activity represents a social activity event.
// Activities are immutable once created.
// User and book info is denormalized for fast feed rendering without joins.
type Activity struct {
	ID        string       `json:"id"`
	UserID    string       `json:"user_id"`
	Type      ActivityType `json:"type"`
	CreatedAt time.Time    `json:"created_at"`

	// Denormalized user info for immediate rendering
	UserDisplayName string `json:"user_display_name"`
	UserAvatarColor string `json:"user_avatar_color"`
	UserAvatarType  string `json:"user_avatar_type"`            // "auto" or "image"
	UserAvatarValue string `json:"user_avatar_value,omitempty"` // Image path for "image" type

	// Book activities (started_book, finished_book, listening_session)
	BookID         string `json:"book_id,omitempty"`
	BookTitle      string `json:"book_title,omitempty"`
	BookAuthorName string `json:"book_author_name,omitempty"`
	BookCoverPath  string `json:"book_cover_path,omitempty"`
	IsReread       bool   `json:"is_reread,omitempty"`

	// Listening session activities
	DurationMs int64 `json:"duration_ms,omitempty"` // Duration listened in milliseconds

	// Milestone activities (streak_milestone, listening_milestone)
	MilestoneValue int    `json:"milestone_value,omitempty"`
	MilestoneUnit  string `json:"milestone_unit,omitempty"` // "days" or "hours"

	// Lens activities (lens_created)
	LensID   string `json:"lens_id,omitempty"`
	LensName string `json:"lens_name,omitempty"`
}

// Milestone thresholds.
var (
	// StreakMilestones are the day counts that trigger streak milestone activities.
	StreakMilestones = []int{7, 14, 30, 60, 100, 365}

	// ListeningMilestones are the hour counts that trigger listening milestone activities.
	ListeningMilestones = []int{10, 50, 100, 250, 500, 1000}
)

// IsStreakMilestone returns true if the given day count is a milestone.
func IsStreakMilestone(days int) bool {
	return slices.Contains(StreakMilestones, days)
}

// CrossedListeningMilestone returns true if going from prevHours to newHours
// crosses a milestone threshold. Returns the milestone value if crossed.
func CrossedListeningMilestone(prevHours, newHours int) (crossed bool, milestone int) {
	for _, m := range ListeningMilestones {
		if prevHours < m && newHours >= m {
			return true, m
		}
	}
	return false, 0
}

// UserMilestoneState tracks a user's previous milestone values
// to detect when they cross thresholds.
type UserMilestoneState struct {
	UserID               string    `json:"user_id"`
	LastStreakDays       int       `json:"last_streak_days"`
	LastListenHoursTotal int       `json:"last_listen_hours_total"`
	UpdatedAt            time.Time `json:"updated_at"`
}
