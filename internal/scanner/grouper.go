package scanner

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
)

// Grouper groups audio files into logical audiobook items.
type Grouper struct {
	logger *slog.Logger
}

// NewGrouper creates a new Grouper instance.
func NewGrouper(logger *slog.Logger) *Grouper {
	return &Grouper{
		logger: logger,
	}
}

// GroupOptions configure grouping behavior in case we need it later.
type GroupOptions struct{}

// Group groups audio files by their parent directory.
func (g *Grouper) Group(_ context.Context, files []WalkResult, _ GroupOptions) map[string][]WalkResult {
	if len(files) == 0 {
		return make(map[string][]WalkResult)
	}

	// First pass: group by immediate parent directory.
	dirGroups := make(map[string][]WalkResult)

	for _, file := range files {
		// Get the directory containing this file.
		dir := filepath.Dir(file.Path)

		// If file is in root (no parent directory structure), use file path as key.
		if dir == "/" || dir == "." || !strings.Contains(file.RelPath, string(filepath.Separator)) {
			// Single file in root - use file path as group key.
			dirGroups[file.Path] = append(dirGroups[file.Path], file)
		} else {
			// File in directory - use directory as group key.
			dirGroups[dir] = append(dirGroups[dir], file)
		}
	}

	// Second pass: merge multi-disc directories.
	grouped := make(map[string][]WalkResult)
	processed := make(map[string]bool)

	for dir, fileList := range dirGroups {
		if processed[dir] {
			continue
		}

		// Check if this is a disc directory (CD1, CD2, Disc 1, etc.).
		dirName := filepath.Base(dir)
		if IsDiscDir(dirName) {
			// This is a disc directory - merge with parent.
			parentDir := filepath.Dir(dir)

			// Find all sibling disc directories.
			var allFiles []WalkResult
			for otherDir, otherFiles := range dirGroups {
				if otherDir == dir || filepath.Dir(otherDir) == parentDir && IsDiscDir(filepath.Base(otherDir)) {
					allFiles = append(allFiles, otherFiles...)
					processed[otherDir] = true
				}
			}

			// Also include files directly in parent directory.
			if parentFiles, exists := dirGroups[parentDir]; exists {
				allFiles = append(allFiles, parentFiles...)
				processed[parentDir] = true
			}

			grouped[parentDir] = allFiles
		} else {
			// Regular directory - use as-is.
			grouped[dir] = fileList
			processed[dir] = true
		}
	}

	return grouped
}

// IsDiscDir checks if a directory name indicates a disc/CD directory.
// Exported so it can be used by other packages (e.g., walker, processor).
func IsDiscDir(name string) bool {
	name = strings.ToLower(name)

	// Match patterns like: CD1, CD 1, cd01, Disc 1, Disc1, etc.
	patterns := []string{
		"cd",
		"disc",
		"disk",
	}

	for _, pattern := range patterns {
		if after, ok := strings.CutPrefix(name, pattern); ok {
			// Check if followed by space or number.
			rest := after
			rest = strings.TrimSpace(rest)
			if rest != "" && (rest[0] >= '0' && rest[0] <= '9') {
				return true
			}
		}
	}

	return false
}
