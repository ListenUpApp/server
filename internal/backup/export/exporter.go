package export

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"encoding/json/v2"

	"github.com/listenupapp/listenup-server/internal/backup/manifest"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Options configures backup creation.
type Options struct {
	IncludeImages bool
	IncludeEvents bool
	OutputPath    string
}

// Result contains the outcome of a backup operation.
type Result struct {
	Path     string
	Size     int64
	Counts   manifest.EntityCounts
	Duration time.Duration
	Checksum string
}

// Exporter creates backup archives.
type Exporter struct {
	store   store.Store
	dataDir string
	version string
}

// New creates an Exporter.
func New(s store.Store, dataDir, version string) *Exporter {
	return &Exporter{store: s, dataDir: dataDir, version: version}
}

// Export creates a backup archive.
//
//nolint:gocyclo // Sequential pipeline (manifest, entity steps, optional sections, finalize); flat is clearer.
func (e *Exporter) Export(ctx context.Context, opts Options) (*Result, error) {
	start := time.Now()

	// Write to temp file, rename on success (atomic)
	tmpPath := opts.OutputPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("create backup file: %w", err)
	}
	defer func() { _ = os.Remove(tmpPath) }() // Clean up on failure
	defer f.Close()

	// Tee to SHA-256 hasher
	hash := sha256.New()
	mw := io.MultiWriter(f, hash)
	zw := zip.NewWriter(mw)

	// Build manifest as we export
	m := &manifest.Manifest{
		Version:          manifest.FormatVersion,
		CreatedAt:        time.Now(),
		ListenUpVersion:  e.version,
		IncludesImages:   opts.IncludeImages,
		IncludesEvents:   opts.IncludeEvents,
		IncludesSettings: true, // Always include
	}

	// Export server identity + settings
	if err := exportServer(ctx, e.store, zw, m); err != nil {
		return nil, fmt.Errorf("export server: %w", err)
	}

	// Export entities in dependency order
	counts := &m.Counts

	exportSteps := []struct {
		name string
		fn   func(context.Context, store.Store, *zip.Writer) (int, error)
		dest *int
	}{
		{"users", exportUsers, &counts.Users},
		{"profiles", exportProfiles, nil}, // Not tracked in counts
		{"libraries", exportLibraries, &counts.Libraries},
		{"contributors", exportContributors, &counts.Contributors},
		{"series", exportSeries, &counts.Series},
		{"genres", exportGenres, &counts.Genres},
		{"tags", exportTags, &counts.Tags},
		{"books", exportBooks, &counts.Books},
		{"collections", exportCollections, &counts.Collections},
		{"collection_shares", exportCollectionShares, &counts.CollectionShares},
		{"shelves", exportShelves, &counts.Shelves},
		{"activities", exportActivities, &counts.Activities},
	}

	for _, step := range exportSteps {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		n, err := step.fn(ctx, e.store, zw)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", step.name, err)
		}
		if step.dest != nil {
			*step.dest = n
		}
	}

	// Listening data (optional but recommended)
	if opts.IncludeEvents {
		n, err := exportListeningEvents(ctx, e.store, zw)
		if err != nil {
			return nil, fmt.Errorf("export listening events: %w", err)
		}
		counts.ListeningEvents = n

		n, err = exportReadingSessions(ctx, e.store, zw)
		if err != nil {
			return nil, fmt.Errorf("export reading sessions: %w", err)
		}
		counts.ReadingSessions = n
	}

	// Images (optional, large)
	if opts.IncludeImages {
		n, err := e.exportImages(ctx, zw)
		if err != nil {
			return nil, fmt.Errorf("export images: %w", err)
		}
		counts.Images = n
	}

	// Write manifest last (has final counts)
	if err := e.writeManifest(zw, m); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	// Finalize zip
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, opts.OutputPath); err != nil {
		return nil, fmt.Errorf("rename backup: %w", err)
	}

	info, _ := os.Stat(opts.OutputPath)

	return &Result{
		Path:     opts.OutputPath,
		Size:     info.Size(),
		Counts:   *counts,
		Duration: time.Since(start),
		Checksum: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func (e *Exporter) writeManifest(zw *zip.Writer, m *manifest.Manifest) error {
	w, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	return json.MarshalWrite(w, m)
}
