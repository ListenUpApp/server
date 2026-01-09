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

	"github.com/listenupapp/listenup-server/internal/store"
)

// FormatVersion is the backup format version.
const FormatVersion = "1.0"

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
	Counts   EntityCounts
	Duration time.Duration
	Checksum string
}

// Manifest describes backup contents and metadata.
type Manifest struct {
	Version          string       `json:"version"`
	CreatedAt        time.Time    `json:"created_at"`
	ServerID         string       `json:"server_id"`
	ServerName       string       `json:"server_name"`
	ListenUpVersion  string       `json:"listenup_version"`
	Counts           EntityCounts `json:"counts"`
	IncludesImages   bool         `json:"includes_images"`
	IncludesEvents   bool         `json:"includes_events"`
	IncludesSettings bool         `json:"includes_settings"`
}

// EntityCounts tracks entity counts for validation and progress reporting.
type EntityCounts struct {
	Users            int `json:"users"`
	Libraries        int `json:"libraries"`
	Books            int `json:"books"`
	Contributors     int `json:"contributors"`
	Series           int `json:"series"`
	Genres           int `json:"genres"`
	Tags             int `json:"tags"`
	Collections      int `json:"collections"`
	CollectionShares int `json:"collection_shares"`
	Lenses           int `json:"lenses"`
	Activities       int `json:"activities"`
	ListeningEvents  int `json:"listening_events"`
	ReadingSessions  int `json:"reading_sessions"`
	Images           int `json:"images,omitempty"`
}

// Exporter creates backup archives.
type Exporter struct {
	store   *store.Store
	dataDir string
	version string
}

// New creates an Exporter.
func New(s *store.Store, dataDir, version string) *Exporter {
	return &Exporter{store: s, dataDir: dataDir, version: version}
}

// Export creates a backup archive.
func (e *Exporter) Export(ctx context.Context, opts Options) (*Result, error) {
	start := time.Now()

	// Write to temp file, rename on success (atomic)
	tmpPath := opts.OutputPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("create backup file: %w", err)
	}
	defer os.Remove(tmpPath) // Clean up on failure
	defer f.Close()

	// Tee to SHA-256 hasher
	hash := sha256.New()
	mw := io.MultiWriter(f, hash)
	zw := zip.NewWriter(mw)

	// Build manifest as we export
	manifest := &Manifest{
		Version:          FormatVersion,
		CreatedAt:        time.Now(),
		ListenUpVersion:  e.version,
		IncludesImages:   opts.IncludeImages,
		IncludesEvents:   opts.IncludeEvents,
		IncludesSettings: true, // Always include
	}

	// Export server identity + settings
	if err := exportServer(ctx, e.store, zw, manifest); err != nil {
		return nil, fmt.Errorf("export server: %w", err)
	}

	// Export entities in dependency order
	counts := &manifest.Counts

	exportSteps := []struct {
		name string
		fn   func(context.Context, *store.Store, *zip.Writer) (int, error)
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
		{"lenses", exportLenses, &counts.Lenses},
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
		if n, err := exportListeningEvents(ctx, e.store, zw); err != nil {
			return nil, fmt.Errorf("export listening events: %w", err)
		} else {
			counts.ListeningEvents = n
		}

		if n, err := exportReadingSessions(ctx, e.store, zw); err != nil {
			return nil, fmt.Errorf("export reading sessions: %w", err)
		} else {
			counts.ReadingSessions = n
		}
	}

	// Images (optional, large)
	if opts.IncludeImages {
		if n, err := e.exportImages(ctx, zw); err != nil {
			return nil, fmt.Errorf("export images: %w", err)
		} else {
			counts.Images = n
		}
	}

	// Write manifest last (has final counts)
	if err := e.writeManifest(zw, manifest); err != nil {
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

func (e *Exporter) writeManifest(zw *zip.Writer, m *Manifest) error {
	w, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	return json.MarshalWrite(w, m)
}
