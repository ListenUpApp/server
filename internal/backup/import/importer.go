// Package backupimport provides backup import and restore functionality.
package backupimport

import (
	"archive/zip"
	"context"
	"fmt"
	"log/slog"
	"time"

	"encoding/json/v2"

	"github.com/listenupapp/listenup-server/internal/backup/stream"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Importer restores from backup archives.
type Importer struct {
	store   *store.Store
	dataDir string
	logger  *slog.Logger
}

// New creates an Importer.
func New(s *store.Store, dataDir string, logger *slog.Logger) *Importer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Importer{store: s, dataDir: dataDir, logger: logger}
}

// Import restores from a backup file.
func (i *Importer) Import(ctx context.Context, path string, opts RestoreOptions) (*RestoreResult, error) {
	start := time.Now()

	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open backup: %w", err)
	}
	defer zr.Close()

	// Read and validate manifest
	manifest, err := i.readManifest(zr)
	if err != nil {
		return nil, err
	}

	if err := i.checkVersion(manifest.Version); err != nil {
		return nil, err
	}

	result := &RestoreResult{
		Imported: make(map[string]int),
		Skipped:  make(map[string]int),
	}

	// Full mode: wipe existing data first
	if opts.Mode == RestoreModeFull && !opts.DryRun {
		if err := i.store.ClearAllData(ctx); err != nil {
			return nil, fmt.Errorf("clear existing data: %w", err)
		}
	}

	// Restore server settings (full mode only)
	if opts.Mode == RestoreModeFull && manifest.IncludesSettings && !opts.DryRun {
		if err := i.importServer(ctx, zr); err != nil {
			result.Errors = append(result.Errors, RestoreError{
				EntityType: "server",
				Error:      err.Error(),
			})
			// Continue - server settings are not critical
		}
	}

	// Events-only mode skips entity import
	if opts.Mode != RestoreModeEventsOnly {
		if err := i.importEntities(ctx, zr, opts, result); err != nil {
			return nil, err
		}
	}

	// Import listening data
	if manifest.IncludesEvents {
		if err := i.importListeningData(ctx, zr, opts, result); err != nil {
			return nil, err
		}

		// Rebuild progress from events
		if !opts.DryRun {
			if err := i.rebuildProgress(ctx); err != nil {
				result.Errors = append(result.Errors, RestoreError{
					EntityType: "progress",
					Error:      fmt.Sprintf("rebuild progress: %v", err),
				})
			}
		}
	}

	// Extract images
	if manifest.IncludesImages && !opts.DryRun {
		if n, err := i.importImages(ctx, zr); err != nil {
			result.Errors = append(result.Errors, RestoreError{
				EntityType: "images",
				Error:      err.Error(),
			})
		} else {
			result.Imported["images"] = n
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (i *Importer) readManifest(zr *zip.ReadCloser) (*Manifest, error) {
	rc, err := stream.OpenFile(zr, "manifest.json")
	if err != nil {
		return nil, ErrInvalidManifest
	}
	defer rc.Close()

	var manifest Manifest
	if err := json.UnmarshalRead(rc, &manifest); err != nil {
		return nil, ErrInvalidManifest
	}

	return &manifest, nil
}

func (i *Importer) checkVersion(version string) error {
	// For now, only support exact version match
	// Future: add migration logic for older versions
	if version != FormatVersion {
		return fmt.Errorf("%w: got %s, want %s", ErrVersionMismatch, version, FormatVersion)
	}
	return nil
}

// importEntities imports all entities in dependency order.
func (i *Importer) importEntities(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions, result *RestoreResult) error {
	steps := []struct {
		name string
		fn   func(context.Context, *zip.ReadCloser, RestoreOptions) (imported, skipped int, errs []RestoreError)
	}{
		{"users", i.importUsers},
		{"profiles", i.importProfiles},
		{"libraries", i.importLibraries},
		{"contributors", i.importContributors},
		{"series", i.importSeries},
		{"genres", i.importGenres},
		{"tags", i.importTags},
		{"books", i.importBooks},
		{"collections", i.importCollections},
		{"collection_shares", i.importCollectionShares},
		{"shelves", i.importShelves},
		{"activities", i.importActivities},
	}

	for _, step := range steps {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		imported, skipped, errs := step.fn(ctx, zr, opts)
		result.Imported[step.name] = imported
		result.Skipped[step.name] = skipped
		result.Errors = append(result.Errors, errs...)

		i.logger.Info("imported entities",
			"type", step.name,
			"imported", imported,
			"skipped", skipped,
			"errors", len(errs))
	}

	return nil
}

func (i *Importer) importListeningData(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions, result *RestoreResult) error {
	// Import listening events
	imported, skipped, errs := i.importListeningEvents(ctx, zr, opts)
	result.Imported["listening_events"] = imported
	result.Skipped["listening_events"] = skipped
	result.Errors = append(result.Errors, errs...)

	i.logger.Info("imported listening events",
		"imported", imported,
		"skipped", skipped,
		"errors", len(errs))

	// Import reading sessions
	imported, skipped, errs = i.importReadingSessions(ctx, zr, opts)
	result.Imported["reading_sessions"] = imported
	result.Skipped["reading_sessions"] = skipped
	result.Errors = append(result.Errors, errs...)

	i.logger.Info("imported reading sessions",
		"imported", imported,
		"skipped", skipped,
		"errors", len(errs))

	return nil
}
