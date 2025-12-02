package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBookPreferencesID(t *testing.T) {
	id := BookPreferencesID("user-123", "book-456")
	assert.Equal(t, "user-123:prefs:book-456", id)
}

func TestNewBookPreferences(t *testing.T) {
	prefs := NewBookPreferences("user-123", "book-456")

	assert.Equal(t, "user-123", prefs.UserID)
	assert.Equal(t, "book-456", prefs.BookID)
	assert.Nil(t, prefs.PlaybackSpeed)
	assert.Nil(t, prefs.SkipForwardSec)
	assert.False(t, prefs.HideFromContinueListening)
	assert.False(t, prefs.UpdatedAt.IsZero())
}
