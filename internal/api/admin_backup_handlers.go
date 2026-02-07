package api

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/backup"
)

func (s *Server) registerAdminBackupRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "createBackup",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/backups",
		Summary:     "Create backup",
		Description: "Creates a new backup archive (admin only)",
		Tags:        []string{"Admin", "Backup"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateBackup)

	huma.Register(s.api, huma.Operation{
		OperationID: "listBackups",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/backups",
		Summary:     "List backups",
		Description: "Lists all available backup files (admin only)",
		Tags:        []string{"Admin", "Backup"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListBackups)

	huma.Register(s.api, huma.Operation{
		OperationID: "getBackup",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/backups/{id}",
		Summary:     "Get backup details",
		Description: "Gets details of a specific backup (admin only)",
		Tags:        []string{"Admin", "Backup"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBackup)

	huma.Register(s.api, huma.Operation{
		OperationID: "downloadBackup",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/backups/{id}/download",
		Summary:     "Download backup",
		Description: "Downloads a backup archive (admin only)",
		Tags:        []string{"Admin", "Backup"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDownloadBackup)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteBackup",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/backups/{id}",
		Summary:     "Delete backup",
		Description: "Deletes a backup archive (admin only)",
		Tags:        []string{"Admin", "Backup"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteBackup)

	huma.Register(s.api, huma.Operation{
		OperationID: "validateBackup",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/backups/validate",
		Summary:     "Validate backup",
		Description: "Validates a backup file without restoring (admin only)",
		Tags:        []string{"Admin", "Backup"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleValidateBackup)

	huma.Register(s.api, huma.Operation{
		OperationID: "restore",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/restore",
		Summary:     "Restore from backup",
		Description: "Restores data from a backup archive (admin only)",
		Tags:        []string{"Admin", "Backup"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRestore)

	huma.Register(s.api, huma.Operation{
		OperationID: "rebuildProgress",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/rebuild-progress",
		Summary:     "Rebuild playback progress",
		Description: "Rebuilds all playback progress from listening events (admin only)",
		Tags:        []string{"Admin", "Backup"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRebuildProgress)
}

// === DTOs ===

// CreateBackupRequest is the request body for creating a backup.
type CreateBackupRequest struct {
	IncludeImages bool `json:"include_images,omitempty" doc:"Include cover images and avatars (increases backup size)"`
	IncludeEvents bool `json:"include_events,omitempty" default:"true" doc:"Include listening events (required for history)"`
}

// CreateBackupInput is the Huma input for creating a backup.
type CreateBackupInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateBackupRequest
}

// BackupResponse represents a backup in API responses.
type BackupResponse struct {
	ID        string `json:"id" doc:"Backup identifier"`
	Path      string `json:"path" doc:"Backup file path"`
	Size      int64  `json:"size" doc:"Backup file size in bytes"`
	CreatedAt string `json:"created_at" doc:"When the backup was created"`
	Checksum  string `json:"checksum,omitempty" doc:"SHA-256 checksum"`
}

// CreateBackupOutput is the Huma output for creating a backup.
type CreateBackupOutput struct {
	Body BackupResponse
}

// ListBackupsInput is the Huma input for listing backups.
type ListBackupsInput struct {
	Authorization string `header:"Authorization"`
}

// ListBackupsOutput is the Huma output for listing backups.
type ListBackupsOutput struct {
	Body []BackupResponse
}

// GetBackupInput is the Huma input for getting a backup.
type GetBackupInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Backup identifier"`
}

// GetBackupOutput is the Huma output for getting a backup.
type GetBackupOutput struct {
	Body BackupResponse
}

// DeleteBackupInput is the Huma input for deleting a backup.
type DeleteBackupInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Backup identifier"`
}

// DeleteBackupOutput is the Huma output for deleting a backup.
type DeleteBackupOutput struct {
	Body struct {
		Message string `json:"message" doc:"Success message"`
	}
}

// DownloadBackupInput is the Huma input for downloading a backup.
type DownloadBackupInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Backup identifier"`
}

// ValidateBackupRequest is the request body for validating a backup.
type ValidateBackupRequest struct {
	BackupID string `json:"backup_id" doc:"ID of backup to validate"`
}

// ValidateBackupInput is the Huma input for validating a backup.
type ValidateBackupInput struct {
	Authorization string `header:"Authorization"`
	Body          ValidateBackupRequest
}

// ValidationResponse is the API response for backup validation.
type ValidationResponse struct {
	Valid          bool           `json:"valid" doc:"Whether the backup is valid"`
	Version        string         `json:"version,omitempty" doc:"Backup format version"`
	ServerID       string         `json:"server_id,omitempty" doc:"Source server ID"`
	ServerName     string         `json:"server_name,omitempty" doc:"Source server name"`
	ExpectedCounts map[string]int `json:"expected_counts,omitempty" doc:"Expected entity counts"`
	Errors         []string       `json:"errors,omitempty" doc:"Validation errors"`
	Warnings       []string       `json:"warnings,omitempty" doc:"Validation warnings"`
}

// ValidateBackupOutput is the Huma output for validating a backup.
type ValidateBackupOutput struct {
	Body ValidationResponse
}

// RestoreRequest is the request body for restoring from backup.
type RestoreRequest struct {
	BackupID        string `json:"backup_id" doc:"ID of backup to restore from"`
	Mode            string `json:"mode" doc:"Restore mode: full, merge, or events_only" enum:"full,merge,events_only"`
	MergeStrategy   string `json:"merge_strategy,omitempty" doc:"Conflict resolution: keep_local, keep_backup, or newest" enum:"keep_local,keep_backup,newest"`
	DryRun          bool   `json:"dry_run,omitempty" doc:"Validate without actually restoring"`
	ConfirmFullWipe bool   `json:"confirm_full_wipe,omitempty" doc:"Required for full mode to confirm data wipe"`
}

// RestoreInput is the Huma input for restoring from backup.
type RestoreInput struct {
	Authorization string `header:"Authorization"`
	Body          RestoreRequest
}

// RestoreResponse is the API response for restore operations.
type RestoreResponse struct {
	Imported map[string]int        `json:"imported" doc:"Entities imported by type"`
	Skipped  map[string]int        `json:"skipped" doc:"Entities skipped by type"`
	Errors   []backup.RestoreError `json:"errors,omitempty" doc:"Non-fatal errors during restore"`
	Duration string                `json:"duration" doc:"Total restore duration"`
}

// RestoreOutput is the Huma output for restore operations.
type RestoreOutput struct {
	Body RestoreResponse
}

// RebuildProgressInput is the Huma input for rebuilding progress.
type RebuildProgressInput struct {
	Authorization string `header:"Authorization"`
}

// RebuildProgressOutput is the Huma output for rebuilding progress.
type RebuildProgressOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// === Handlers ===

func (s *Server) handleCreateBackup(ctx context.Context, input *CreateBackupInput) (*CreateBackupOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	opts := backup.BackupOptions{
		IncludeImages: input.Body.IncludeImages,
		IncludeEvents: input.Body.IncludeEvents,
	}

	result, err := s.backupService.Create(ctx, opts)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create backup", err)
	}

	createdAt := ""
	if result.Counts.Books > 0 || result.Counts.Users > 0 {
		createdAt = time.Now().Format(time.RFC3339)
	}

	return &CreateBackupOutput{
		Body: BackupResponse{
			ID:        extractBackupID(result.Path),
			Path:      result.Path,
			Size:      result.Size,
			CreatedAt: createdAt,
			Checksum:  result.Checksum,
		},
	}, nil
}

func (s *Server) handleListBackups(ctx context.Context, _ *ListBackupsInput) (*ListBackupsOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	backups, err := s.backupService.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list backups", err)
	}

	response := make([]BackupResponse, len(backups))
	for i, b := range backups {
		response[i] = BackupResponse{
			ID:        b.ID,
			Path:      b.Path,
			Size:      b.Size,
			CreatedAt: b.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return &ListBackupsOutput{Body: response}, nil
}

func (s *Server) handleGetBackup(ctx context.Context, input *GetBackupInput) (*GetBackupOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	b, err := s.backupService.Get(ctx, input.ID)
	if err != nil {
		if err == backup.ErrBackupNotFound {
			return nil, huma.Error404NotFound("backup not found")
		}
		return nil, huma.Error500InternalServerError("failed to get backup", err)
	}

	return &GetBackupOutput{
		Body: BackupResponse{
			ID:        b.ID,
			Path:      b.Path,
			Size:      b.Size,
			CreatedAt: b.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
	}, nil
}

func (s *Server) handleDownloadBackup(ctx context.Context, input *DownloadBackupInput) (*huma.StreamResponse, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	b, err := s.backupService.Get(ctx, input.ID)
	if err != nil {
		if err == backup.ErrBackupNotFound {
			return nil, huma.Error404NotFound("backup not found")
		}
		return nil, huma.Error500InternalServerError("failed to get backup", err)
	}

	f, err := os.Open(b.Path)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to open backup file", err)
	}

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			ctx.SetHeader("Content-Type", "application/zip")
			ctx.SetHeader("Content-Disposition", "attachment; filename=\""+input.ID+".listenup.zip\"")
			io.Copy(ctx.BodyWriter(), f)
			f.Close()
		},
	}, nil
}

func (s *Server) handleDeleteBackup(ctx context.Context, input *DeleteBackupInput) (*DeleteBackupOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.backupService.Delete(ctx, input.ID); err != nil {
		if err == backup.ErrBackupNotFound {
			return nil, huma.Error404NotFound("backup not found")
		}
		return nil, huma.Error500InternalServerError("failed to delete backup", err)
	}

	return &DeleteBackupOutput{
		Body: struct {
			Message string `json:"message" doc:"Success message"`
		}{Message: "Backup deleted"},
	}, nil
}

func (s *Server) handleValidateBackup(ctx context.Context, input *ValidateBackupInput) (*ValidateBackupOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	path := s.backupService.GetPath(input.Body.BackupID)
	result, err := s.restoreService.Validate(ctx, path)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to validate backup", err)
	}

	resp := ValidationResponse{
		Valid:    result.Valid,
		Errors:   result.Errors,
		Warnings: result.Warnings,
	}

	if result.Manifest != nil {
		resp.Version = result.Manifest.Version
		resp.ServerID = result.Manifest.ServerID
		resp.ServerName = result.Manifest.ServerName
		resp.ExpectedCounts = map[string]int{
			"users":             result.ExpectedCounts.Users,
			"libraries":         result.ExpectedCounts.Libraries,
			"books":             result.ExpectedCounts.Books,
			"contributors":      result.ExpectedCounts.Contributors,
			"series":            result.ExpectedCounts.Series,
			"genres":            result.ExpectedCounts.Genres,
			"tags":              result.ExpectedCounts.Tags,
			"collections":       result.ExpectedCounts.Collections,
			"collection_shares": result.ExpectedCounts.CollectionShares,
			"shelves":            result.ExpectedCounts.Shelves,
			"activities":        result.ExpectedCounts.Activities,
			"listening_events":  result.ExpectedCounts.ListeningEvents,
			"reading_sessions":  result.ExpectedCounts.ReadingSessions,
		}
	}

	return &ValidateBackupOutput{Body: resp}, nil
}

func (s *Server) handleRestore(ctx context.Context, input *RestoreInput) (*RestoreOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	mode := backup.RestoreMode(input.Body.Mode)
	if !mode.Valid() {
		return nil, huma.Error400BadRequest("invalid restore mode")
	}

	// Require confirmation for full wipe
	if mode == backup.RestoreModeFull && !input.Body.ConfirmFullWipe {
		return nil, huma.Error400BadRequest("full restore requires confirm_full_wipe=true")
	}

	var strategy backup.MergeStrategy
	if mode == backup.RestoreModeMerge {
		strategy = backup.MergeStrategy(input.Body.MergeStrategy)
		if !strategy.Valid() || strategy == "" {
			return nil, huma.Error400BadRequest("merge mode requires a valid merge_strategy")
		}
	}

	opts := backup.RestoreOptions{
		Mode:          mode,
		MergeStrategy: strategy,
		DryRun:        input.Body.DryRun,
	}

	path := s.backupService.GetPath(input.Body.BackupID)
	result, err := s.restoreService.Restore(ctx, path, opts)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to restore backup", err)
	}

	return &RestoreOutput{
		Body: RestoreResponse{
			Imported: result.Imported,
			Skipped:  result.Skipped,
			Errors:   result.Errors,
			Duration: result.Duration.String(),
		},
	}, nil
}

func (s *Server) handleRebuildProgress(ctx context.Context, _ *RebuildProgressInput) (*RebuildProgressOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.restoreService.RebuildProgress(ctx); err != nil {
		return nil, huma.Error500InternalServerError("failed to rebuild progress", err)
	}

	return &RebuildProgressOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: "Progress rebuilt successfully from listening events",
		},
	}, nil
}

// extractBackupID extracts the backup ID from a path.
func extractBackupID(path string) string {
	// Path is like /path/to/backup-2026-01-09-120000.listenup.zip
	// We want: backup-2026-01-09-120000
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			path = path[i+1:]
			break
		}
	}
	// Remove .listenup.zip suffix
	if len(path) > 13 && path[len(path)-13:] == ".listenup.zip" {
		return path[:len(path)-13]
	}
	return path
}
