package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/backup/export"
	"github.com/listenupapp/listenup-server/internal/store"
)

// BackupService manages backup creation and listing.
type BackupService struct {
	store     store.Store
	backupDir string
	dataDir   string
	version   string
	logger    *slog.Logger
	exporter  *export.Exporter
}

// NewBackupService creates a BackupService.
func NewBackupService(s store.Store, backupDir, dataDir, version string, logger *slog.Logger) *BackupService {
	return &BackupService{
		store:     s,
		backupDir: backupDir,
		dataDir:   dataDir,
		version:   version,
		logger:    logger,
		exporter:  export.New(s, dataDir, version),
	}
}

// Create creates a new backup.
func (s *BackupService) Create(ctx context.Context, opts BackupOptions) (*BackupResult, error) {
	// Ensure backup directory exists
	if err := os.MkdirAll(s.backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	// Generate output path if not specified
	outputPath := opts.OutputPath
	if outputPath == "" {
		timestamp := time.Now().Format("2006-01-02-150405")
		outputPath = filepath.Join(s.backupDir, fmt.Sprintf("backup-%s.listenup.zip", timestamp))
	}

	s.logger.Info("creating backup",
		"output", outputPath,
		"include_images", opts.IncludeImages,
		"include_events", opts.IncludeEvents)

	// Convert to export options
	exportOpts := export.Options{
		IncludeImages: opts.IncludeImages,
		IncludeEvents: opts.IncludeEvents,
		OutputPath:    outputPath,
	}

	result, err := s.exporter.Export(ctx, exportOpts)
	if err != nil {
		return nil, err
	}

	s.logger.Info("backup complete",
		"path", result.Path,
		"size", result.Size,
		"duration", result.Duration,
		"checksum", result.Checksum)

	// Convert result
	return &BackupResult{
		Path:     result.Path,
		Size:     result.Size,
		Counts:   convertCounts(result.Counts),
		Duration: result.Duration,
		Checksum: result.Checksum,
	}, nil
}

// convertCounts converts export.EntityCounts to backup.EntityCounts.
func convertCounts(c export.EntityCounts) EntityCounts {
	return EntityCounts{
		Users:            c.Users,
		Libraries:        c.Libraries,
		Books:            c.Books,
		Contributors:     c.Contributors,
		Series:           c.Series,
		Genres:           c.Genres,
		Tags:             c.Tags,
		Collections:      c.Collections,
		CollectionShares: c.CollectionShares,
		Shelves:          c.Shelves,
		Activities:       c.Activities,
		ListeningEvents:  c.ListeningEvents,
		ReadingSessions:  c.ReadingSessions,
		Images:           c.Images,
	}
}

// List returns all available backups.
func (s *BackupService) List(ctx context.Context) ([]BackupInfo, error) {
	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".listenup.zip") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			ID:        strings.TrimSuffix(entry.Name(), ".listenup.zip"),
			Path:      filepath.Join(s.backupDir, entry.Name()),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}

	// Sort by creation time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// Get returns a backup by ID.
func (s *BackupService) Get(ctx context.Context, id string) (*BackupInfo, error) {
	path := filepath.Join(s.backupDir, id+".listenup.zip")

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBackupNotFound
		}
		return nil, err
	}

	return &BackupInfo{
		ID:        id,
		Path:      path,
		Size:      info.Size(),
		CreatedAt: info.ModTime(),
	}, nil
}

// Delete removes a backup.
func (s *BackupService) Delete(ctx context.Context, id string) error {
	path := filepath.Join(s.backupDir, id+".listenup.zip")

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return ErrBackupNotFound
		}
		return err
	}

	return os.Remove(path)
}

// GetPath returns the file path for a backup ID.
func (s *BackupService) GetPath(id string) string {
	return filepath.Join(s.backupDir, id+".listenup.zip")
}

// GetUploadsDir returns the directory for uploaded backup files (e.g., ABS imports).
// Creates the directory if it doesn't exist.
func (s *BackupService) GetUploadsDir() (string, error) {
	uploadsDir := filepath.Join(s.backupDir, "uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		return "", fmt.Errorf("create uploads dir: %w", err)
	}
	return uploadsDir, nil
}
