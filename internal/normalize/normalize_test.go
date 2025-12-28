package normalize

import "testing"

func TestLanguageCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// ISO 639-1 codes (passthrough)
		{"en", "en"},
		{"de", "de"},
		{"fr", "fr"},
		// ISO 639-2 codes
		{"eng", "en"},
		{"deu", "de"},
		{"ger", "de"}, // bibliographic variant
		// Locale codes
		{"en-US", "en"},
		{"en_GB", "en"},
		{"de-AT", "de"},
		// Language names
		{"english", "en"},
		{"English", "en"},
		{"ENGLISH", "en"},
		{"german", "de"},
		{"German", "de"},
		// Edge cases
		{"", ""},
		{"  en  ", "en"},
		{"xyz", ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := LanguageCode(tt.input)
			if result != tt.expected {
				t.Errorf("LanguageCode(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLanguage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// ISO codes to display names
		{"en", "English"},
		{"de", "German"},
		{"fr", "French"},
		{"ja", "Japanese"},
		{"zh", "Chinese"},
		// Names normalized
		{"english", "English"},
		{"GERMAN", "German"},
		{"  french  ", "French"},
		// ISO 639-2 codes
		{"eng", "English"},
		{"deu", "German"},
		// Locale codes
		{"en-US", "English"},
		{"de-AT", "German"},
		// Edge cases
		{"", ""},
		{"xyz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Language(tt.input)
			if result != tt.expected {
				t.Errorf("Language(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenreSlugs(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Science Fiction", []string{"science-fiction"}},
		{"Sci-Fi", []string{"science-fiction"}},
		{"Science Fiction & Fantasy", []string{"science-fiction", "fantasy"}},
		{"Mystery, Thriller & Suspense", []string{"mystery-thriller"}},
		{"Unknown Genre", []string{"unknown-genre"}}, // Falls back to slugified
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := GenreSlugs(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("GenreSlugs(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("GenreSlugs(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}
