package chapters

import (
	"testing"
)

func TestIsGenericName(t *testing.T) {
	generic := []string{
		"Chapter 1",
		"Chapter 12",
		"chapter 1",
		"CHAPTER 5",
		"Track 01",
		"Track 5",
		"Part 1",
		"Part One",
		"Part Two",
		"Part 3",
		"1",
		"01",
		"12",
		"1.",
		"01 -",
		"",
		"   ",
	}

	for _, name := range generic {
		if !IsGenericName(name) {
			t.Errorf("expected %q to be generic", name)
		}
	}

	notGeneric := []string{
		"The Boy Who Lived",
		"Prologue",
		"Epilogue",
		"Introduction",
		"Chapter One: The Beginning",
		"Opening Credits",
		"The End",
		"Foreword by Stephen King",
	}

	for _, name := range notGeneric {
		if IsGenericName(name) {
			t.Errorf("expected %q to NOT be generic", name)
		}
	}
}

func TestAnalyzeChapters(t *testing.T) {
	tests := []struct {
		name        string
		titles      []string
		wantGeneric int
		wantUpdate  bool
	}{
		{
			name:        "all generic",
			titles:      []string{"Chapter 1", "Chapter 2", "Chapter 3"},
			wantGeneric: 3,
			wantUpdate:  true,
		},
		{
			name:        "none generic",
			titles:      []string{"Prologue", "The Beginning", "The End"},
			wantGeneric: 0,
			wantUpdate:  false,
		},
		{
			name:        "mixed - over threshold",
			titles:      []string{"Chapter 1", "Chapter 2", "The Climax", "Chapter 4"},
			wantGeneric: 3,
			wantUpdate:  true,
		},
		{
			name:        "mixed - under threshold",
			titles:      []string{"Prologue", "The Beginning", "Chapter 3", "The End"},
			wantGeneric: 1,
			wantUpdate:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chapters := make([]Chapter, len(tt.titles))
			for i, title := range tt.titles {
				chapters[i] = Chapter{Title: title}
			}

			result := AnalyzeChapters(chapters)

			if result.GenericCount != tt.wantGeneric {
				t.Errorf("GenericCount = %d, want %d", result.GenericCount, tt.wantGeneric)
			}
			if result.NeedsUpdate != tt.wantUpdate {
				t.Errorf("NeedsUpdate = %v, want %v", result.NeedsUpdate, tt.wantUpdate)
			}
		})
	}
}
