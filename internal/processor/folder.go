package processor

import (
	"path/filepath"
	"strings"
)

// determineBookFolder determines which book a file belongs to based on its path.
//
// The folder path is the source of truth for book identity. Files in the same
// folder belong to the same book.
//
// Special handling for multi-disc audiobooks:
// - Files in disc folders (CD1, CD2, Disc 1, etc.) are grouped under the parent folder
// - Example: /library/Author/Book/CD1/01.mp3 → /library/Author/Book
//
// This ensures that multi-disc audiobooks are treated as a single book.
func determineBookFolder(filePath string) string {
	dir := filepath.Dir(filePath)

	// Check if this is a disc folder (CD1, CD2, Disc 1, etc.)
	if isDiscDir(filepath.Base(dir)) {
		// Use parent folder as book identity
		// /library/Author/Book/CD1/01.mp3 → /library/Author/Book
		return filepath.Dir(dir)
	}

	// Regular folder
	// /library/Author/Book/01.mp3 → /library/Author/Book
	return dir
}

// isDiscDir checks if a directory name indicates a disc/CD directory.
//
// Matches patterns like:
// - CD1, CD2, CD 1, CD 01
// - Disc 1, Disc 2, Disc1, Disc 01
// - Disk 1, Disk 2, Disk1, Disk 01
// - Case insensitive
//
// This logic is reused from internal/scanner/grouper.go to ensure consistency
// in how multi-disc audiobooks are handled throughout the system.
func isDiscDir(name string) bool {
	name = strings.ToLower(name)

	// Match patterns like: CD1, CD 1, cd01, Disc 1, Disc1, etc.
	patterns := []string{
		"cd",
		"disc",
		"disk",
	}

	for _, pattern := range patterns {
		if strings.HasPrefix(name, pattern) {
			// Check if followed by space or number
			rest := strings.TrimPrefix(name, pattern)
			rest = strings.TrimSpace(rest)
			if len(rest) > 0 && (rest[0] >= '0' && rest[0] <= '9') {
				return true
			}
		}
	}

	return false
}
