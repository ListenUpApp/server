package api

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func (s *Server) handleCreateABSImport(ctx context.Context, input *CreateABSImportInput) (*CreateABSImportOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.BackupPath == "" {
		return nil, huma.Error400BadRequest("backup_path is required")
	}

	if _, err := os.Stat(input.Body.BackupPath); os.IsNotExist(err) {
		return nil, huma.Error400BadRequest("backup file not found")
	}

	// Create import record immediately with "analyzing" status and return
	now := time.Now()
	name := input.Body.Name
	if name == "" {
		name = "ABS Import " + now.Format("2006-01-02")
	}

	imp := &domain.ABSImport{
		ID:         uuid.New().String(),
		Name:       name,
		BackupPath: input.Body.BackupPath,
		Status:     domain.ABSImportStatusAnalyzing,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.store.CreateABSImport(ctx, imp); err != nil {
		return nil, huma.Error500InternalServerError("failed to create import", err)
	}

	// Launch background analysis on a tracked goroutine so it can be
	// canceled and drained on server shutdown (see importJobManager).
	s.importJobs.Submit(imp.ID, input.Body.BackupPath)

	return &CreateABSImportOutput{
		Body: toABSImportResponse(imp),
	}, nil
}

func (s *Server) handleListABSImports(ctx context.Context, _ *ListABSImportsInput) (*ListABSImportsOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	imports, err := s.store.ListABSImports(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list imports", err)
	}

	resp := &ListABSImportsOutput{}
	resp.Body.Imports = make([]ABSImportResponse, len(imports))
	for i, imp := range imports {
		resp.Body.Imports[i] = toABSImportResponse(imp)
	}

	return resp, nil
}

func (s *Server) handleGetABSImport(ctx context.Context, input *GetABSImportInput) (*GetABSImportOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	imp, err := s.store.GetABSImport(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("import not found")
	}

	return &GetABSImportOutput{
		Body: toABSImportResponse(imp),
	}, nil
}

func (s *Server) handleDeleteABSImport(ctx context.Context, input *DeleteABSImportInput) (*DeleteABSImportOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.store.DeleteABSImport(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete import", err)
	}

	return &DeleteABSImportOutput{
		Body: struct {
			Message string `json:"message" doc:"Success message"`
		}{Message: "Import deleted"},
	}, nil
}

// === Shared helpers ===

// updateImportStats refreshes the aggregate counts on an import record
// (users mapped, books mapped, sessions imported). Called after any mutation
// to user/book mappings or session imports.
func (s *Server) updateImportStats(ctx context.Context, importID string) {
	imp, err := s.store.GetABSImport(ctx, importID)
	if err != nil {
		s.logger.Error("updateImportStats: failed to get import", slog.String("import_id", importID), slog.String("error", err.Error()))
		return
	}

	users, err := s.store.ListABSImportUsers(ctx, importID, domain.MappingFilterAll)
	if err != nil {
		s.logger.Error("updateImportStats: failed to list users", slog.String("import_id", importID), slog.String("error", err.Error()))
		return
	}

	books, err := s.store.ListABSImportBooks(ctx, importID, domain.MappingFilterAll)
	if err != nil {
		s.logger.Error("updateImportStats: failed to list books", slog.String("import_id", importID), slog.String("error", err.Error()))
		return
	}

	sessions, err := s.store.ListABSImportSessions(ctx, importID, domain.SessionFilterImported)
	if err != nil {
		s.logger.Error("updateImportStats: failed to list sessions", slog.String("import_id", importID), slog.String("error", err.Error()))
		return
	}

	usersMapped := 0
	for _, u := range users {
		if u.IsMapped() {
			usersMapped++
		}
	}

	booksMapped := 0
	for _, b := range books {
		if b.IsMapped() {
			booksMapped++
		}
	}

	imp.UsersMapped = usersMapped
	imp.BooksMapped = booksMapped
	imp.SessionsImported = len(sessions)

	if err := s.store.UpdateABSImport(ctx, imp); err != nil {
		s.logger.Error("updateImportStats: failed to update import", slog.String("import_id", importID), slog.String("error", err.Error()))
	}
}

func toABSImportResponse(imp *domain.ABSImport) ABSImportResponse {
	return ABSImportResponse{
		ID:               imp.ID,
		Name:             imp.Name,
		BackupPath:       imp.BackupPath,
		Status:           string(imp.Status),
		CreatedAt:        imp.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        imp.UpdatedAt.Format(time.RFC3339),
		TotalUsers:       imp.TotalUsers,
		TotalBooks:       imp.TotalBooks,
		TotalSessions:    imp.TotalSessions,
		UsersMapped:      imp.UsersMapped,
		BooksMapped:      imp.BooksMapped,
		SessionsImported: imp.SessionsImported,
	}
}
