package domain

import "time"

// BookPreferences contains per-book settings that override user defaults.
// Nil fields mean "use global default from UserSettings".
type BookPreferences struct {
	UserID string `json:"user_id"`
	BookID string `json:"book_id"`

	PlaybackSpeed  *float32 `json:"playback_speed,omitempty"`
	SkipForwardSec *int     `json:"skip_forward_sec,omitempty"`

	HideFromContinueListening bool `json:"hide_from_continue_listening"`

	UpdatedAt time.Time `json:"updated_at"`
}

// BookPreferencesID generates composite key: "userID:prefs:bookID"
func BookPreferencesID(userID, bookID string) string {
	return userID + ":prefs:" + bookID
}

// NewBookPreferences creates empty preferences (all defaults).
func NewBookPreferences(userID, bookID string) *BookPreferences {
	return &BookPreferences{
		UserID:    userID,
		BookID:    bookID,
		UpdatedAt: time.Now(),
	}
}
