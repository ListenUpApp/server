package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/listenupapp/listenup-server/internal/backup/abs"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
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
	ListenUpID          string   `json:"listenup_id,omitempty" doc:"Mapped ListenUp user ID"`
	ListenUpEmail       string   `json:"listenup_email,omitempty" doc:"Mapped ListenUp user email"`
	ListenUpDisplayName string   `json:"listenup_display_name,omitempty" doc:"Mapped ListenUp user display name"`
	SessionCount        int      `json:"session_count" doc:"Number of sessions for this user"`
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
	ListenUpID     string   `json:"listenup_id,omitempty" doc:"Mapped ListenUp book ID"`
	ListenUpTitle  string   `json:"listenup_title,omitempty" doc:"Mapped ListenUp book title"`
	ListenUpAuthor string   `json:"listenup_author,omitempty" doc:"Mapped ListenUp book author (first contributor)"`
	SessionCount   int      `json:"session_count" doc:"Number of sessions for this book"`
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
		SessionsImported       int    `json:"sessions_imported" doc:"Sessions successfully imported"`
		SessionsFailed         int    `json:"sessions_failed" doc:"Sessions that failed to import"`
		EventsCreated          int    `json:"events_created" doc:"Listening events created"`
		ProgressRebuilt        int    `json:"progress_rebuilt" doc:"User+book progress records rebuilt"`
		ProgressFailed         int    `json:"progress_failed" doc:"Progress rebuilds that failed"`
		ABSProgressUnmatched   int    `json:"abs_progress_unmatched" doc:"Books where ABS progress could not be matched (finished status may be incorrect)"`
		ReadingSessionsCreated int    `json:"reading_sessions_created" doc:"BookReadingSession records created for readers section"`
		ReadingSessionsSkipped int    `json:"reading_sessions_skipped" doc:"Sessions skipped (already existed)"`
		Duration               string `json:"duration" doc:"Import duration"`
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
		wasAutoMapped := false
		if um.Confidence.ShouldAutoImport() && um.ListenUpID != "" {
			user.ListenUpID = &um.ListenUpID
			now := time.Now()
			user.MappedAt = &now
			wasAutoMapped = true
		}

		if err := s.store.CreateABSImportUser(ctx, user); err != nil {
			s.logger.Error("failed to store import user",
				slog.String("abs_user_id", um.ABSUser.ID),
				slog.String("error", err.Error()))
			continue // Skip counting if store failed
		}

		// Store user's media progress entries (for finished status tracking)
		progressStored := 0
		finishedStored := 0
		for _, mp := range um.ABSUser.Progress {
			if !mp.IsBook() {
				continue // Skip podcasts
			}
			if mp.LibraryItemID == "" {
				s.logger.Warn("skipping media progress with empty library item ID",
					slog.String("abs_user_id", um.ABSUser.ID),
					slog.String("progress_id", mp.ID))
				continue
			}
			progress := &domain.ABSImportProgress{
				ImportID:    imp.ID,
				ABSUserID:   um.ABSUser.ID,
				ABSMediaID:  mp.LibraryItemID, // This is books.id, same as LibraryItem.MediaID
				CurrentTime: int64(mp.CurrentTime * 1000),
				Duration:    int64(mp.Duration * 1000),
				Progress:    mp.Progress,
				IsFinished:  mp.IsFinished,
				LastUpdate:  mp.LastUpdateTime(),
				Status:      domain.SessionStatusPendingBook, // Will be updated when book is mapped
			}
			if mp.FinishedAt > 0 {
				finishedAt := time.UnixMilli(mp.FinishedAt)
				progress.FinishedAt = &finishedAt
			}
			if err := s.store.CreateABSImportProgress(ctx, progress); err != nil {
				s.logger.Error("failed to store import progress",
					slog.String("abs_user_id", um.ABSUser.ID),
					slog.String("abs_media_id", mp.LibraryItemID),
					slog.String("error", err.Error()))
				// Continue - progress storage failure is not critical
			} else {
				progressStored++
				if mp.IsFinished {
					finishedStored++
					s.logger.Debug("stored FINISHED progress entry",
						slog.String("abs_user_id", um.ABSUser.ID),
						slog.String("abs_media_id", mp.LibraryItemID),
						slog.Int64("duration_ms", progress.Duration),
						slog.Int64("current_time_ms", progress.CurrentTime))
				}
			}
		}
		s.logger.Info("stored ABS import progress for user",
			slog.String("abs_user_id", um.ABSUser.ID),
			slog.String("abs_username", um.ABSUser.Username),
			slog.Int("progress_stored", progressStored),
			slog.Int("finished_stored", finishedStored))

		// Only count after successful store
		if wasAutoMapped {
			usersMapped++
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
		wasAutoMapped := false
		if bm.Confidence.ShouldAutoImport() && bm.ListenUpID != "" {
			book.ListenUpID = &bm.ListenUpID
			now := time.Now()
			book.MappedAt = &now
			wasAutoMapped = true
		}

		if err := s.store.CreateABSImportBook(ctx, book); err != nil {
			s.logger.Error("failed to store import book",
				slog.String("abs_media_id", bm.ABSItem.MediaID),
				slog.String("title", bm.ABSItem.Media.Metadata.Title),
				slog.String("error", err.Error()))
			continue // Skip counting if store failed
		}
		listenUpIDStr := ""
		if book.ListenUpID != nil {
			listenUpIDStr = *book.ListenUpID
		}
		s.logger.Debug("stored ABS import book",
			slog.String("abs_media_id", bm.ABSItem.MediaID),
			slog.String("abs_title", book.ABSTitle),
			slog.Bool("auto_mapped", wasAutoMapped),
			slog.String("listenup_id", listenUpIDStr))

		// Only count after successful store
		if wasAutoMapped {
			booksMapped++
		}
	}

	// DIAGNOSTIC: Log book mapping summary
	s.logger.Info("import creation: book mapping summary",
		slog.Int("total_books_in_abs", len(analysis.BookMatches)),
		slog.Int("auto_mapped_to_listenup", booksMapped),
		slog.Int("unmapped_books", len(analysis.BookMatches)-booksMapped))

	// Build a lookup from book MediaID (what sessions use) to the stored ABSMediaID
	// This handles potential discrepancies between session.LibraryItemID and book ABSMediaID
	bookMediaIDLookup := make(map[string]string) // session's mediaID -> book's ABSMediaID
	for _, bm := range analysis.BookMatches {
		// The book's ABSMediaID is bm.ABSItem.MediaID
		// Sessions might reference this as their LibraryItemID
		bookMediaIDLookup[bm.ABSItem.MediaID] = bm.ABSItem.MediaID
		// Also map the libraryItem's ID (li.id) to the book's mediaID in case sessions use li.id
		if bm.ABSItem.ID != "" && bm.ABSItem.ID != bm.ABSItem.MediaID {
			bookMediaIDLookup[bm.ABSItem.ID] = bm.ABSItem.MediaID
		}
	}

	s.logger.Info("built book media ID lookup for session normalization",
		slog.Int("lookup_entries", len(bookMediaIDLookup)),
		slog.Int("book_count", len(analysis.BookMatches)),
	)

	// Store all sessions with initial status
	sessionsStored := 0
	sessionsNormalized := 0
	sessionsUnmatched := 0
	unmatchedSample := make([]string, 0, 5)
	for i, session := range backup.Sessions {
		// DEBUG: Log first 5 sessions to trace position values
		if i < 5 {
			s.logger.Debug("storing ABS import session",
				slog.String("session_id", session.ID),
				slog.String("title", session.DisplayTitle),
				slog.Float64("startTime_sec", session.StartTime),
				slog.Float64("currentTime_sec", session.CurrentTime),
				slog.Int64("start_position_ms", session.StartPositionMs()),
				slog.Int64("end_position_ms", session.EndPositionMs()),
			)
		}

		// Try to normalize the session's ABSMediaID to match a book's ABSMediaID
		absMediaID := session.LibraryItemID
		if normalizedID, found := bookMediaIDLookup[absMediaID]; found {
			if normalizedID != absMediaID {
				sessionsNormalized++
			}
			absMediaID = normalizedID
		} else {
			sessionsUnmatched++
			if len(unmatchedSample) < 5 {
				unmatchedSample = append(unmatchedSample, absMediaID)
			}
		}

		sess := &domain.ABSImportSession{
			ImportID:      imp.ID,
			ABSSessionID:  session.ID,
			ABSUserID:     session.UserID,
			ABSMediaID:    absMediaID, // Normalized to match book's ABSMediaID
			StartTime:     session.StartedAtTime(),
			Duration:      session.DurationMs(),
			StartPosition: session.StartPositionMs(),
			EndPosition:   session.EndPositionMs(),
			Status:        domain.SessionStatusPendingUser, // Will be recalculated
		}

		if err := s.store.CreateABSImportSession(ctx, sess); err != nil {
			s.logger.Error("failed to store import session",
				slog.String("session_id", session.ID),
				slog.String("user_id", session.UserID),
				slog.String("error", err.Error()))
			continue // Session won't be available for import
		}
		sessionsStored++
	}

	// Log session normalization stats
	if sessionsUnmatched > 0 {
		s.logger.Warn("sessions with unmatched ABSMediaID - these won't be ready for import",
			slog.Int("unmatched_count", sessionsUnmatched),
			slog.Int("total_sessions", len(backup.Sessions)),
			slog.Any("unmatched_sample", unmatchedSample),
		)
	}
	if sessionsNormalized > 0 {
		s.logger.Info("sessions with normalized ABSMediaID",
			slog.Int("normalized_count", sessionsNormalized),
		)
	}

	// Log if any sessions failed to store
	if sessionsStored < len(backup.Sessions) {
		s.logger.Warn("some sessions failed to store",
			slog.Int("stored", sessionsStored),
			slog.Int("total", len(backup.Sessions)))
	}

	// Recalculate session statuses based on mappings
	if err := s.store.RecalculateSessionStatuses(ctx, imp.ID); err != nil {
		s.logger.Error("failed to recalculate session statuses",
			slog.String("import_id", imp.ID),
			slog.String("error", err.Error()))
		// Continue - statuses will be wrong but data is stored
	}

	// Update import with mapped counts
	imp.UsersMapped = usersMapped
	imp.BooksMapped = booksMapped
	if err := s.store.UpdateABSImport(ctx, imp); err != nil {
		s.logger.Error("failed to update import counts",
			slog.String("import_id", imp.ID),
			slog.Int("users_mapped", usersMapped),
			slog.Int("books_mapped", booksMapped),
			slog.String("error", err.Error()))
		// Continue - counts may be wrong in DB but response will be correct
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
	luUser, err := s.store.GetUser(ctx, input.Body.ListenUpID)
	if err != nil {
		return nil, huma.Error400BadRequest("ListenUp user not found")
	}

	// Resolve display info for the mapped user
	var luEmail, luDisplayName *string
	if luUser.Email != "" {
		luEmail = &luUser.Email
	}
	if luUser.DisplayName != "" {
		luDisplayName = &luUser.DisplayName
	}

	if err := s.store.UpdateABSImportUserMapping(ctx, input.ID, input.ABSUserID, &input.Body.ListenUpID, luEmail, luDisplayName); err != nil {
		return nil, huma.Error500InternalServerError("failed to update mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, input.ID); err != nil {
		s.logger.Error("failed to recalculate sessions", slog.String("error", err.Error()))
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

	if err := s.store.UpdateABSImportUserMapping(ctx, input.ID, input.ABSUserID, nil, nil, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to clear mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, input.ID); err != nil {
		s.logger.Error("failed to recalculate sessions", slog.String("error", err.Error()))
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
	luBook, err := s.store.GetBook(ctx, input.Body.ListenUpID, "")
	if err != nil {
		return nil, huma.Error400BadRequest("ListenUp book not found")
	}

	// Resolve display info for the mapped book
	var luTitle *string
	if luBook.Title != "" {
		luTitle = &luBook.Title
	}
	luAuthor := (*string)(nil) // Contributors are separate entities; author display TBD

	if err := s.store.UpdateABSImportBookMapping(ctx, input.ID, input.ABSMediaID, &input.Body.ListenUpID, luTitle, luAuthor); err != nil {
		return nil, huma.Error500InternalServerError("failed to update mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, input.ID); err != nil {
		s.logger.Error("failed to recalculate sessions", slog.String("error", err.Error()))
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

	if err := s.store.UpdateABSImportBookMapping(ctx, input.ID, input.ABSMediaID, nil, nil, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to clear mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, input.ID); err != nil {
		s.logger.Error("failed to recalculate sessions", slog.String("error", err.Error()))
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

	// Also get all sessions for summary - if this fails, we still return filtered results
	// but with empty summary (better than hiding the error completely)
	allSessions, err := s.store.ListABSImportSessions(ctx, input.ID, domain.SessionFilterAll)
	if err != nil {
		s.logger.Error("failed to get all sessions for summary", slog.String("error", err.Error()))
		// Continue with empty allSessions - summary will be zeros
		allSessions = nil
	}

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
				SessionsImported       int    `json:"sessions_imported" doc:"Sessions successfully imported"`
				SessionsFailed         int    `json:"sessions_failed" doc:"Sessions that failed to import"`
				EventsCreated          int    `json:"events_created" doc:"Listening events created"`
				ProgressRebuilt        int    `json:"progress_rebuilt" doc:"User+book progress records rebuilt"`
				ProgressFailed         int    `json:"progress_failed" doc:"Progress rebuilds that failed"`
				ABSProgressUnmatched   int    `json:"abs_progress_unmatched" doc:"Books where ABS progress could not be matched (finished status may be incorrect)"`
				ReadingSessionsCreated int    `json:"reading_sessions_created" doc:"BookReadingSession records created for readers section"`
				ReadingSessionsSkipped int    `json:"reading_sessions_skipped" doc:"Sessions skipped (already existed)"`
				Duration               string `json:"duration" doc:"Import duration"`
			}{
				SessionsImported:       0,
				SessionsFailed:         0,
				EventsCreated:          0,
				ProgressRebuilt:        0,
				ProgressFailed:         0,
				ABSProgressUnmatched:   0,
				ReadingSessionsCreated: 0,
				ReadingSessionsSkipped: 0,
				Duration:               time.Since(start).String(),
			},
		}, nil
	}

	// Get all user and book mappings - these MUST succeed or import will silently fail
	users, err := s.store.ListABSImportUsers(ctx, input.ID, domain.MappingFilterMapped)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load user mappings", err)
	}
	books, err := s.store.ListABSImportBooks(ctx, input.ID, domain.MappingFilterMapped)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load book mappings", err)
	}

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

	// DIAGNOSTIC: Log mapping counts and sample data
	s.logger.Info("execute import: mapping summary",
		slog.Int("user_mappings", len(userMap)),
		slog.Int("book_mappings", len(bookMap)),
		slog.Int("ready_sessions", len(readySessions)))

	// DIAGNOSTIC: Log sample of book mappings for debugging
	bookMapSample := make([]string, 0, 5)
	for absID, luID := range bookMap {
		bookMapSample = append(bookMapSample, absID+" -> "+luID)
		if len(bookMapSample) >= 5 {
			break
		}
	}
	s.logger.Debug("execute import: book mapping sample", slog.Any("samples", bookMapSample))

	// Import each ready session
	sessionsImported := 0
	sessionsFailed := 0
	eventsCreated := 0

	// Track user+book combinations to update progress after import
	// Include ABS IDs so we can look up finished status from ABSImportProgress
	type userBookKey struct {
		userID     string // ListenUp user ID
		bookID     string // ListenUp book ID
		absUserID  string // ABS user ID (for progress lookup)
		absMediaID string // ABS media ID (for progress lookup)
	}
	affectedUserBooks := make(map[userBookKey]bool)

	for i, sess := range readySessions {
		listenUpUserID, userOK := userMap[sess.ABSUserID]
		listenUpBookID, bookOK := bookMap[sess.ABSMediaID]

		if !userOK || !bookOK {
			continue // Skip if mappings not found (shouldn't happen for ready sessions)
		}

		// DEBUG: Log first 5 sessions to trace position values
		if i < 5 {
			s.logger.Debug("creating listening event from ABS session",
				slog.String("session_id", sess.ABSSessionID),
				slog.String("abs_media_id", sess.ABSMediaID),
				slog.String("listenup_book_id", listenUpBookID),
				slog.Int64("start_position", sess.StartPosition),
				slog.Int64("end_position", sess.EndPosition),
				slog.Int64("duration", sess.Duration),
			)
		}

		// Generate event ID
		eventID, err := id.Generate("evt")
		if err != nil {
			s.logger.Error("failed to generate event ID",
				slog.String("session_id", sess.ABSSessionID),
				slog.String("error", err.Error()))
			sessionsFailed++
			continue
		}

		// Create listening event
		event := &domain.ListeningEvent{
			ID:              eventID,
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
			s.logger.Error("failed to create listening event",
				slog.String("session_id", sess.ABSSessionID),
				slog.String("error", err.Error()))
			sessionsFailed++
			continue
		}

		eventsCreated++
		affectedUserBooks[userBookKey{
			userID:     listenUpUserID,
			bookID:     listenUpBookID,
			absUserID:  sess.ABSUserID,
			absMediaID: sess.ABSMediaID,
		}] = true

		// Mark session as imported - if this fails, the event was still created
		// so we count it as imported but log the status update failure
		if err := s.store.UpdateABSImportSessionStatus(ctx, input.ID, sess.ABSSessionID, domain.SessionStatusImported); err != nil {
			s.logger.Error("failed to mark session imported (event was created)",
				slog.String("session_id", sess.ABSSessionID),
				slog.String("error", err.Error()))
		}

		sessionsImported++
	}

	// DIAGNOSTIC: Log session import results
	s.logger.Info("execute import: session import complete",
		slog.Int("sessions_imported", sessionsImported),
		slog.Int("sessions_failed", sessionsFailed),
		slog.Int("events_created", eventsCreated),
		slog.Int("unique_user_book_combos", len(affectedUserBooks)))

	// Rebuild PlaybackProgress for all affected user+book combinations
	// This is critical for Continue Listening to work after ABS import
	progressRebuilt := 0
	progressFailed := 0
	progressUnmatchedABS := 0 // Books where we couldn't match ABS progress data
	for key := range affectedUserBooks {
		matched, err := s.rebuildProgressFromEvents(ctx, input.ID, key.userID, key.bookID, key.absUserID, key.absMediaID)
		if err != nil {
			s.logger.Error("failed to rebuild progress",
				slog.String("user_id", key.userID),
				slog.String("book_id", key.bookID),
				slog.String("error", err.Error()))
			progressFailed++
		} else {
			progressRebuilt++
			if !matched {
				progressUnmatchedABS++
			}
		}
	}
	s.logger.Info("rebuilt progress for imported sessions",
		slog.Int("progress_rebuilt", progressRebuilt),
		slog.Int("progress_failed", progressFailed),
		slog.Int("progress_unmatched_abs", progressUnmatchedABS))

	// Create progress records from MediaProgress entries that don't have sessions
	// This ensures the Readers section is populated even for books without detailed history
	//
	// IMPORTANT: We iterate over stored ABSImportProgress entries directly rather than
	// iterating over bookMap. This handles cases where the ABSMediaID in progress
	// (from mediaProgresses.mediaItemId) might differ from the key used in bookMap
	// (from libraryItems.mediaId).
	progressFromMediaProgress := 0
	progressSkippedNoBook := 0
	progressSkippedHasProgress := 0
	for _, u := range users {
		if u.ListenUpID == nil {
			continue
		}
		listenUpUserID := *u.ListenUpID

		// Get all MediaProgress entries stored for this user
		allProgress, err := s.store.ListABSImportProgressForUser(ctx, input.ID, u.ABSUserID)
		if err != nil {
			s.logger.Warn("failed to list ABS progress for user",
				slog.String("abs_user_id", u.ABSUserID),
				slog.String("error", err.Error()))
			continue
		}

		// DIAGNOSTIC: Log progress entries for this user
		s.logger.Info("found ABS progress entries for user",
			slog.String("abs_user_id", u.ABSUserID),
			slog.String("listenup_user_id", listenUpUserID),
			slog.Int("count", len(allProgress)))

		for _, absProgress := range allProgress {
			if absProgress.CurrentTime == 0 {
				s.logger.Debug("skipping progress with zero position",
					slog.String("abs_media_id", absProgress.ABSMediaID))
				continue // No position to import
			}

			// Find the matching book - first try direct lookup, then try via bookMap
			var listenUpBookID string
			absBook, err := s.store.GetABSImportBook(ctx, input.ID, absProgress.ABSMediaID)
			if err == nil && absBook != nil && absBook.ListenUpID != nil {
				listenUpBookID = *absBook.ListenUpID
				s.logger.Debug("found book via direct lookup",
					slog.String("abs_progress_media_id", absProgress.ABSMediaID),
					slog.String("listenup_book_id", listenUpBookID),
					slog.Bool("is_finished", absProgress.IsFinished))
			} else {
				// Fallback: check if bookMap has this ID (handles potential ID variations)
				if id, ok := bookMap[absProgress.ABSMediaID]; ok {
					listenUpBookID = id
					s.logger.Debug("found book via bookMap fallback",
						slog.String("abs_progress_media_id", absProgress.ABSMediaID),
						slog.String("listenup_book_id", listenUpBookID))
				} else {
					s.logger.Warn("no book mapping found for ABS progress - SKIPPING",
						slog.String("abs_media_id", absProgress.ABSMediaID),
						slog.Int64("current_time_ms", absProgress.CurrentTime),
						slog.Bool("is_finished", absProgress.IsFinished),
						slog.Float64("progress_pct", absProgress.Progress*100))
					progressSkippedNoBook++
					continue
				}
			}

			// Skip if we already have progress from sessions
			key := userBookKey{
				userID:     listenUpUserID,
				bookID:     listenUpBookID,
				absUserID:  u.ABSUserID,
				absMediaID: absProgress.ABSMediaID,
			}
			if affectedUserBooks[key] {
				s.logger.Debug("skipping - already handled via sessions",
					slog.String("abs_media_id", absProgress.ABSMediaID),
					slog.String("listenup_book_id", listenUpBookID))
				continue // Already handled via sessions
			}

			// Only create if no existing state
			existingProgress, _ := s.store.GetState(ctx, listenUpUserID, listenUpBookID)
			if existingProgress != nil {
				// Get book duration for logging
				existingBook, _ := s.store.GetBookNoAccessCheck(ctx, listenUpBookID)
				existingDuration := int64(0)
				if existingBook != nil {
					existingDuration = existingBook.TotalDuration
				}
				s.logger.Debug("skipping - existing progress found",
					slog.String("abs_media_id", absProgress.ABSMediaID),
					slog.String("listenup_book_id", listenUpBookID),
					slog.Bool("existing_is_finished", existingProgress.IsFinished),
					slog.Float64("existing_progress_pct", existingProgress.ComputeProgress(existingDuration)*100))
				progressSkippedHasProgress++
				continue // Already has progress
			}

			// Get book duration from ListenUp
			book, err := s.store.GetBookNoAccessCheck(ctx, listenUpBookID)
			if err != nil || book == nil {
				continue
			}

			// Calculate position - clamp if needed, but don't skip books without duration
			bookDurationMs := book.TotalDuration
			positionMs := absProgress.CurrentTime
			if bookDurationMs > 0 && positionMs > bookDurationMs {
				positionMs = int64(float64(bookDurationMs) * 0.98)
			}

			progress := &domain.PlaybackState{
				UserID:            listenUpUserID,
				BookID:            listenUpBookID,
				CurrentPositionMs: positionMs,
				StartedAt:         absProgress.LastUpdate, // Best approximation
				LastPlayedAt:      absProgress.LastUpdate,
				TotalListenTimeMs: positionMs, // Approximate
				UpdatedAt:         time.Now(),
				IsFinished:        absProgress.IsFinished,
			}
			if absProgress.IsFinished {
				progress.FinishedAt = absProgress.FinishedAt
			}

			if err := s.store.UpsertState(ctx, progress); err != nil {
				s.logger.Warn("failed to create state from MediaProgress",
					slog.String("user_id", listenUpUserID),
					slog.String("book_id", listenUpBookID),
					slog.String("error", err.Error()))
			} else {
				progressFromMediaProgress++
				s.logger.Debug("created progress from MediaProgress",
					slog.String("user_id", listenUpUserID),
					slog.String("book_id", listenUpBookID),
					slog.Bool("is_finished", progress.IsFinished),
					slog.Float64("progress_pct", progress.ComputeProgress(bookDurationMs)*100))
			}
		}
	}
	if progressFromMediaProgress > 0 || progressSkippedNoBook > 0 {
		s.logger.Info("created progress from MediaProgress",
			slog.Int("created", progressFromMediaProgress),
			slog.Int("skipped_no_book", progressSkippedNoBook),
			slog.Int("skipped_has_progress", progressSkippedHasProgress))
	}

	// Create BookReadingSession records from ABS progress (populates the "Readers" section)
	// This is a separate pass to ensure sessions are created for all user+book combinations
	// that have progress, regardless of whether the progress was created in this import or existed before.
	readingSessionsCreated := 0
	readingSessionsSkipped := 0
	for _, u := range users {
		if u.ListenUpID == nil {
			continue
		}
		listenUpUserID := *u.ListenUpID

		// Get all MediaProgress entries stored for this user
		allProgress, err := s.store.ListABSImportProgressForUser(ctx, input.ID, u.ABSUserID)
		if err != nil {
			continue
		}

		for _, absProgress := range allProgress {
			// Find the ListenUp book ID
			var listenUpBookID string
			absBook, err := s.store.GetABSImportBook(ctx, input.ID, absProgress.ABSMediaID)
			if err == nil && absBook != nil && absBook.ListenUpID != nil {
				listenUpBookID = *absBook.ListenUpID
			} else if bookID, ok := bookMap[absProgress.ABSMediaID]; ok {
				listenUpBookID = bookID
			} else {
				continue // Can't create session without book mapping
			}

			// Check if user already has a session for this book
			existingSessions, err := s.store.GetUserBookSessions(ctx, listenUpUserID, listenUpBookID)
			if err == nil && len(existingSessions) > 0 {
				readingSessionsSkipped++
				continue // Already has a reading session
			}

			// Generate session ID
			sessionID, err := id.Generate("rsession")
			if err != nil {
				s.logger.Warn("failed to generate reading session ID",
					slog.String("user_id", listenUpUserID),
					slog.String("book_id", listenUpBookID),
					slog.String("error", err.Error()))
				continue
			}

			// Determine timestamps (use LastUpdate as best approximation for started time)
			now := time.Now()
			startedAt := absProgress.LastUpdate
			if startedAt.IsZero() {
				startedAt = now
			}

			// Calculate estimated listen time from progress
			var listenTimeMs int64
			if absProgress.Duration > 0 {
				listenTimeMs = int64(absProgress.Progress * float64(absProgress.Duration))
			}

			// Create the reading session
			session := &domain.BookReadingSession{
				ID:            sessionID,
				UserID:        listenUpUserID,
				BookID:        listenUpBookID,
				StartedAt:     startedAt,
				FinishedAt:    nil,
				IsCompleted:   absProgress.IsFinished,
				FinalProgress: absProgress.Progress,
				ListenTimeMs:  listenTimeMs,
				CreatedAt:     now,
				UpdatedAt:     now,
			}

			// If finished, set the FinishedAt timestamp
			if absProgress.IsFinished {
				if absProgress.FinishedAt != nil {
					session.FinishedAt = absProgress.FinishedAt
				} else {
					session.FinishedAt = &now
				}
			}

			// Store the session
			if err := s.store.CreateReadingSession(ctx, session); err != nil {
				s.logger.Warn("failed to create reading session",
					slog.String("session_id", sessionID),
					slog.String("user_id", listenUpUserID),
					slog.String("book_id", listenUpBookID),
					slog.String("error", err.Error()))
				continue
			}

			readingSessionsCreated++
			s.logger.Debug("created reading session from ABS progress",
				slog.String("session_id", sessionID),
				slog.String("user_id", listenUpUserID),
				slog.String("book_id", listenUpBookID),
				slog.Bool("is_finished", absProgress.IsFinished),
				slog.Float64("progress", absProgress.Progress))
		}
	}
	if readingSessionsCreated > 0 || readingSessionsSkipped > 0 {
		s.logger.Info("created reading sessions from ABS progress",
			slog.Int("created", readingSessionsCreated),
			slog.Int("skipped_existing", readingSessionsSkipped))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	return &ImportABSSessionsOutput{
		Body: struct {
			SessionsImported       int    `json:"sessions_imported" doc:"Sessions successfully imported"`
			SessionsFailed         int    `json:"sessions_failed" doc:"Sessions that failed to import"`
			EventsCreated          int    `json:"events_created" doc:"Listening events created"`
			ProgressRebuilt        int    `json:"progress_rebuilt" doc:"User+book progress records rebuilt"`
			ProgressFailed         int    `json:"progress_failed" doc:"Progress rebuilds that failed"`
			ABSProgressUnmatched   int    `json:"abs_progress_unmatched" doc:"Books where ABS progress could not be matched (finished status may be incorrect)"`
			ReadingSessionsCreated int    `json:"reading_sessions_created" doc:"BookReadingSession records created for readers section"`
			ReadingSessionsSkipped int    `json:"reading_sessions_skipped" doc:"Sessions skipped (already existed)"`
			Duration               string `json:"duration" doc:"Import duration"`
		}{
			SessionsImported:       sessionsImported,
			SessionsFailed:         sessionsFailed,
			EventsCreated:          eventsCreated,
			ProgressRebuilt:        progressRebuilt,
			ProgressFailed:         progressFailed,
			ABSProgressUnmatched:   progressUnmatchedABS,
			ReadingSessionsCreated: readingSessionsCreated,
			ReadingSessionsSkipped: readingSessionsSkipped,
			Duration:               time.Since(start).String(),
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
	if u.ListenUpEmail != nil {
		resp.ListenUpEmail = *u.ListenUpEmail
	}
	if u.ListenUpDisplayName != nil {
		resp.ListenUpDisplayName = *u.ListenUpDisplayName
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
	if b.ListenUpTitle != nil {
		resp.ListenUpTitle = *b.ListenUpTitle
	}
	if b.ListenUpAuthor != nil {
		resp.ListenUpAuthor = *b.ListenUpAuthor
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

// rebuildProgressFromEvents rebuilds PlaybackProgress for a user+book from all events.
// This is used after ABS import to ensure Continue Listening works correctly.
// Handles duration mismatch: if ABS position > ListenUp duration, clamps to 98% to avoid
// incorrectly marking books as completed.
// Also honors the finished status from ABSImportProgress if the book was marked completed in ABS.
// Returns (absProgressMatched, error) where absProgressMatched indicates if ABS progress was found.
func (s *Server) rebuildProgressFromEvents(ctx context.Context, importID, userID, bookID, absUserID, absMediaID string) (absProgressMatched bool, err error) {
	// Get all events for this user+book
	events, err := s.store.GetEventsForUserBook(ctx, userID, bookID)
	if err != nil {
		return false, fmt.Errorf("get events: %w", err)
	}

	if len(events) == 0 {
		return true, nil // Nothing to do - consider matched (no progress to check)
	}

	// DEBUG: Log events being used for progress rebuild
	s.logger.Debug("events for progress rebuild",
		slog.String("user_id", userID),
		slog.String("book_id", bookID),
		slog.Int("event_count", len(events)),
	)
	for i, e := range events {
		if i < 3 { // Log first 3 events
			s.logger.Debug("event detail",
				slog.String("event_id", e.ID),
				slog.String("event_book_id", e.BookID),
				slog.Int64("end_position_ms", e.EndPositionMs),
			)
		}
	}

	// Get book duration from ListenUp
	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return false, fmt.Errorf("get book: %w", err)
	}
	if book == nil {
		return false, fmt.Errorf("book %s not found (nil returned without error)", bookID)
	}
	bookDurationMs := book.TotalDuration
	if bookDurationMs <= 0 {
		return false, fmt.Errorf("book %s has invalid duration: %d", bookID, bookDurationMs)
	}

	// Find the latest event and calculate totals
	var latestEvent *domain.ListeningEvent
	var totalListenTimeMs int64
	var maxPositionMs int64
	var earliestStartedAt time.Time

	for _, event := range events {
		totalListenTimeMs += event.DurationMs

		// Track max position (forward progress only)
		if event.EndPositionMs > maxPositionMs {
			maxPositionMs = event.EndPositionMs
		}

		// Track latest event by EndedAt
		if latestEvent == nil || event.EndedAt.After(latestEvent.EndedAt) {
			latestEvent = event
		}

		// Track earliest start
		if earliestStartedAt.IsZero() || event.StartedAt.Before(earliestStartedAt) {
			earliestStartedAt = event.StartedAt
		}
	}

	// Handle duration mismatch: if imported position exceeds book duration,
	// clamp to 98% to avoid incorrectly marking as completed.
	// This can happen when ABS had a different duration than ListenUp.
	if maxPositionMs > bookDurationMs && bookDurationMs > 0 {
		s.logger.Warn("ABS position exceeds book duration, clamping",
			slog.String("user_id", userID),
			slog.String("book_id", bookID),
			slog.Int64("abs_position_ms", maxPositionMs),
			slog.Int64("book_duration_ms", bookDurationMs))
		// Clamp to 98% - this ensures the book appears in Continue Listening
		// rather than being incorrectly marked as finished
		maxPositionMs = int64(float64(bookDurationMs) * 0.98)
	}

	// Get existing state or create new
	existingProgress, err := s.store.GetState(ctx, userID, bookID)
	if err != nil && !errors.Is(err, store.ErrProgressNotFound) {
		// Real error, not just "not found"
		return false, fmt.Errorf("get state: %w", err)
	}

	var progress *domain.PlaybackState
	needsSave := true
	if existingProgress != nil {
		// Update existing - only if we have newer data
		if latestEvent.EndedAt.After(existingProgress.LastPlayedAt) {
			progress = existingProgress
			progress.CurrentPositionMs = maxPositionMs
			progress.TotalListenTimeMs += totalListenTimeMs - existingProgress.TotalListenTimeMs // Add delta
			progress.LastPlayedAt = latestEvent.EndedAt
			progress.UpdatedAt = time.Now()

			// Check completion (but we already clamped to avoid false positives)
			if bookDurationMs > 0 && float64(progress.CurrentPositionMs) >= float64(bookDurationMs)*0.99 {
				progress.IsFinished = true
				now := time.Now()
				progress.FinishedAt = &now
			}
		} else {
			// Existing progress is newer - don't update position data
			// BUT still check ABS finished status below (user might have finished in ABS
			// but only played a few seconds in ListenUp)
			progress = existingProgress
			needsSave = false // Will be set to true if we update IsFinished
		}
	} else {
		// Create new progress
		progress = &domain.PlaybackState{
			UserID:            userID,
			BookID:            bookID,
			CurrentPositionMs: maxPositionMs,
			StartedAt:         earliestStartedAt,
			LastPlayedAt:      latestEvent.EndedAt,
			TotalListenTimeMs: totalListenTimeMs,
			UpdatedAt:         time.Now(),
		}

		// Check completion
		if bookDurationMs > 0 && float64(progress.CurrentPositionMs) >= float64(bookDurationMs)*0.99 {
			progress.IsFinished = true
			now := time.Now()
			progress.FinishedAt = &now
		}
	}

	// Check if ABS marked this book as finished (honors original listening history)
	// This handles cases where the position doesn't reach 99% but the user marked it complete in ABS
	//
	// IMPORTANT: We look up ABSImportProgress by ListenUp book ID rather than ABS media ID.
	// This is because ABS playbackSessions.mediaItemId and mediaProgresses.mediaItemId can contain
	// DIFFERENT UUIDs for the same logical book. By resolving through the ListenUp book ID,
	// we correctly match progress entries regardless of which ABS ID scheme was used.
	absProgressMatched = true // Assume matched unless we explicitly fail to find it
	s.logger.Info("checking ABS finished status for book",
		slog.String("book_id", bookID),
		slog.String("import_id", importID),
		slog.String("abs_user_id", absUserID),
		slog.Bool("progress_is_finished", progress.IsFinished),
		slog.Float64("progress_pct", progress.ComputeProgress(bookDurationMs)*100))
	if !progress.IsFinished && importID != "" && absUserID != "" {
		absProgress, err := s.store.FindABSImportProgressByListenUpBook(ctx, importID, absUserID, bookID)
		if err != nil {
			s.logger.Warn("failed to find ABS import progress by book",
				slog.String("abs_user_id", absUserID),
				slog.String("book_id", bookID),
				slog.String("error", err.Error()))
			absProgressMatched = false
		} else if absProgress == nil {
			s.logger.Warn("no ABS import progress maps to this book - cannot honor ABS finished status",
				slog.String("import_id", importID),
				slog.String("abs_user_id", absUserID),
				slog.String("book_id", bookID))
			absProgressMatched = false
		} else if absProgress.IsFinished {
			progress.IsFinished = true
			if absProgress.FinishedAt != nil {
				progress.FinishedAt = absProgress.FinishedAt
			} else {
				now := time.Now()
				progress.FinishedAt = &now
			}
			progress.UpdatedAt = time.Now()
			needsSave = true // ABS says finished, must save this
			s.logger.Info("marking book as finished from ABS import",
				slog.String("user_id", userID),
				slog.String("book_id", bookID),
				slog.String("abs_media_id", absProgress.ABSMediaID))
		} else {
			s.logger.Debug("ABS import progress found but not finished",
				slog.String("abs_user_id", absUserID),
				slog.String("abs_media_id", absProgress.ABSMediaID),
				slog.Bool("abs_is_finished", absProgress.IsFinished))
		}
	}

	// Save progress if we have changes
	if !needsSave {
		s.logger.Debug("skipping save - existing progress is newer and no ABS finished status change",
			slog.String("book_id", bookID))
		return absProgressMatched, nil
	}

	if err := s.store.UpsertState(ctx, progress); err != nil {
		return absProgressMatched, fmt.Errorf("upsert state: %w", err)
	}

	// VERIFY: Read back what was actually saved to confirm IsFinished persisted
	savedProgress, verifyErr := s.store.GetState(ctx, userID, bookID)
	if verifyErr != nil {
		s.logger.Error("VERIFY FAILED: could not read back saved progress",
			slog.String("user_id", userID),
			slog.String("book_id", bookID),
			slog.String("error", verifyErr.Error()))
	} else {
		s.logger.Info("VERIFY: read back saved progress",
			slog.String("book_id", bookID),
			slog.Bool("intended_is_finished", progress.IsFinished),
			slog.Bool("saved_is_finished", savedProgress.IsFinished),
			slog.Bool("match", progress.IsFinished == savedProgress.IsFinished))
		if progress.IsFinished != savedProgress.IsFinished {
			s.logger.Error("VERIFY MISMATCH: IsFinished was not saved correctly!",
				slog.String("book_id", bookID),
				slog.Bool("intended", progress.IsFinished),
				slog.Bool("actual", savedProgress.IsFinished))
		}
	}

	s.logger.Debug("rebuilt progress from events",
		slog.String("user_id", userID),
		slog.String("book_id", bookID),
		slog.Int64("position_ms", progress.CurrentPositionMs),
		slog.Float64("progress", progress.ComputeProgress(bookDurationMs)),
		slog.Bool("is_finished", progress.IsFinished))

	return absProgressMatched, nil
}
