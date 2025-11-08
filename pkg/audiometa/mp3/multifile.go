package mp3

import (
	"cmp"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
)

// filenamePattern represents a pattern for extracting chapter info from filenames
type filenamePattern struct {
	regex *regexp.Regexp
	// Capture groups: 1 = chapter number, 2 = title (optional)
}

var filenamePatterns = []filenamePattern{
	{regexp.MustCompile(`^(\d+)\s*[-._]\s*(.+)\.mp3$`)},               // "01 - Title.mp3"
	{regexp.MustCompile(`^[Cc]hapter\s*(\d+)\s*[-._]?\s*(.*)\.mp3$`)}, // "Chapter 02 - Title.mp3"
	{regexp.MustCompile(`^[Tt]rack\s*(\d+)\s*[-._]?\s*(.*)\.mp3$`)},   // "Track03.mp3"
	{regexp.MustCompile(`^[Pp]art\s*(\d+)\s*[-._]?\s*(.*)\.mp3$`)},    // "Part 1.mp3"
	{regexp.MustCompile(`^(\d+)\s+(.+)\.mp3$`)},                       // "01 Title.mp3"
	{regexp.MustCompile(`^(\d+)\.mp3$`)},                              // "03.mp3"
}

// fileMetadata represents metadata for a single file in a multi-file audio collection
type fileMetadata struct {
	Path     string
	Metadata *audiometa.Metadata
	Index    int // Derived from track number or filename
}

// ParseMultiFile parses multiple MP3 files as a single audio collection (album, audiobook, etc.)
// Aggregates metadata and creates chapters from individual files
func ParseMultiFile(paths []string) (*audiometa.Metadata, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files provided")
	}

	// Parse each file
	files := make([]fileMetadata, 0, len(paths))
	for _, path := range paths {
		meta, err := Parse(path)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}

		// Extract index from track number or filename
		index := meta.TrackNumber
		if index == 0 {
			// Try to extract from filename
			index = extractChapterIndex(filepath.Base(path))
		}

		files = append(files, fileMetadata{
			Path:     path,
			Metadata: meta,
			Index:    index,
		})
	}

	// Sort files by index (or alphabetically if no index)
	sortFiles(files)

	// Aggregate metadata
	aggregated := aggregateMetadata(files)

	// Create chapters from files
	aggregated.Chapters = createChaptersFromFiles(files)

	return aggregated, nil
}

// sortFiles sorts files by index, falling back to natural filename sort
func sortFiles(files []fileMetadata) {
	slices.SortFunc(files, func(a, b fileMetadata) int {
		// If both have indices, use them
		if a.Index > 0 && b.Index > 0 {
			return cmp.Compare(a.Index, b.Index)
		}

		// If only one has an index, it comes first
		if a.Index > 0 {
			return -1
		}
		if b.Index > 0 {
			return 1
		}

		// Both have no index - use natural string sort
		if naturalLess(filepath.Base(a.Path), filepath.Base(b.Path)) {
			return -1
		}
		return 1
	})

	// Renumber indices sequentially
	for i := range files {
		files[i].Index = i + 1
	}
}

// aggregateMetadata merges metadata from multiple files
// Strategy: first file wins for most fields, sum for duration/size
func aggregateMetadata(files []fileMetadata) *audiometa.Metadata {
	if len(files) == 0 {
		return &audiometa.Metadata{
			Format: audiometa.FormatMP3,
		}
	}

	// Start with first file's metadata
	first := files[0].Metadata
	aggregated := &audiometa.Metadata{
		Format:     audiometa.FormatMP3,
		Title:      first.Title,
		Artist:     first.Artist,
		Album:      first.Album,
		Year:       first.Year,
		Genre:      first.Genre,
		Composer:   first.Composer,
		Narrator:   first.Narrator,
		Publisher:  first.Publisher,
		Series:     first.Series,
		SeriesPart: first.SeriesPart,
		ISBN:       first.ISBN,
		ASIN:       first.ASIN,
		Comment:    first.Comment,

		// Technical info from first file
		BitRate:    first.BitRate,
		SampleRate: first.SampleRate,
		Channels:   first.Channels,
		Codec:      "MP3",

		// Track info
		TrackTotal: len(files),
	}

	// Check for metadata inconsistencies
	inconsistentFields := make(map[string][]string)

	for i, file := range files {
		meta := file.Metadata

		// Sum duration and file size
		aggregated.Duration += meta.Duration
		aggregated.FileSize += meta.FileSize

		// Check for inconsistencies (skip first file)
		if i > 0 {
			checkInconsistency(inconsistentFields, "title", first.Title, meta.Title)
			checkInconsistency(inconsistentFields, "artist", first.Artist, meta.Artist)
			checkInconsistency(inconsistentFields, "album", first.Album, meta.Album)
			checkInconsistency(inconsistentFields, "narrator", first.Narrator, meta.Narrator)
			checkInconsistency(inconsistentFields, "series", first.Series, meta.Series)
		}
	}

	// Add warnings for inconsistencies
	for field, values := range inconsistentFields {
		aggregated.AddWarning("inconsistent %s across files: using '%s' from first file (found: %v)",
			field, values[0], values)
	}

	return aggregated
}

