package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNeedsTranscode(t *testing.T) {
	tests := []struct {
		codec    string
		expected bool
	}{
		// Codecs that need transcoding
		{"ac3", true},
		{"eac3", true},
		{"ac4", true},
		{"ac-4", true}, // ffprobe reports with hyphen
		{"truehd", true},
		{"dts", true},
		{"wma", true},

		// Codecs that don't need transcoding
		{"aac", false},
		{"mp3", false},
		{"opus", false},
		{"flac", false},
		{"vorbis", false},
		{"pcm_s16le", false},

		// Unknown codecs default to not needing transcode
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.codec, func(t *testing.T) {
			result := NeedsTranscode(tt.codec)
			assert.Equal(t, tt.expected, result, "NeedsTranscode(%q)", tt.codec)
		})
	}
}
