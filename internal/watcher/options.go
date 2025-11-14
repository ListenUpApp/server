package watcher

import (
	"path/filepath"
	"strings"
	"time"
)

// Options configures the file watcher behavior.
type Options struct {
	IgnorePatterns []string
	SettleDelay    time.Duration
	IgnoreHidden   bool
}

// setDefaults applies default values to unset options.
func (o *Options) setDefaults() {
	if o.SettleDelay == 0 {
		o.SettleDelay = 100 * time.Millisecond
	}

	// Set default ignore patterns if none specified (nil, not just empty).
	if o.IgnorePatterns == nil {
		o.IgnorePatterns = []string{
			".DS_Store",
			"*.tmp",
			"*.temp",
			"Thumbs.db",
		}
		// Also default to ignoring hidden files when no custom config provided.
		// If patterns were explicitly set (even to empty slice), respect user's IgnoreHidden choice.
		o.IgnoreHidden = true
	}
}

// shouldIgnore checks if a path matches ignore patterns.
func (o *Options) shouldIgnore(path string) bool {
	// Check if hidden and we're ignoring hidden files.
	if o.IgnoreHidden {
		parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
		for _, part := range parts {
			if strings.HasPrefix(part, ".") && part != "." && part != ".." {
				return true
			}
		}
	}

	// Check against ignore patterns.
	base := filepath.Base(path)
	for _, pattern := range o.IgnorePatterns {
		matched, err := filepath.Match(pattern, base)
		if err == nil && matched {
			return true
		}
	}

	return false
}
