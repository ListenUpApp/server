package scanner

import (
	"log/slog"

	"context"

	"github.com/listenupapp/listenup-server/internal/scanner/audio"
)

// Analyzer analyzes audio files and extracts metadata
type Analyzer struct {
	logger *slog.Logger
	parser audio.Parser
}

// NewAnalyzer creates a new analyzer
func NewAnalyzer(logger *slog.Logger) *Analyzer {
	return &Analyzer{
		logger: logger,
		parser: audio.NewFFprobeParser(),
	}
}

// AnalyzeOptions configures analysis behavior
type AnalyzeOptions struct {
	// Number of concurrent workers
	Workers int

	// Skip files that haven't changed (based on modtime/size
	UseCache bool
}

func (a *Analyzer) Analyze(ctx context.Context, files []AudioFileData, opts AnalyzeOptions) ([]AudioFileData, error) {
	// TODO: implement
	//
	// Requirements:
	// - Concurrent analysis using worker pool
	// - Respect context cancellation
	// - Use parser interface (allows swapping ffprobe -> native)
	// - Handle errors gracefully (skip file, log error)
	// - Optional caching based on file modtime/size
	// - Progress reporting via context
	return nil, nil
}
