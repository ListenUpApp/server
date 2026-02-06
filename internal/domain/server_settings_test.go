package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerSettings_Defaults(t *testing.T) {
	settings := NewServerSettings()

	require.NotNil(t, settings)
	assert.Equal(t, "", settings.Name)
	assert.False(t, settings.InboxEnabled)
	assert.False(t, settings.UpdatedAt.IsZero())
}

func TestServerSettings_GetDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty name returns default",
			input:    "",
			expected: "ListenUp Server",
		},
		{
			name:     "custom name returned as-is",
			input:    "My Library",
			expected: "My Library",
		},
		{
			name:     "whitespace-only treated as set",
			input:    "  ",
			expected: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &ServerSettings{Name: tt.input}
			assert.Equal(t, tt.expected, settings.GetDisplayName())
		})
	}
}
