// Package util provides common utility functions.
package util

import (
	"regexp"
	"strings"
)

var (
	// Matches spaces, underscores, and slashes (for replacement with dashes).
	wordSeparatorRe = regexp.MustCompile(`[\s_/]+`)
	// Matches non-alphanumeric characters (except dashes).
	nonAlphanumericRe = regexp.MustCompile(`[^a-z0-9-]`)
	// Matches multiple consecutive dashes.
	multipleDashRe = regexp.MustCompile(`-+`)
)

// NormalizeTagSlug converts user input to a canonical tag slug.
// The slug is the source of truth for tag identity.
//
// Normalization rules:
//  1. Trim whitespace and lowercase
//  2. Replace spaces and underscores with dashes
//  3. Remove non-alphanumeric characters (except dashes)
//  4. Collapse multiple dashes
//  5. Trim leading/trailing dashes
//
// Examples:
//
//	"Slow Burn"     → "slow-burn"
//	"slow_burn"     → "slow-burn"
//	"SLOW-BURN"     → "slow-burn"
//	"🐉 Dragons!"   → "dragons"
//	"  multi   word " → "multi-word"
//	"--leading--"   → "leading"
func NormalizeTagSlug(input string) string {
	// 1. Trim and lowercase
	s := strings.ToLower(strings.TrimSpace(input))

	// 2. Replace word separators (spaces, underscores, slashes) with dashes
	s = wordSeparatorRe.ReplaceAllString(s, "-")

	// 3. Remove non-alphanumeric (except dashes)
	s = nonAlphanumericRe.ReplaceAllString(s, "")

	// 4. Collapse multiple dashes
	s = multipleDashRe.ReplaceAllString(s, "-")

	// 5. Trim leading/trailing dashes
	s = strings.Trim(s, "-")

	return s
}
