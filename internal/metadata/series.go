// Package metadata provides utilities for extracting and inferring metadata from audiobook files.
package metadata

import (
	"strconv"

	"github.com/simonhull/audiometa"
)

// InferSeriesPosition attempts to determine series position for audiobooks.
// This implements application-specific heuristics that were previously in audiometa.
func InferSeriesPosition(file *audiometa.File) string {
	// First, check if audiometa found an explicit marker
	if file.Tags.SeriesPart != "" {
		return file.Tags.SeriesPart
	}

	// No explicit marker found. Should we infer from track numbers?
	// Only do this if we're confident it's an audiobook series

	// Application-specific heuristic: If it's in the audiobook library
	// and has a small track total, we can infer it's likely a series position
	if file.Tags.Series != "" && isLikelySeriesTrackNumber(file.Tags.TrackNumber, file.Tags.TrackTotal) {
		return strconv.Itoa(file.Tags.TrackNumber)
	}

	return ""
}

// isLikelySeriesTrackNumber determines if a track number likely represents series position.
// This is the heuristic that was removed from audiometa.
func isLikelySeriesTrackNumber(trackNum, trackTotal int) bool {
	// Invalid input
	if trackNum == 0 || trackTotal == 0 {
		return false
	}

	// Invalid track number (out of bounds)
	if trackNum > trackTotal {
		return false
	}

	// Special case: Track 1/1 is highly ambiguous
	// Most M4B files are single-file audiobooks with track 1/1, but this doesn't
	// indicate series position. Prefer to fall through to other methods (title, album, path)
	if trackNum == 1 && trackTotal == 1 {
		return false
	}

	// Heuristic 1: Very small totals (≤ 10) are likely series
	// Examples: 3-book trilogy, 5-book series
	if trackTotal <= 10 {
		return true
	}

	// Heuristic 2: Medium totals (11-30) could be either
	// Use ratio: if track number is low relative to total, likely chapters
	// Examples:
	// - Book 2 of 15 = likely series (ratio: 0.13)
	// - Chapter 2 of 25 = likely chapters (ratio: 0.08)
	if trackTotal <= 30 {
		ratio := float64(trackNum) / float64(trackTotal)
		// If we're past 1/3 of the total, more likely to be a series
		return ratio > 0.33 || trackNum == 1
	}

	// Heuristic 3: Large totals (31-100) - could be large series
	// Examples: Discworld (41 books), Horus Heresy (54+ books)
	// Use distribution heuristic: chapters are usually evenly distributed
	// while series books can have any position
	if trackTotal <= 100 {
		// If it's near the beginning or end, more likely chapters
		// If it's a middle position, check ratio
		if trackNum <= 3 || trackNum >= trackTotal-3 {
			// Near edges - ambiguous, default to false (safer)
			return false
		}
		// Middle positions with large totals are likely chapters
		return false
	}

	// Heuristic 4: Very large totals (> 100) are almost certainly chapters
	// No audiobook series has 100+ books in a single M4B with track metadata
	// (They would be separate files)
	return false
}
