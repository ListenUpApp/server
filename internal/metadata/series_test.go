package metadata

import (
	"testing"

	"github.com/simonhull/audiometa"
)

func TestInferSeriesPosition_ExplicitMarker(t *testing.T) {
	file := &audiometa.File{}
	file.Tags.Series = "Epic Fantasy"
	file.Tags.SeriesPart = "2"

	result := InferSeriesPosition(file)
	if result != "2" {
		t.Errorf("Expected '2', got '%s'", result)
	}
}

func TestInferSeriesPosition_SmallTrackTotal(t *testing.T) {
	file := &audiometa.File{}
	file.Tags.Series = "Epic Fantasy"
	file.Tags.SeriesPart = "" // No explicit marker
	file.Tags.TrackNumber = 2
	file.Tags.TrackTotal = 5

	result := InferSeriesPosition(file)
	if result != "2" {
		t.Errorf("Expected '2' (inferred), got '%s'", result)
	}
}

func TestInferSeriesPosition_NoInference(t *testing.T) {
	file := &audiometa.File{}
	file.Tags.Series = "Epic Fantasy"
	file.Tags.SeriesPart = "" // No explicit marker
	file.Tags.TrackNumber = 1
	file.Tags.TrackTotal = 1

	result := InferSeriesPosition(file)
	if result != "" {
		t.Errorf("Expected empty (no inference), got '%s'", result)
	}
}

func TestInferSeriesPosition_LargeTrackTotal(t *testing.T) {
	file := &audiometa.File{}
	file.Tags.Series = "Epic Fantasy"
	file.Tags.SeriesPart = "" // No explicit marker
	file.Tags.TrackNumber = 15
	file.Tags.TrackTotal = 69 // Likely chapters, not series

	result := InferSeriesPosition(file)
	if result != "" {
		t.Errorf("Expected empty (chapters, not series), got '%s'", result)
	}
}

func TestInferSeriesPosition_NoSeries(t *testing.T) {
	file := &audiometa.File{}
	file.Tags.Series = "" // No series tag
	file.Tags.SeriesPart = ""
	file.Tags.TrackNumber = 2
	file.Tags.TrackTotal = 5

	result := InferSeriesPosition(file)
	if result != "" {
		t.Errorf("Expected empty (no series tag), got '%s'", result)
	}
}

func TestIsLikelySeriesTrackNumber(t *testing.T) {
	tests := []struct {
		name       string
		trackNum   int
		trackTotal int
		expected   bool
	}{
		{"Track 1/1 - ambiguous", 1, 1, false},
		{"Trilogy book 2", 2, 3, true},
		{"5-book series", 3, 5, true},
		{"10-book series", 7, 10, true},
		{"Book 2 of 15 - likely series", 6, 15, true},
		{"Chapter 2 of 25 - likely chapters", 2, 25, false},
		{"Large series", 20, 41, false},
		{"Very large total", 50, 150, false},
		{"Invalid: zero track", 0, 5, false},
		{"Invalid: zero total", 3, 0, false},
		{"Invalid: track exceeds total", 10, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLikelySeriesTrackNumber(tt.trackNum, tt.trackTotal)
			if result != tt.expected {
				t.Errorf("isLikelySeriesTrackNumber(%d, %d) = %v, want %v",
					tt.trackNum, tt.trackTotal, result, tt.expected)
			}
		})
	}
}
