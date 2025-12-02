package domain

import "time"

// UserSettings contains user-level playback defaults.
// These apply to all books unless overridden per-book.
type UserSettings struct {
	UserID string `json:"user_id"`

	DefaultPlaybackSpeed   float32 `json:"default_playback_speed"`
	DefaultSkipForwardSec  int     `json:"default_skip_forward_sec"`
	DefaultSkipBackwardSec int     `json:"default_skip_backward_sec"`

	DefaultSleepTimerMin   *int `json:"default_sleep_timer_min,omitempty"`
	ShakeToResetSleepTimer bool `json:"shake_to_reset_sleep_timer"`

	UpdatedAt time.Time `json:"updated_at"`
}

// NewUserSettings creates settings with sensible defaults.
func NewUserSettings(userID string) *UserSettings {
	return &UserSettings{
		UserID:                 userID,
		DefaultPlaybackSpeed:   1.0,
		DefaultSkipForwardSec:  30,
		DefaultSkipBackwardSec: 10,
		UpdatedAt:              time.Now(),
	}
}
