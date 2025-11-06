package scanner

import (
	"log/slog"

	"context"
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
	// TODO: implement
	//
	// Requirements:
	// - Use filepath.WalkDir for efficiency
	// - Respect context cancellation
	// - Skip hidden files/directories (starting with .)
	// - Get inode information for each file
	// - Stream results via channel (don't buffer all in memory)
	// - Handle errors gracefully (log and continue)
	results := make(chan WalkResult)
	close(results)
	return results
}
