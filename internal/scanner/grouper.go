package scanner

import (
	"context"
	"log/slog"
)

type Grouper struct {
	logger *slog.Logger
}

func NewGrouper(logger *slog.Logger) *Grouper {
	return &Grouper{
		logger: logger,
	}
}

// GroupOptions configure grouping behavior in case we need it later.
type GroupOptions struct{}

func (g *Grouper) Group(ctx context.Context, files []WalkResult, opts GroupOptions) {
	grouped := make(map[string][]WalkResult)
	// TODO: implement
	//
	// Grouping rules:
	// Rule 1: Single audio file in root = standalone audiobook
	// Rule 2: Directory with audio files = one audiobook
	// Rule 3: Multi-disc handling (CD1/, CD2/, Disc 1/, etc.)
	// Rule 4: Nested author/book structure
	//
	// For podcasts:
	// - Each audio file in podcast folder is an episode
	// - Podcast folder itself is the item
	//
	// For books:
	// - All audio files in a directory belong to one book
	// - Handle multi-disc structure (CD1/, CD2/, etc.)
	return grouped
}
