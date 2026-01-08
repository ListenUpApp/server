package scanner

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Walker traverses the filesystem and discovers files.
type Walker struct {
	logger *slog.Logger
}

// NewWalker creates a new walker.
func NewWalker(logger *slog.Logger) *Walker {
	return &Walker{
		logger: logger,
	}
}

// WalkResult represents a file discovered during walking.
type WalkResult struct {
	Error   error
	Path    string
	RelPath string
	Size    int64
	ModTime int64
	Inode   uint64
	IsDir   bool
}

// Walk traverses a directory and streams discovered files.
// Returns a channel that will receive results.
// Channel closes when walk is complete or context is canceled.
func (w *Walker) Walk(ctx context.Context, rootPath string) <-chan WalkResult {
	results := make(chan WalkResult, 100) // Buffered channel for better performance

	go func() {
		defer close(results)

		err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
			// Check context cancellation.
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Handle walk errors.
			if err != nil {
				w.logger.Error("walk error", "path", path, "error", err)
				// Continue walking despite errors.
				return nil
			}

			// Skip hidden files/directories.
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip directories (we only want files).
			if d.IsDir() {
				return nil
			}

			// Get file info.
			info, err := d.Info()
			if err != nil {
				w.logger.Error("failed to get file info", "path", path, "error", err)
				return nil
			}

			// Get inode.
			inode, err := getInode(info)
			if err != nil {
				w.logger.Error("failed to get inode", "path", path, "error", err)
				// Continue even if we can't get inode.
				inode = 0
			}

			// Compute relative path.
			relPath, err := filepath.Rel(rootPath, path)
			if err != nil {
				w.logger.Error("failed to compute relative path", "path", path, "error", err)
				relPath = path
			}

			// Send result.
			result := WalkResult{
				Path:    path,
				RelPath: relPath,
				IsDir:   false,
				Size:    info.Size(),
				ModTime: info.ModTime().UnixMilli(),
				Inode:   inode,
			}

			select {
			case results <- result:
			case <-ctx.Done():
				return ctx.Err()
			}

			return nil
		})

		if err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Error("walk failed", "root", rootPath, "error", err)
		}
	}()

	return results
}

// WalkFolder walks a single folder non-recursively.
// Returns a channel that will receive results for files in just this folder.
func (w *Walker) WalkFolder(ctx context.Context, folderPath string) <-chan WalkResult {
	results := make(chan WalkResult, 100)

	go func() {
		defer close(results)

		// Check if folder contains disc subdirectories (CD1, CD2, etc.).
		// If so, we need to include those as well.
		entries, err := os.ReadDir(folderPath)
		if err != nil {
			w.logger.Error("failed to read directory", "path", folderPath, "error", err)
			return
		}

		// Collect all directories to scan (main folder + disc folders if any).
		dirsToScan := []string{folderPath}

		// Check for disc subdirectories.
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			// Check context cancellation.
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Skip hidden directories.
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			// Check if this is a disc directory using the grouper's IsDiscDir function.
			// Note: We can't import grouper here due to circular dependency,.
			// but since both are in the same package, we'll use a local helper.
			if isDiscDirLocal(entry.Name()) {
				discPath := filepath.Join(folderPath, entry.Name())
				dirsToScan = append(dirsToScan, discPath)
			}
		}

		// Walk each directory (non-recursively).
		for _, dir := range dirsToScan {
			// Check context cancellation.
			select {
			case <-ctx.Done():
				return
			default:
			}

			dirEntries, err := os.ReadDir(dir)
			if err != nil {
				w.logger.Error("failed to read directory", "path", dir, "error", err)
				continue
			}

			for _, entry := range dirEntries {
				// Check context cancellation.
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Skip directories.
				if entry.IsDir() {
					continue
				}

				// Skip hidden files.
				if strings.HasPrefix(entry.Name(), ".") {
					continue
				}

				path := filepath.Join(dir, entry.Name())

				// Get file info.
				info, err := entry.Info()
				if err != nil {
					w.logger.Error("failed to get file info", "path", path, "error", err)
					continue
				}

				// Get inode.
				inode, err := getInode(info)
				if err != nil {
					w.logger.Error("failed to get inode", "path", path, "error", err)
					inode = 0
				}

				// Compute relative path from the original folder (not disc subfolder).
				relPath, err := filepath.Rel(folderPath, path)
				if err != nil {
					w.logger.Error("failed to compute relative path", "path", path, "error", err)
					relPath = entry.Name()
				}

				// Send result.
				result := WalkResult{
					Path:    path,
					RelPath: relPath,
					IsDir:   false,
					Size:    info.Size(),
					ModTime: info.ModTime().UnixMilli(),
					Inode:   inode,
				}

				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return results
}

// isDiscDirLocal checks if a directory name indicates a disc/CD directory.
// This is a local helper that delegates to the shared implementation.
func isDiscDirLocal(name string) bool {
	name = strings.ToLower(name)

	patterns := []string{
		"cd",
		"disc",
		"disk",
	}

	for _, pattern := range patterns {
		if after, ok := strings.CutPrefix(name, pattern); ok {
			rest := after
			rest = strings.TrimSpace(rest)
			if rest != "" && (rest[0] >= '0' && rest[0] <= '9') {
				return true
			}
		}
	}

	return false
}

// getInode extracts the inode number from file info.
func getInode(info fs.FileInfo) (uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("failed to get stat")
	}
	return stat.Ino, nil
}
