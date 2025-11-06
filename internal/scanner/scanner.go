package scanner

import (
	"log/slog"

	"context"

	"github.com/listenupapp/listenup-server/internal/store"
)

// Scanner orchestrates the library scanning process
type Scanner struct {
	store  *store.Store
	logger *slog.Logger

	walker   *Walker
	grouper  *Grouper
	analyzer *Analyzer
	differ   *Differ
}

// NewScanner creates a new scanner instance
func NewScanner(store *store.Store, logger *slog.Logger) *Scanner {
	return &Scanner{
		store:    store,
		logger:   logger,
		walker:   NewWalker(logger),
		grouper:  NewGrouper(logger),
		analyzer: NewAnalyzer(logger),
		differ:   NewDiffer(logger),
	}
}

// ScanOptions configures a scan
type ScanOptions struct {
	Force      bool
	DryRun     bool
	Workers    int
	OnProgress func(*Progress)
}

func (s *Scanner) Scan(ctx context.Context, folderPath string, opts ScanOptions) (*ScanResult, error) {
	// TODO: implement
	//
	// Flow:
	// 1. Walk filesystem -> stream of files
	// 2. Group files into library items
	// 3. Analyze audio files (concurrent)
	// 4. Resolve metadata from multiple sources
	// 5. Compute diff against database
	// 6. Apply changes (if not dry run)
	// 7. Return result

	return nil, nil
}
