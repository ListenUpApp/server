// Package genre provides genre normalization, aliasing, and default taxonomy.
package genre

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	// Matches any non-alphanumeric character.
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)
	// Matches multiple hyphens.
	multipleHyphens = regexp.MustCompile(`-+`)
)

// Slugify converts a string to a URL-safe slug.
// "Science Fiction" -> "science-fiction".
// "LitRPG" -> "litrpg".
// "Sci-Fi/Fantasy" -> "sci-fi-fantasy".
func Slugify(s string) string {
	// Normalize unicode (decompose accented characters).
	s = norm.NFKD.String(s)

	// Remove non-ASCII characters.
	s = strings.Map(func(r rune) rune {
		if r > unicode.MaxASCII {
			return -1
		}
		return r
	}, s)

	// Lowercase.
	s = strings.ToLower(s)

	// Replace non-alphanumeric with hyphens.
	s = nonAlphanumeric.ReplaceAllString(s, "-")

	// Collapse multiple hyphens.
	s = multipleHyphens.ReplaceAllString(s, "-")

	// Trim leading/trailing hyphens.
	s = strings.Trim(s, "-")

	return s
}
