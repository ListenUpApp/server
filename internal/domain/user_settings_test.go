package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUserSettings_Defaults(t *testing.T) {
	settings := NewUserSettings("user-123")

	require.NotNil(t, settings)
	assert.Equal(t, "user-123", settings.UserID)
	assert.Equal(t, float32(1.0), settings.DefaultPlaybackSpeed)
	assert.Equal(t, 30, settings.DefaultSkipForwardSec)
	assert.Equal(t, 10, settings.DefaultSkipBackwardSec)
	assert.Nil(t, settings.DefaultSleepTimerMin)
	assert.False(t, settings.ShakeToResetSleepTimer)
	assert.False(t, settings.UpdatedAt.IsZero())
}
