package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Walker traverses the filesystem and discovers files
type Walker struct {
	logger *slog.Logger
}

// NewWalker creates a new walker
func NewWalker(logger *slog.Logger) *Walker {
	return &Walker{
		logger: logger,
	}
}

// WalkResult represents a file discovered during walking
type WalkResult struct {
	Path    string
	RelPath string
	IsDir   bool
	Size    int64
	ModTime int64 // Unix Milliseconds
	Inode   uint64
	Error   error
}

// Walk traverses a directory and streams discovered files
// Returns a channel that will receive results
// Channel closes when walk is complete or context is cancelled
func (w *Walker) Walk(ctx context.Context, rootPath string) <-chan WalkResult {
	results := make(chan WalkResult, 100) // Buffered channel for better performance

	go func() {
		defer close(results)

		err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
			// Check context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Handle walk errors
			if err != nil {
				w.logger.Error("walk error", "path", path, "error", err)
				// Continue walking despite errors
				return nil
			}

			// Skip hidden files/directories
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip directories (we only want files)
			if d.IsDir() {
				return nil
			}

			// Get file info
			info, err := d.Info()
			if err != nil {
				w.logger.Error("failed to get file info", "path", path, "error", err)
				return nil
			}

			// Get inode
			inode, err := getInode(info)
			if err != nil {
				w.logger.Error("failed to get inode", "path", path, "error", err)
				// Continue even if we can't get inode
				inode = 0
			}

			// Compute relative path
			relPath, err := filepath.Rel(rootPath, path)
			if err != nil {
				w.logger.Error("failed to compute relative path", "path", path, "error", err)
				relPath = path
			}

			// Send result
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

// getInode extracts the inode number from file info
func getInode(info fs.FileInfo) (uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("failed to get stat")
	}
	return stat.Ino, nil
}