// createChaptersFromFiles creates chapters from individual files
func createChaptersFromFiles(files []fileMetadata) []audiometa.Chapter {
	chapters := make([]audiometa.Chapter, len(files))
	currentTime := time.Duration(0)

	for i, file := range files {
		// Extract title from filename
		title := extractChapterTitle(filepath.Base(file.Path))
		if title == "" {
			// Fall back to file's title or filename
			if file.Metadata.Title != "" {
				title = file.Metadata.Title
			} else {
				title = filepath.Base(file.Path)
			}
		}

		chapters[i] = audiometa.Chapter{
			Index:     i + 1,
			Title:     title,
			StartTime: currentTime,
			EndTime:   currentTime + file.Metadata.Duration,
		}

		currentTime += file.Metadata.Duration
	}

	return chapters
}

// extractChapterIndex extracts chapter number from filename
func extractChapterIndex(filename string) int {
	for _, pattern := range filenamePatterns {
		matches := pattern.regex.FindStringSubmatch(filename)
		if len(matches) >= 2 {
			index, err := strconv.Atoi(matches[1])
			if err == nil {
				return index
			}
		}
	}
	return 0
}

// extractChapterTitle extracts chapter title from filename
func extractChapterTitle(filename string) string {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	for _, pattern := range filenamePatterns {
		matches := pattern.regex.FindStringSubmatch(filename)
		if len(matches) >= 3 && matches[2] != "" {
			// Title captured
			return cleanTitle(matches[2])
		} else if len(matches) >= 2 {
			// Only number captured - use default title
			return fmt.Sprintf("Chapter %s", matches[1])
		}
	}

	// No pattern matched - use filename as-is
	return name
}

// cleanTitle cleans up extracted titles
func cleanTitle(title string) string {
	// Trim whitespace
	title = strings.TrimSpace(title)

	// Remove common prefixes/suffixes
	title = strings.TrimPrefix(title, "-")
	title = strings.TrimPrefix(title, "_")
	title = strings.TrimSpace(title)

	return title
}

// checkInconsistency checks if a field is inconsistent and tracks unique values
func checkInconsistency(inconsistencies map[string][]string, field, first, current string) {
	if first == "" || current == "" {
		return // Ignore empty values
	}

	if first != current {
		if _, exists := inconsistencies[field]; !exists {
			inconsistencies[field] = []string{first, current}
		} else {
			// Add if not already present
			found := false
			for _, v := range inconsistencies[field] {
				if v == current {
					found = true
					break
				}
			}
			if !found {
				inconsistencies[field] = append(inconsistencies[field], current)
			}
		}
	}
}

// naturalLess implements natural string comparison
// "file2.mp3" < "file10.mp3" (not "file10.mp3" < "file2.mp3" like alphabetical)
func naturalLess(a, b string) bool {
	// Compare character by character, treating consecutive digits as numbers
	i, j := 0, 0

	for i < len(a) && j < len(b) {
		aIsDigit := a[i] >= '0' && a[i] <= '9'
		bIsDigit := b[j] >= '0' && b[j] <= '9'

		if aIsDigit && bIsDigit {
			// Both are digits - extract and compare numbers
			aNum, aNext := extractNumberAt(a, i)
			bNum, bNext := extractNumberAt(b, j)

			if aNum != bNum {
				return aNum < bNum
			}

			i = aNext
			j = bNext
		} else if aIsDigit {
			// Numbers come before non-numbers
			return true
		} else if bIsDigit {
			return false
		} else {
			// Both are non-digits - compare characters
			if a[i] != b[j] {
				return a[i] < b[j]
			}
			i++
			j++
		}
	}

	// One string is a prefix of the other
	return len(a) < len(b)
}

// extractNumberAt extracts a number starting at position i
func extractNumberAt(s string, i int) (int, int) {
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	num, _ := strconv.Atoi(s[start:i])
	return num, i
}

// extractLeadingNumber extracts leading digits from a string
func extractLeadingNumber(s string) (int, string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}

	if i == 0 {
		return 0, s
	}

	num, _ := strconv.Atoi(s[:i])
	return num, s[i:]
}
