package util

import "testing"

func TestNormalizeTagSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic normalization
		{"lowercase", "DRAGONS", "dragons"},
		{"spaces to dashes", "slow burn", "slow-burn"},
		{"underscores to dashes", "slow_burn", "slow-burn"},
		{"already normalized", "slow-burn", "slow-burn"},

		// Whitespace handling
		{"trim whitespace", "  dragons  ", "dragons"},
		{"multiple spaces", "slow   burn", "slow-burn"},
		{"tabs and spaces", "slow\t burn", "slow-burn"},

		// Special characters
		{"emoji removal", "🐉 Dragons!", "dragons"},
		{"punctuation removal", "sci-fi/fantasy", "sci-fi-fantasy"},
		{"apostrophe removal", "don't", "dont"},

		// Dash handling
		{"multiple dashes", "slow--burn", "slow-burn"},
		{"leading dashes", "--dragons", "dragons"},
		{"trailing dashes", "dragons--", "dragons"},
		{"mixed dashes", "--slow--burn--", "slow-burn"},

		// Edge cases
		{"empty string", "", ""},
		{"only spaces", "   ", ""},
		{"only special chars", "!@#$%", ""},
		{"numbers allowed", "top10", "top10"},
		{"mixed case with numbers", "Top 10 Books", "top-10-books"},

		// Real-world examples
		{"found family", "Found Family", "found-family"},
		{"unreliable narrator", "Unreliable Narrator", "unreliable-narrator"},
		{"slow burn romance", "Slow-Burn Romance", "slow-burn-romance"},
		{"grimdark", "GrimDark", "grimdark"},
		{"cozy mystery", "cozy_mystery", "cozy-mystery"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeTagSlug(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeTagSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
