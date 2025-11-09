package watcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOptions_Defaults(t *testing.T) {
	opts := Options{}
	opts.setDefaults()

	assert.True(t, opts.IgnoreHidden, "Should ignore hidden files by default")
	assert.Equal(t, 100*time.Millisecond, opts.SettleDelay, "Default settle delay should be 100ms")
	assert.Contains(t, opts.IgnorePatterns, ".DS_Store", "Should ignore .DS_Store by default")
	assert.Contains(t, opts.IgnorePatterns, "*.tmp", "Should ignore *.tmp by default")
}

func TestOptions_CustomValues(t *testing.T) {
	opts := Options{
		IgnoreHidden:   false,
		SettleDelay:    200 * time.Millisecond,
		IgnorePatterns: []string{"*.bak"},
	}
	opts.setDefaults()

	assert.False(t, opts.IgnoreHidden, "Custom ignore hidden should be preserved")
	assert.Equal(t, 200*time.Millisecond, opts.SettleDelay, "Custom settle delay should be preserved")
	assert.Contains(t, opts.IgnorePatterns, "*.bak", "Custom patterns should be preserved")
}

func TestOptions_ShouldIgnore(t *testing.T) {
	opts := Options{
		IgnoreHidden:   true,
		IgnorePatterns: []string{"*.tmp", ".DS_Store", "*.bak"},
	}
	opts.setDefaults()

	tests := []struct {
		name   string
		path   string
		expect bool
	}{
		{"hidden file", "/path/.hidden", true},
		{"hidden directory", "/path/.git/config", true},
		{"DS_Store", "/path/.DS_Store", true},
		{"tmp file", "/path/file.tmp", true},
		{"bak file", "/path/file.bak", true},
		{"normal file", "/path/file.m4b", false},
		{"normal path", "/path/to/file.mp3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := opts.shouldIgnore(tt.path)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestOptions_ShouldIgnore_NoIgnoreHidden(t *testing.T) {
	opts := Options{
		IgnoreHidden:   false,
		IgnorePatterns: []string{},
	}
	opts.setDefaults()

	assert.False(t, opts.shouldIgnore("/path/.hidden"), "Should not ignore hidden when disabled")
	assert.False(t, opts.shouldIgnore("/path/file.m4b"), "Should not ignore normal files")
}
