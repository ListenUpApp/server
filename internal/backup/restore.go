package backup

import (
	"archive/zip"
	"context"
	"fmt"
	"log/slog"

	"encoding/json/v2"

	backupimport "github.com/listenupapp/listenup-server/internal/backup/import"
	"github.com/listenupapp/listenup-server/internal/backup/stream"
	"github.com/listenupapp/listenup-server/internal/store"
)

// RestoreService restores from backups.
type RestoreService struct {
	store    *store.Store
	dataDir  string
	logger   *slog.Logger
	importer *backupimport.Importer
}

// NewRestoreService creates a RestoreService.
func NewRestoreService(s *store.Store, dataDir string, logger *slog.Logger) *RestoreService {
	return &RestoreService{
		store:    s,
		dataDir:  dataDir,
		logger:   logger,
		importer: backupimport.New(s, dataDir, logger),
	}
}

// Restore restores from a backup file.
func (s *RestoreService) Restore(ctx context.Context, path string, opts RestoreOptions) (*RestoreResult, error) {
	s.logger.Info("starting restore",
		"path", path,
		"mode", opts.Mode,
		"merge_strategy", opts.MergeStrategy,
		"dry_run", opts.DryRun)

	// Convert to import package types
	importOpts := backupimport.RestoreOptions{
		Mode:          backupimport.RestoreMode(opts.Mode),
		MergeStrategy: backupimport.MergeStrategy(opts.MergeStrategy),
		DryRun:        opts.DryRun,
	}

	importResult, err := s.importer.Import(ctx, path, importOpts)
	if err != nil {
		return nil, err
	}

	s.logger.Info("restore complete",
		"imported", importResult.Imported,
		"skipped", importResult.Skipped,
		"errors", len(importResult.Errors),
		"duration", importResult.Duration)

	// Convert result back to backup package types
	result := &RestoreResult{
		Imported: importResult.Imported,
		Skipped:  importResult.Skipped,
		Duration: importResult.Duration,
	}

	for _, e := range importResult.Errors {
		result.Errors = append(result.Errors, RestoreError{
			EntityType: e.EntityType,
			EntityID:   e.EntityID,
			Error:      e.Error,
		})
	}

	return result, nil
}

// Validate checks a backup without importing.
func (s *RestoreService) Validate(ctx context.Context, path string) (*ValidationResult, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return &ValidationResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("failed to open backup: %v", err)},
		}, nil
	}
	defer zr.Close()

	result := &ValidationResult{
		Valid: true,
	}

	// Check manifest
	rc, err := stream.OpenFile(zr, "manifest.json")
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, "missing manifest.json")
		return result, nil
	}

	var manifest Manifest
	if err := json.UnmarshalRead(rc, &manifest); err != nil {
		rc.Close()
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid manifest: %v", err))
		return result, nil
	}
	rc.Close()

	result.Manifest = &manifest
	result.ExpectedCounts = manifest.Counts

	// Check version
	if manifest.Version != FormatVersion {
		result.Valid = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("unsupported version %s (want %s)", manifest.Version, FormatVersion))
	}

	// Check required files exist
	requiredFiles := []string{
		"server.json",
		"entities/users.jsonl",
		"entities/libraries.jsonl",
	}

	for _, path := range requiredFiles {
		if _, err := stream.OpenFile(zr, path); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("missing file: %s", path))
		}
	}

	return result, nil
}

// RebuildProgress rebuilds all PlaybackProgress from ListeningEvents.
func (s *RestoreService) RebuildProgress(ctx context.Context) error {
	return backupimport.RebuildAllProgress(ctx, s.store, s.logger)
}
