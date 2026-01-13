package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/listenupapp/listenup-server/internal/backup/abs"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func (s *Server) registerAdminABSImportRoutes() {
	// Import management
	huma.Register(s.api, huma.Operation{
		OperationID: "createABSImport",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/abs/imports",
		Summary:     "Create ABS import",
		Description: "Creates a new persistent ABS import from an uploaded backup (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateABSImport)

	huma.Register(s.api, huma.Operation{
		OperationID: "listABSImports",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports",
		Summary:     "List ABS imports",
		Description: "Lists all ABS imports with status (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListABSImports)

	huma.Register(s.api, huma.Operation{
		OperationID: "getABSImport",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports/{id}",
		Summary:     "Get ABS import",
		Description: "Gets details of a single ABS import (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetABSImport)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteABSImport",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/abs/imports/{id}",
		Summary:     "Delete ABS import",
		Description: "Deletes an ABS import and all its data (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteABSImport)

	// User mapping
	huma.Register(s.api, huma.Operation{
		OperationID: "listABSImportUsers",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports/{id}/users",
		Summary:     "List ABS import users",
		Description: "Lists users in an ABS import with mapping status (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListABSImportUsers)

	huma.Register(s.api, huma.Operation{
		OperationID: "mapABSImportUser",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/abs/imports/{id}/users/{absUserId}",
		Summary:     "Map ABS user",
		Description: "Maps an ABS user to a ListenUp user (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMapABSImportUser)

	huma.Register(s.api, huma.Operation{
		OperationID: "clearABSImportUserMapping",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/abs/imports/{id}/users/{absUserId}",
		Summary:     "Clear ABS user mapping",
		Description: "Clears the mapping for an ABS user (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleClearABSImportUserMapping)

	// Book mapping
	huma.Register(s.api, huma.Operation{
		OperationID: "listABSImportBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports/{id}/books",
		Summary:     "List ABS import books",
		Description: "Lists books in an ABS import with mapping status (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListABSImportBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "mapABSImportBook",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/abs/imports/{id}/books/{absMediaId}",
		Summary:     "Map ABS book",
		Description: "Maps an ABS book to a ListenUp book (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMapABSImportBook)

	huma.Register(s.api, huma.Operation{
		OperationID: "clearABSImportBookMapping",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/abs/imports/{id}/books/{absMediaId}",
		Summary:     "Clear ABS book mapping",
		Description: "Clears the mapping for an ABS book (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleClearABSImportBookMapping)

	// Session management
	huma.Register(s.api, huma.Operation{
		OperationID: "listABSImportSessions",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports/{id}/sessions",
		Summary:     "List ABS import sessions",
		Description: "Lists sessions in an ABS import with status (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListABSImportSessions)

	huma.Register(s.api, huma.Operation{
		OperationID: "importABSSessions",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/abs/imports/{id}/sessions/import",
		Summary:     "Import ready sessions",
		Description: "Imports all ready sessions from an ABS import (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleImportABSSessions)

	huma.Register(s.api, huma.Operation{
		OperationID: "skipABSSession",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/abs/imports/{id}/sessions/{sessionId}/skip",
		Summary:     "Skip ABS session",
		Description: "Marks an ABS session as skipped (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSkipABSSession)
}

// === DTOs for persistent imports ===

// CreateABSImportRequest is the request for creating a persistent import.
type CreateABSImportRequest struct {
	BackupPath string `json:"backup_path" doc:"Path to uploaded .audiobookshelf backup file"`
	Name       string `json:"name,omitempty" doc:"Optional friendly name for this import"`
}

type CreateABSImportInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateABSImportRequest
}

// ABSImportResponse represents an ABS import.
type ABSImportResponse struct {
	ID               string `json:"id" doc:"Import ID"`
	Name             string `json:"name" doc:"Import name"`
	BackupPath       string `json:"backup_path" doc:"Path to the backup file"`
	Status           string `json:"status" doc:"Import status: active, completed, archived"`
	CreatedAt        string `json:"created_at" doc:"When the import was created"`
	UpdatedAt        string `json:"updated_at" doc:"When the import was last updated"`
	TotalUsers       int    `json:"total_users" doc:"Total users in backup"`
	TotalBooks       int    `json:"total_books" doc:"Total books in backup"`
	TotalSessions    int    `json:"total_sessions" doc:"Total sessions in backup"`
	UsersMapped      int    `json:"users_mapped" doc:"Users with confirmed mappings"`
	BooksMapped      int    `json:"books_mapped" doc:"Books with confirmed mappings"`
	SessionsImported int    `json:"sessions_imported" doc:"Sessions already imported"`
}

type CreateABSImportOutput struct {
	Body ABSImportResponse
}

type ListABSImportsInput struct {
	Authorization string `header:"Authorization"`
}

type ListABSImportsOutput struct {
	Body struct {
		Imports []ABSImportResponse `json:"imports" doc:"List of ABS imports"`
	}
}

type GetABSImportInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
}

type GetABSImportOutput struct {
	Body ABSImportResponse
}

type DeleteABSImportInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
}

type DeleteABSImportOutput struct {
	Body struct {
		Message string `json:"message" doc:"Success message"`
	}
}

// User mapping DTOs

type ListABSImportUsersInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	Filter        string `query:"filter" doc:"Filter: all, mapped, unmapped" default:"all"`
}

type ABSImportUserResponse struct {
	ABSUserID     string   `json:"abs_user_id" doc:"ABS user ID"`
	ABSUsername   string   `json:"abs_username" doc:"ABS username"`
	ABSEmail      string   `json:"abs_email,omitempty" doc:"ABS email"`
	ListenUpID    string   `json:"listenup_id,omitempty" doc:"Mapped ListenUp user ID"`
	SessionCount  int      `json:"session_count" doc:"Number of sessions for this user"`
	TotalListenMs int64    `json:"total_listen_ms" doc:"Total listening time in milliseconds"`
	Confidence    string   `json:"confidence" doc:"Match confidence"`
	MatchReason   string   `json:"match_reason,omitempty" doc:"Why matched"`
	Suggestions   []string `json:"suggestions,omitempty" doc:"Suggested ListenUp user IDs"`
	IsMapped      bool     `json:"is_mapped" doc:"Whether this user is mapped"`
}

type ListABSImportUsersOutput struct {
	Body struct {
		Users []ABSImportUserResponse `json:"users" doc:"List of ABS users"`
	}
}

type MapABSImportUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	ABSUserID     string `path:"absUserId" doc:"ABS user ID"`
	Body          struct {
		ListenUpID string `json:"listenup_id" doc:"ListenUp user ID to map to"`
	}
}

type MapABSImportUserOutput struct {
	Body ABSImportUserResponse
}

type ClearABSImportUserMappingInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	ABSUserID     string `path:"absUserId" doc:"ABS user ID"`
}

type ClearABSImportUserMappingOutput struct {
	Body ABSImportUserResponse
}

// Book mapping DTOs

type ListABSImportBooksInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	Filter        string `query:"filter" doc:"Filter: all, mapped, unmapped" default:"all"`
}

type ABSImportBookResponse struct {
	ABSMediaID    string   `json:"abs_media_id" doc:"ABS media ID"`
	ABSTitle      string   `json:"abs_title" doc:"ABS book title"`
	ABSAuthor     string   `json:"abs_author,omitempty" doc:"ABS author"`
	ABSDurationMs int64    `json:"abs_duration_ms" doc:"ABS duration in milliseconds"`
	ListenUpID    string   `json:"listenup_id,omitempty" doc:"Mapped ListenUp book ID"`
	SessionCount  int      `json:"session_count" doc:"Number of sessions for this book"`
	Confidence    string   `json:"confidence" doc:"Match confidence"`
	MatchReason   string   `json:"match_reason,omitempty" doc:"Why matched"`
	Suggestions   []string `json:"suggestions,omitempty" doc:"Suggested ListenUp book IDs"`
	IsMapped      bool     `json:"is_mapped" doc:"Whether this book is mapped"`
}

type ListABSImportBooksOutput struct {
	Body struct {
		Books []ABSImportBookResponse `json:"books" doc:"List of ABS books"`
	}
}

type MapABSImportBookInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	ABSMediaID    string `path:"absMediaId" doc:"ABS media ID"`
	Body          struct {
		ListenUpID string `json:"listenup_id" doc:"ListenUp book ID to map to"`
	}
}

type MapABSImportBookOutput struct {
	Body ABSImportBookResponse
}

type ClearABSImportBookMappingInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	ABSMediaID    string `path:"absMediaId" doc:"ABS media ID"`
}

type ClearABSImportBookMappingOutput struct {
	Body ABSImportBookResponse
}

// Session DTOs

type ListABSImportSessionsInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	Status        string `query:"status" doc:"Filter: all, pending, ready, imported, skipped" default:"all"`
}

type ABSImportSessionResponse struct {
	ABSSessionID  string `json:"abs_session_id" doc:"ABS session ID"`
	ABSUserID     string `json:"abs_user_id" doc:"ABS user ID"`
	ABSMediaID    string `json:"abs_media_id" doc:"ABS media ID"`
	StartTime     string `json:"start_time" doc:"Session start time"`
	Duration      int64  `json:"duration" doc:"Session duration in milliseconds"`
	StartPosition int64  `json:"start_position" doc:"Start position in milliseconds"`
	EndPosition   int64  `json:"end_position" doc:"End position in milliseconds"`
	Status        string `json:"status" doc:"Import status"`
	ImportedAt    string `json:"imported_at,omitempty" doc:"When imported"`
	SkipReason    string `json:"skip_reason,omitempty" doc:"Why skipped"`
}

type ListABSImportSessionsOutput struct {
	Body struct {
		Sessions []ABSImportSessionResponse `json:"sessions" doc:"List of sessions"`
		Summary  struct {
			Total    int `json:"total" doc:"Total sessions"`
			Pending  int `json:"pending" doc:"Pending sessions"`
			Ready    int `json:"ready" doc:"Ready to import"`
			Imported int `json:"imported" doc:"Already imported"`
			Skipped  int `json:"skipped" doc:"Skipped sessions"`
		} `json:"summary" doc:"Session counts by status"`
	}
}

type ImportABSSessionsInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
}

type ImportABSSessionsOutput struct {
	Body struct {
		SessionsImported int    `json:"sessions_imported" doc:"Sessions imported this batch"`
		EventsCreated    int    `json:"events_created" doc:"Listening events created"`
		Duration         string `json:"duration" doc:"Import duration"`
	}
}

type SkipABSSessionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	SessionID     string `path:"sessionId" doc:"ABS session ID"`
	Body          struct {
		Reason string `json:"reason,omitempty" doc:"Why skipping this session"`
	}
}

type SkipABSSessionOutput struct {
	Body ABSImportSessionResponse
}

// === Handlers ===

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

	// Parse the backup
	backup, err := abs.Parse(input.Body.BackupPath)
	if err != nil {
		return nil, huma.Error400BadRequest("failed to parse ABS backup: " + err.Error())
	}

	// Run initial analysis to get matches
	opts := abs.AnalysisOptions{
		MatchByEmail:    true,
		MatchByPath:     true,
		FuzzyMatchBooks: true,
		FuzzyThreshold:  0.85,
		UserMappings:    make(map[string]string),
		BookMappings:    make(map[string]string),
	}
	analyzer := abs.NewAnalyzer(s.store, s.logger, opts)
	analysis, err := analyzer.Analyze(ctx, backup)
	if err != nil {
		return nil, huma.Error500InternalServerError("analysis failed", err)
	}

	// Create the persistent import
	now := time.Now()
	name := input.Body.Name
	if name == "" {
		name = "ABS Import " + now.Format("2006-01-02")
	}

	imp := &domain.ABSImport{
		ID:            uuid.New().String(),
		Name:          name,
		BackupPath:    input.Body.BackupPath,
		Status:        domain.ABSImportStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		TotalUsers:    analysis.TotalUsers,
		TotalBooks:    analysis.TotalBooks,
		TotalSessions: analysis.TotalSessions,
	}

	if err := s.store.CreateABSImport(ctx, imp); err != nil {
		return nil, huma.Error500InternalServerError("failed to create import", err)
	}

	// Store all parsed users with analysis results
	usersMapped := 0
	for _, um := range analysis.UserMatches {
		user := &domain.ABSImportUser{
			ImportID:      imp.ID,
			ABSUserID:     um.ABSUser.ID,
			ABSUsername:   um.ABSUser.Username,
			ABSEmail:      um.ABSUser.Email,
			SessionCount:  len(um.ABSUser.Progress),
			TotalListenMs: calculateUserListenTime(um.ABSUser.Progress),
			Confidence:    um.Confidence.String(),
			MatchReason:   um.MatchReason,
			Suggestions:   extractUserSuggestionIDs(um.Suggestions),
		}

		// Apply auto-match if confidence is strong enough
		if um.Confidence.ShouldAutoImport() && um.ListenUpID != "" {
			user.ListenUpID = &um.ListenUpID
			now := time.Now()
			user.MappedAt = &now
			usersMapped++
		}

		if err := s.store.CreateABSImportUser(ctx, user); err != nil {
			s.logger.Warn("failed to store import user", slog.String("error", err.Error()))
		}
	}

	// Store all parsed books with analysis results
	booksMapped := 0
	for _, bm := range analysis.BookMatches {
		book := &domain.ABSImportBook{
			ImportID:      imp.ID,
			ABSMediaID:    bm.ABSItem.MediaID,
			ABSTitle:      bm.ABSItem.Media.Metadata.Title,
			ABSAuthor:     bm.ABSItem.Media.Metadata.PrimaryAuthor(),
			ABSDurationMs: bm.ABSItem.Media.DurationMs(),
			ABSASIN:       bm.ABSItem.Media.Metadata.ASIN,
			ABSISBN:       bm.ABSItem.Media.Metadata.ISBN,
			SessionCount:  countSessionsForBook(backup.Sessions, bm.ABSItem.MediaID),
			Confidence:    bm.Confidence.String(),
			MatchReason:   bm.MatchReason,
			Suggestions:   extractBookSuggestionIDs(bm.Suggestions),
		}

		// Apply auto-match if confidence is strong enough
		if bm.Confidence.ShouldAutoImport() && bm.ListenUpID != "" {
			book.ListenUpID = &bm.ListenUpID
			now := time.Now()
			book.MappedAt = &now
			booksMapped++
		}

		if err := s.store.CreateABSImportBook(ctx, book); err != nil {
			s.logger.Warn("failed to store import book", slog.String("error", err.Error()))
		}
	}

	// Store all sessions with initial status
	for _, session := range backup.Sessions {
		sess := &domain.ABSImportSession{
			ImportID:      imp.ID,
			ABSSessionID:  session.ID,
			ABSUserID:     session.UserID,
			ABSMediaID:    session.LibraryItemID, // This is actually mediaId
			StartTime:     session.StartedAtTime(),
			Duration:      session.DurationMs(),
			StartPosition: session.StartPositionMs(),
			EndPosition:   session.EndPositionMs(),
			Status:        domain.SessionStatusPendingUser, // Will be recalculated
		}

		if err := s.store.CreateABSImportSession(ctx, sess); err != nil {
			s.logger.Warn("failed to store import session", slog.String("error", err.Error()))
		}
	}

	// Recalculate session statuses based on mappings
	if err := s.store.RecalculateSessionStatuses(ctx, imp.ID); err != nil {
		s.logger.Warn("failed to recalculate session statuses", slog.String("error", err.Error()))
	}

	// Update import with mapped counts
	imp.UsersMapped = usersMapped
	imp.BooksMapped = booksMapped
	if err := s.store.UpdateABSImport(ctx, imp); err != nil {
		s.logger.Warn("failed to update import counts", slog.String("error", err.Error()))
	}

	return &CreateABSImportOutput{
		Body: toABSImportResponse(imp),
	}, nil
}

func (s *Server) handleListABSImports(ctx context.Context, input *ListABSImportsInput) (*ListABSImportsOutput, error) {
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

func (s *Server) handleListABSImportUsers(ctx context.Context, input *ListABSImportUsersInput) (*ListABSImportUsersOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	filter := domain.MappingFilter(input.Filter)
	if filter == "" {
		filter = domain.MappingFilterAll
	}

	users, err := s.store.ListABSImportUsers(ctx, input.ID, filter)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list users", err)
	}

	resp := &ListABSImportUsersOutput{}
	resp.Body.Users = make([]ABSImportUserResponse, len(users))
	for i, u := range users {
		resp.Body.Users[i] = toABSImportUserResponse(u)
	}

	return resp, nil
}

func (s *Server) handleMapABSImportUser(ctx context.Context, input *MapABSImportUserInput) (*MapABSImportUserOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.ListenUpID == "" {
		return nil, huma.Error400BadRequest("listenup_id is required")
	}

	// Verify ListenUp user exists
	_, err = s.store.GetUser(ctx, input.Body.ListenUpID)
	if err != nil {
		return nil, huma.Error400BadRequest("ListenUp user not found")
	}

	if err := s.store.UpdateABSImportUserMapping(ctx, input.ID, input.ABSUserID, &input.Body.ListenUpID); err != nil {
		return nil, huma.Error500InternalServerError("failed to update mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, input.ID); err != nil {
		s.logger.Warn("failed to recalculate sessions", slog.String("error", err.Error()))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	user, err := s.store.GetABSImportUser(ctx, input.ID, input.ABSUserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get user", err)
	}

	return &MapABSImportUserOutput{
		Body: toABSImportUserResponse(user),
	}, nil
}

func (s *Server) handleClearABSImportUserMapping(ctx context.Context, input *ClearABSImportUserMappingInput) (*ClearABSImportUserMappingOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.store.UpdateABSImportUserMapping(ctx, input.ID, input.ABSUserID, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to clear mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, input.ID); err != nil {
		s.logger.Warn("failed to recalculate sessions", slog.String("error", err.Error()))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	user, err := s.store.GetABSImportUser(ctx, input.ID, input.ABSUserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get user", err)
	}

	return &ClearABSImportUserMappingOutput{
		Body: toABSImportUserResponse(user),
	}, nil
}

func (s *Server) handleListABSImportBooks(ctx context.Context, input *ListABSImportBooksInput) (*ListABSImportBooksOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	filter := domain.MappingFilter(input.Filter)
	if filter == "" {
		filter = domain.MappingFilterAll
	}

	books, err := s.store.ListABSImportBooks(ctx, input.ID, filter)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list books", err)
	}

	resp := &ListABSImportBooksOutput{}
	resp.Body.Books = make([]ABSImportBookResponse, len(books))
	for i, b := range books {
		resp.Body.Books[i] = toABSImportBookResponse(b)
	}

	return resp, nil
}

func (s *Server) handleMapABSImportBook(ctx context.Context, input *MapABSImportBookInput) (*MapABSImportBookOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.ListenUpID == "" {
		return nil, huma.Error400BadRequest("listenup_id is required")
	}

	// Verify ListenUp book exists (pass empty userID for admin access)
	_, err = s.store.GetBook(ctx, input.Body.ListenUpID, "")
	if err != nil {
		return nil, huma.Error400BadRequest("ListenUp book not found")
	}

	if err := s.store.UpdateABSImportBookMapping(ctx, input.ID, input.ABSMediaID, &input.Body.ListenUpID); err != nil {
		return nil, huma.Error500InternalServerError("failed to update mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, input.ID); err != nil {
		s.logger.Warn("failed to recalculate sessions", slog.String("error", err.Error()))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	book, err := s.store.GetABSImportBook(ctx, input.ID, input.ABSMediaID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get book", err)
	}

	return &MapABSImportBookOutput{
		Body: toABSImportBookResponse(book),
	}, nil
}

func (s *Server) handleClearABSImportBookMapping(ctx context.Context, input *ClearABSImportBookMappingInput) (*ClearABSImportBookMappingOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.store.UpdateABSImportBookMapping(ctx, input.ID, input.ABSMediaID, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to clear mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, input.ID); err != nil {
		s.logger.Warn("failed to recalculate sessions", slog.String("error", err.Error()))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	book, err := s.store.GetABSImportBook(ctx, input.ID, input.ABSMediaID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get book", err)
	}

	return &ClearABSImportBookMappingOutput{
		Body: toABSImportBookResponse(book),
	}, nil
}

func (s *Server) handleListABSImportSessions(ctx context.Context, input *ListABSImportSessionsInput) (*ListABSImportSessionsOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	filter := domain.SessionStatusFilter(input.Status)
	if filter == "" {
		filter = domain.SessionFilterAll
	}

	sessions, err := s.store.ListABSImportSessions(ctx, input.ID, filter)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list sessions", err)
	}

	// Also get all sessions for summary
	allSessions, _ := s.store.ListABSImportSessions(ctx, input.ID, domain.SessionFilterAll)

	resp := &ListABSImportSessionsOutput{}
	resp.Body.Sessions = make([]ABSImportSessionResponse, len(sessions))
	for i, sess := range sessions {
		resp.Body.Sessions[i] = toABSImportSessionResponse(sess)
	}

	// Calculate summary
	for _, sess := range allSessions {
		resp.Body.Summary.Total++
		switch sess.Status {
		case domain.SessionStatusPendingUser, domain.SessionStatusPendingBook:
			resp.Body.Summary.Pending++
		case domain.SessionStatusReady:
			resp.Body.Summary.Ready++
		case domain.SessionStatusImported:
			resp.Body.Summary.Imported++
		case domain.SessionStatusSkipped:
			resp.Body.Summary.Skipped++
		}
	}

	return resp, nil
}

func (s *Server) handleImportABSSessions(ctx context.Context, input *ImportABSSessionsInput) (*ImportABSSessionsOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	// Get ready sessions
	readySessions, err := s.store.ListABSImportSessions(ctx, input.ID, domain.SessionFilterReady)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get sessions", err)
	}

	if len(readySessions) == 0 {
		return &ImportABSSessionsOutput{
			Body: struct {
				SessionsImported int    `json:"sessions_imported" doc:"Sessions imported this batch"`
				EventsCreated    int    `json:"events_created" doc:"Listening events created"`
				Duration         string `json:"duration" doc:"Import duration"`
			}{
				SessionsImported: 0,
				EventsCreated:    0,
				Duration:         time.Since(start).String(),
			},
		}, nil
	}

	// Get all user and book mappings
	users, _ := s.store.ListABSImportUsers(ctx, input.ID, domain.MappingFilterMapped)
	books, _ := s.store.ListABSImportBooks(ctx, input.ID, domain.MappingFilterMapped)

	userMap := make(map[string]string) // ABS user ID -> ListenUp user ID
	for _, u := range users {
		if u.ListenUpID != nil {
			userMap[u.ABSUserID] = *u.ListenUpID
		}
	}

	bookMap := make(map[string]string) // ABS media ID -> ListenUp book ID
	for _, b := range books {
		if b.ListenUpID != nil {
			bookMap[b.ABSMediaID] = *b.ListenUpID
		}
	}

	// Import each ready session
	sessionsImported := 0
	eventsCreated := 0

	for _, sess := range readySessions {
		listenUpUserID, userOK := userMap[sess.ABSUserID]
		listenUpBookID, bookOK := bookMap[sess.ABSMediaID]

		if !userOK || !bookOK {
			continue // Skip if mappings not found (shouldn't happen for ready sessions)
		}

		// Create listening event
		event := &domain.ListeningEvent{
			UserID:          listenUpUserID,
			BookID:          listenUpBookID,
			StartPositionMs: sess.StartPosition,
			EndPositionMs:   sess.EndPosition,
			DurationMs:      sess.Duration,
			DeviceID:        "abs-import",
			DeviceName:      "ABS Import",
			StartedAt:       sess.StartTime,
			EndedAt:         sess.StartTime.Add(time.Duration(sess.Duration) * time.Millisecond),
			PlaybackSpeed:   1.0,
			CreatedAt:       time.Now(),
		}

		if err := s.store.CreateListeningEvent(ctx, event); err != nil {
			s.logger.Warn("failed to create event", slog.String("error", err.Error()))
			continue
		}

		eventsCreated++

		// Mark session as imported
		if err := s.store.UpdateABSImportSessionStatus(ctx, input.ID, sess.ABSSessionID, domain.SessionStatusImported); err != nil {
			s.logger.Warn("failed to mark session imported", slog.String("error", err.Error()))
		}

		sessionsImported++
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	return &ImportABSSessionsOutput{
		Body: struct {
			SessionsImported int    `json:"sessions_imported" doc:"Sessions imported this batch"`
			EventsCreated    int    `json:"events_created" doc:"Listening events created"`
			Duration         string `json:"duration" doc:"Import duration"`
		}{
			SessionsImported: sessionsImported,
			EventsCreated:    eventsCreated,
			Duration:         time.Since(start).String(),
		},
	}, nil
}

func (s *Server) handleSkipABSSession(ctx context.Context, input *SkipABSSessionInput) (*SkipABSSessionOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	reason := input.Body.Reason
	if reason == "" {
		reason = "Skipped by admin"
	}

	if err := s.store.SkipABSImportSession(ctx, input.ID, input.SessionID, reason); err != nil {
		return nil, huma.Error500InternalServerError("failed to skip session", err)
	}

	sess, err := s.store.GetABSImportSession(ctx, input.ID, input.SessionID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get session", err)
	}

	return &SkipABSSessionOutput{
		Body: toABSImportSessionResponse(sess),
	}, nil
}

// === Helper functions ===

func (s *Server) updateImportStats(ctx context.Context, importID string) {
	imp, err := s.store.GetABSImport(ctx, importID)
	if err != nil {
		return
	}

	users, _ := s.store.ListABSImportUsers(ctx, importID, domain.MappingFilterAll)
	books, _ := s.store.ListABSImportBooks(ctx, importID, domain.MappingFilterAll)
	sessions, _ := s.store.ListABSImportSessions(ctx, importID, domain.SessionFilterImported)

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

	_ = s.store.UpdateABSImport(ctx, imp)
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

func toABSImportUserResponse(u *domain.ABSImportUser) ABSImportUserResponse {
	resp := ABSImportUserResponse{
		ABSUserID:     u.ABSUserID,
		ABSUsername:   u.ABSUsername,
		ABSEmail:      u.ABSEmail,
		SessionCount:  u.SessionCount,
		TotalListenMs: u.TotalListenMs,
		Confidence:    u.Confidence,
		MatchReason:   u.MatchReason,
		Suggestions:   u.Suggestions,
		IsMapped:      u.IsMapped(),
	}
	if u.ListenUpID != nil {
		resp.ListenUpID = *u.ListenUpID
	}
	return resp
}

func toABSImportBookResponse(b *domain.ABSImportBook) ABSImportBookResponse {
	resp := ABSImportBookResponse{
		ABSMediaID:    b.ABSMediaID,
		ABSTitle:      b.ABSTitle,
		ABSAuthor:     b.ABSAuthor,
		ABSDurationMs: b.ABSDurationMs,
		SessionCount:  b.SessionCount,
		Confidence:    b.Confidence,
		MatchReason:   b.MatchReason,
		Suggestions:   b.Suggestions,
		IsMapped:      b.IsMapped(),
	}
	if b.ListenUpID != nil {
		resp.ListenUpID = *b.ListenUpID
	}
	return resp
}

func toABSImportSessionResponse(s *domain.ABSImportSession) ABSImportSessionResponse {
	resp := ABSImportSessionResponse{
		ABSSessionID:  s.ABSSessionID,
		ABSUserID:     s.ABSUserID,
		ABSMediaID:    s.ABSMediaID,
		StartTime:     s.StartTime.Format(time.RFC3339),
		Duration:      s.Duration,
		StartPosition: s.StartPosition,
		EndPosition:   s.EndPosition,
		Status:        string(s.Status),
	}
	if s.ImportedAt != nil {
		resp.ImportedAt = s.ImportedAt.Format(time.RFC3339)
	}
	if s.SkipReason != nil {
		resp.SkipReason = *s.SkipReason
	}
	return resp
}

func calculateUserListenTime(progress []abs.MediaProgress) int64 {
	var total int64
	for _, p := range progress {
		total += int64(p.CurrentTime * 1000) // Convert seconds to milliseconds
	}
	return total
}

func countSessionsForBook(sessions []abs.Session, mediaID string) int {
	count := 0
	for _, s := range sessions {
		if s.LibraryItemID == mediaID {
			count++
		}
	}
	return count
}

func extractUserSuggestionIDs(suggestions []abs.UserSuggestion) []string {
	ids := make([]string, len(suggestions))
	for i, s := range suggestions {
		ids[i] = s.UserID
	}
	return ids
}

func extractBookSuggestionIDs(suggestions []abs.BookSuggestion) []string {
	ids := make([]string, len(suggestions))
	for i, s := range suggestions {
		ids[i] = s.BookID
	}
	return ids
}
