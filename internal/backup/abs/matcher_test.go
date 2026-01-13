package abs

import "testing"

func TestNormalizeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello world"},
		{"  trimmed  ", "trimmed"},
		{"UPPERCASE", "uppercase"},
		{"MixedCase", "mixedcase"},
		{"", ""},
	}

	for _, tc := range tests {
		result := normalizeString(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeString(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The Great Book", "great book"},
		{"A Simple Title", "simple title"},
		{"An Example", "example"},
		{"Harry Potter: The Sorcerer's Stone", "harry potter the sorcerers stone"},
		{"Hello, World!", "hello world"},
		{"   Multiple   Spaces   ", "multiple spaces"},
	}

	for _, tc := range tests {
		result := normalizeTitle(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestStringSimilarity(t *testing.T) {
	tests := []struct {
		a, b     string
		minScore float64
		maxScore float64
	}{
		{"identical", "identical", 1.0, 1.0},
		{"", "", 1.0, 1.0}, // Both empty = identical
		{"", "something", 0.0, 0.0},
		{"hello", "helo", 0.7, 0.9},          // One char difference
		{"completely", "different", 0.0, 0.3}, // Very different
		{"similar", "similr", 0.8, 0.95},      // One char missing
	}

	for _, tc := range tests {
		result := stringSimilarity(tc.a, tc.b)
		if result < tc.minScore || result > tc.maxScore {
			t.Errorf("stringSimilarity(%q, %q) = %v, want between %v and %v",
				tc.a, tc.b, result, tc.minScore, tc.maxScore)
		}
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"hello", "", 5},
		{"", "world", 5},
		{"hello", "hello", 0},
		{"hello", "helo", 1},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
	}

	for _, tc := range tests {
		result := levenshteinDistance(tc.a, tc.b)
		if result != tc.expected {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d",
				tc.a, tc.b, result, tc.expected)
		}
	}
}

func TestMatchConfidenceString(t *testing.T) {
	tests := []struct {
		confidence MatchConfidence
		expected   string
	}{
		{MatchNone, "none"},
		{MatchWeak, "weak"},
		{MatchStrong, "strong"},
		{MatchDefinitive, "definitive"},
		{MatchConfidence(99), "unknown"},
	}

	for _, tc := range tests {
		result := tc.confidence.String()
		if result != tc.expected {
			t.Errorf("MatchConfidence(%d).String() = %q, want %q",
				tc.confidence, result, tc.expected)
		}
	}
}

func TestMatchConfidenceShouldAutoImport(t *testing.T) {
	tests := []struct {
		confidence MatchConfidence
		expected   bool
	}{
		{MatchNone, false},
		{MatchWeak, false},
		{MatchStrong, true},
		{MatchDefinitive, true},
	}

	for _, tc := range tests {
		result := tc.confidence.ShouldAutoImport()
		if result != tc.expected {
			t.Errorf("MatchConfidence(%d).ShouldAutoImport() = %v, want %v",
				tc.confidence, result, tc.expected)
		}
	}
}
