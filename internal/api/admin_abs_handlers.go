package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/listenupapp/listenup-server/internal/backup/abs"
)

func (s *Server) registerAdminABSRoutes() {
	// Upload endpoint uses chi directly for multipart form handling
	// Wrapped with extended timeout (10 minutes) to handle large file uploads
	s.router.Post("/api/v1/admin/abs/upload", withExtendedTimeout(s.handleUploadABSBackup, 10*time.Minute))

	huma.Register(s.api, huma.Operation{
		OperationID: "analyzeABSBackup",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/abs/analyze",
		Summary:     "Analyze ABS backup",
		Description: "Analyzes an Audiobookshelf backup and shows what can be imported (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAnalyzeABSBackup)

	huma.Register(s.api, huma.Operation{
		OperationID: "importABSBackup",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/abs/import",
		Summary:     "Import ABS backup",
		Description: "Imports listening history from an Audiobookshelf backup (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleImportABSBackup)

	huma.Register(s.api, huma.Operation{
		OperationID: "analyzeABSBackupAsync",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/abs/analyze/async",
		Summary:     "Start async ABS backup analysis",
		Description: "Starts analysis in the background and returns an analysis ID for polling progress (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAnalyzeABSBackupAsync)

	huma.Register(s.api, huma.Operation{
		OperationID: "getAnalysisStatus",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/analyze/{analysisId}/status",
		Summary:     "Get analysis progress",
		Description: "Returns the current progress and result of an async analysis (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetAnalysisStatus)
}

// === DTOs ===

// AnalyzeABSRequest is the request body for analyzing an ABS backup.
type AnalyzeABSRequest struct {
	BackupPath      string            `json:"backup_path" doc:"Path to .audiobookshelf backup file"`
	MatchByEmail    bool              `json:"match_by_email" required:"false" default:"true" doc:"Match users by email address"`
	MatchByPath     bool              `json:"match_by_path" required:"false" default:"true" doc:"Match books by filesystem path"`
	FuzzyMatchBooks bool              `json:"fuzzy_match_books" required:"false" default:"true" doc:"Enable fuzzy title/author matching"`
	FuzzyThreshold  float64           `json:"fuzzy_threshold" required:"false" default:"0.85" doc:"Minimum similarity for fuzzy matches (0.0-1.0)"`
	UserMappings    map[string]string `json:"user_mappings,omitempty" doc:"Manual ABS user ID -> ListenUp user ID mappings"`
	BookMappings    map[string]string `json:"book_mappings,omitempty" doc:"Manual ABS item ID -> ListenUp book ID mappings"`
}

// AnalyzeABSInput is the Huma input for analyzing an ABS backup.
type AnalyzeABSInput struct {
	Authorization string `header:"Authorization"`
	Body          AnalyzeABSRequest
}

// ABSUserMatchResponse represents a user matching result.
type ABSUserMatchResponse struct {
	ABSUserID   string              `json:"abs_user_id" doc:"ABS user ID"`
	ABSUsername string              `json:"abs_username" doc:"ABS username"`
	ABSEmail    string              `json:"abs_email,omitempty" doc:"ABS email"`
	ListenUpID  string              `json:"listenup_id,omitempty" doc:"Matched ListenUp user ID"`
	Confidence  string              `json:"confidence" doc:"Match confidence: none, weak, strong, definitive"`
	MatchReason string              `json:"match_reason,omitempty" doc:"Why this match was made"`
	Suggestions []ABSUserSuggestion `json:"suggestions,omitempty" doc:"Suggested matches for admin review"`
}

// ABSUserSuggestion is a suggested user match.
type ABSUserSuggestion struct {
	UserID      string  `json:"user_id" doc:"ListenUp user ID"`
	Email       string  `json:"email,omitempty" doc:"User email"`
	DisplayName string  `json:"display_name,omitempty" doc:"User display name"`
	Score       float64 `json:"score" doc:"Similarity score 0.0-1.0"`
	Reason      string  `json:"reason" doc:"Why this is suggested"`
}

// ABSBookMatchResponse represents a book matching result.
type ABSBookMatchResponse struct {
	ABSItemID   string              `json:"abs_item_id" doc:"ABS library item ID"`
	ABSTitle    string              `json:"abs_title" doc:"ABS book title"`
	ABSAuthor   string              `json:"abs_author,omitempty" doc:"ABS primary author"`
	ListenUpID  string              `json:"listenup_id,omitempty" doc:"Matched ListenUp book ID"`
	Confidence  string              `json:"confidence" doc:"Match confidence: none, weak, strong, definitive"`
	MatchReason string              `json:"match_reason,omitempty" doc:"Why this match was made"`
	Suggestions []ABSBookSuggestion `json:"suggestions,omitempty" doc:"Suggested matches for admin review"`
}

// ABSBookSuggestion is a suggested book match.
type ABSBookSuggestion struct {
	BookID     string  `json:"book_id" doc:"ListenUp book ID"`
	Title      string  `json:"title" doc:"Book title"`
	Author     string  `json:"author,omitempty" doc:"Primary author"`
	DurationMs int64   `json:"duration_ms" doc:"Book duration in milliseconds"`
	Score      float64 `json:"score" doc:"Similarity score 0.0-1.0"`
	Reason     string  `json:"reason" doc:"Why this is suggested"`
}

// AnalyzeABSResponse is the response from analyzing an ABS backup.
type AnalyzeABSResponse struct {
	BackupPath string `json:"backup_path" doc:"Path to analyzed backup"`
	AnalyzedAt string `json:"analyzed_at" doc:"When analysis was performed"`
	Summary    string `json:"summary" doc:"Human-readable summary of backup contents"`

	// Counts
	TotalUsers    int `json:"total_users" doc:"Total importable users in backup"`
	TotalBooks    int `json:"total_books" doc:"Total audiobooks in backup"`
	TotalSessions int `json:"total_sessions" doc:"Total listening sessions in backup"`

	// Match results
	UsersMatched int `json:"users_matched" doc:"Users that matched automatically"`
	UsersPending int `json:"users_pending" doc:"Users needing manual mapping"`
	BooksMatched int `json:"books_matched" doc:"Books that matched automatically"`
	BooksPending int `json:"books_pending" doc:"Books needing manual mapping"`

	// What can be imported
	SessionsReady   int `json:"sessions_ready" doc:"Sessions ready to import"`
	SessionsPending int `json:"sessions_pending" doc:"Sessions pending (need user/book mapping)"`
	ProgressReady   int `json:"progress_ready" doc:"Progress records ready to import"`
	ProgressPending int `json:"progress_pending" doc:"Progress records pending"`

	// Details (for admin review)
	UserMatches []ABSUserMatchResponse `json:"user_matches" doc:"Detailed user matching results"`
	BookMatches []ABSBookMatchResponse `json:"book_matches" doc:"Detailed book matching results"`
	Warnings    []string               `json:"warnings,omitempty" doc:"Warnings from analysis"`
}

// AnalyzeABSOutput is the Huma output for analyzing an ABS backup.
type AnalyzeABSOutput struct {
	Body AnalyzeABSResponse
}

// AsyncAnalyzeOutput is the response for starting an async analysis.
type AsyncAnalyzeOutput struct {
	Body struct {
		AnalysisID string `json:"analysis_id" doc:"Unique ID for polling progress"`
	}
}

// AnalysisStatusInput is the input for getting analysis status.
type AnalysisStatusInput struct {
	Authorization string `header:"Authorization"`
	AnalysisID    string `path:"analysisId" doc:"Analysis ID from async analyze endpoint"`
}

// AnalysisStatusResponse is the body of the status response.
type AnalysisStatusResponse struct {
	Status  string              `json:"status" doc:"running, completed, or failed"`
	Phase   string              `json:"phase" doc:"Current phase: parsing, matching_users, matching_books, matching_sessions, matching_progress, done"`
	Current int                 `json:"current" doc:"Current item in phase"`
	Total   int                 `json:"total" doc:"Total items in phase"`
	Result  *AnalyzeABSResponse `json:"result,omitempty" doc:"Analysis result when completed"`
	Error   string              `json:"error,omitempty" doc:"Error message if failed"`
}

// AnalysisStatusOutput wraps the status response.
type AnalysisStatusOutput struct {
	Body AnalysisStatusResponse
}

// ImportABSRequest is the request body for importing from an ABS backup.
type ImportABSRequest struct {
	BackupPath      string            `json:"backup_path" doc:"Path to .audiobookshelf backup file"`
	UserMappings    map[string]string `json:"user_mappings" doc:"Final ABS user ID -> ListenUp user ID mappings"`
	BookMappings    map[string]string `json:"book_mappings" doc:"Final ABS item ID -> ListenUp book ID mappings"`
	ImportSessions  bool              `json:"import_sessions" required:"false" default:"true" doc:"Import listening session history"`
	ImportProgress  bool              `json:"import_progress" required:"false" default:"true" doc:"Import current progress state"`
	RebuildProgress bool              `json:"rebuild_progress" required:"false" default:"true" doc:"Rebuild progress after import"`
}

// ImportABSInput is the Huma input for importing from an ABS backup.
type ImportABSInput struct {
	Authorization string `header:"Authorization"`
	Body          ImportABSRequest
}

// ImportABSResponse is the response from importing an ABS backup.
type ImportABSResponse struct {
	SessionsImported int      `json:"sessions_imported" doc:"Number of sessions imported"`
	SessionsSkipped  int      `json:"sessions_skipped" doc:"Number of sessions skipped"`
	ProgressImported int      `json:"progress_imported" doc:"Number of progress records imported"`
	ProgressSkipped  int      `json:"progress_skipped" doc:"Number of progress records skipped"`
	EventsCreated    int      `json:"events_created" doc:"Total listening events created"`
	AffectedUsers    int      `json:"affected_users" doc:"Number of users whose progress was affected"`
	Duration         string   `json:"duration" doc:"Import duration"`
	Warnings         []string `json:"warnings,omitempty" doc:"Non-fatal warnings during import"`
	Errors           []string `json:"errors,omitempty" doc:"Non-fatal errors during import"`
}

// ImportABSOutput is the Huma output for importing from an ABS backup.
type ImportABSOutput struct {
	Body ImportABSResponse
}

// UploadABSResponse is the response from uploading an ABS backup.
type UploadABSResponse struct {
	Path string `json:"path" doc:"Server path where the backup was saved"`
}

// === Handlers ===

func (s *Server) handleAnalyzeABSBackup(ctx context.Context, input *AnalyzeABSInput) (*AnalyzeABSOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate backup path
	if input.Body.BackupPath == "" {
		return nil, huma.Error400BadRequest("backup_path is required")
	}

	// Check file exists
	if _, err := os.Stat(input.Body.BackupPath); os.IsNotExist(err) {
		return nil, huma.Error400BadRequest("backup file not found: " + input.Body.BackupPath)
	}

	// Parse the backup
	backup, err := abs.Parse(input.Body.BackupPath)
	if err != nil {
		return nil, huma.Error400BadRequest("failed to parse ABS backup: " + err.Error())
	}

	// Build analysis options
	opts := abs.AnalysisOptions{
		UserMappings:    input.Body.UserMappings,
		BookMappings:    input.Body.BookMappings,
		MatchByEmail:    input.Body.MatchByEmail,
		MatchByPath:     input.Body.MatchByPath,
		FuzzyMatchBooks: input.Body.FuzzyMatchBooks,
		FuzzyThreshold:  input.Body.FuzzyThreshold,
	}
	if opts.UserMappings == nil {
		opts.UserMappings = make(map[string]string)
	}
	if opts.BookMappings == nil {
		opts.BookMappings = make(map[string]string)
	}
	if opts.FuzzyThreshold == 0 {
		opts.FuzzyThreshold = 0.85
	}

	// Run analysis
	analyzer := abs.NewAnalyzer(s.store, s.logger, opts)
	result, err := analyzer.Analyze(ctx, backup)
	if err != nil {
		return nil, huma.Error500InternalServerError("analysis failed", err)
	}

	// Convert to API response
	resp := s.buildAnalyzeResponse(result, backup.Summary())
	return &AnalyzeABSOutput{Body: resp}, nil
}

// buildAnalyzeResponse converts an analysis result to the API response type.
func (s *Server) buildAnalyzeResponse(result *abs.AnalysisResult, backupSummary string) AnalyzeABSResponse {
	resp := AnalyzeABSResponse{
		BackupPath:      result.BackupPath,
		AnalyzedAt:      result.AnalyzedAt.Format(time.RFC3339),
		Summary:         backupSummary,
		TotalUsers:      result.TotalUsers,
		TotalBooks:      result.TotalBooks,
		TotalSessions:   result.TotalSessions,
		UsersMatched:    result.UsersMatched,
		UsersPending:    result.UsersPending,
		BooksMatched:    result.BooksMatched,
		BooksPending:    result.BooksPending,
		SessionsReady:   result.SessionsReady,
		SessionsPending: result.SessionsPending,
		ProgressReady:   result.ProgressReady,
		ProgressPending: result.ProgressPending,
		Warnings:        result.Warnings,
	}

	// Convert user matches
	resp.UserMatches = make([]ABSUserMatchResponse, len(result.UserMatches))
	for i, m := range result.UserMatches {
		resp.UserMatches[i] = ABSUserMatchResponse{
			ABSUserID:   m.ABSUser.ID,
			ABSUsername: m.ABSUser.Username,
			ABSEmail:    m.ABSUser.Email,
			ListenUpID:  m.ListenUpID,
			Confidence:  m.Confidence.String(),
			MatchReason: m.MatchReason,
		}
		for _, sg := range m.Suggestions {
			resp.UserMatches[i].Suggestions = append(resp.UserMatches[i].Suggestions, ABSUserSuggestion{
				UserID:      sg.UserID,
				Email:       sg.Email,
				DisplayName: sg.DisplayName,
				Score:       sg.Score,
				Reason:      sg.Reason,
			})
		}
	}

	// Convert book matches
	// Note: We use MediaID (books.id) as ABSItemID since sessions reference mediaItemId
	resp.BookMatches = make([]ABSBookMatchResponse, len(result.BookMatches))
	for i, m := range result.BookMatches {
		resp.BookMatches[i] = ABSBookMatchResponse{
			ABSItemID:   m.ABSItem.MediaID,
			ABSTitle:    m.ABSItem.Media.Metadata.Title,
			ABSAuthor:   m.ABSItem.Media.Metadata.PrimaryAuthor(),
			ListenUpID:  m.ListenUpID,
			Confidence:  m.Confidence.String(),
			MatchReason: m.MatchReason,
		}
		for _, sg := range m.Suggestions {
			resp.BookMatches[i].Suggestions = append(resp.BookMatches[i].Suggestions, ABSBookSuggestion{
				BookID:     sg.BookID,
				Title:      sg.Title,
				Author:     sg.Author,
				DurationMs: sg.DurationMs,
				Score:      sg.Score,
				Reason:     sg.Reason,
			})
		}
	}

	return resp
}

func (s *Server) handleAnalyzeABSBackupAsync(ctx context.Context, input *AnalyzeABSInput) (*AsyncAnalyzeOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate backup path
	if input.Body.BackupPath == "" {
		return nil, huma.Error400BadRequest("backup_path is required")
	}

	// Check file exists
	if _, err := os.Stat(input.Body.BackupPath); os.IsNotExist(err) {
		return nil, huma.Error400BadRequest("backup file not found: " + input.Body.BackupPath)
	}

	// Build analysis options
	opts := abs.AnalysisOptions{
		UserMappings:    input.Body.UserMappings,
		BookMappings:    input.Body.BookMappings,
		MatchByEmail:    input.Body.MatchByEmail,
		MatchByPath:     input.Body.MatchByPath,
		FuzzyMatchBooks: input.Body.FuzzyMatchBooks,
		FuzzyThreshold:  input.Body.FuzzyThreshold,
	}
	if opts.UserMappings == nil {
		opts.UserMappings = make(map[string]string)
	}
	if opts.BookMappings == nil {
		opts.BookMappings = make(map[string]string)
	}
	if opts.FuzzyThreshold == 0 {
		opts.FuzzyThreshold = 0.85
	}

	analysisID := uuid.New().String()
	progress := s.analysisTracker.Start(analysisID)

	go func() {
		// Parse the backup
		backup, err := abs.Parse(input.Body.BackupPath)
		if err != nil {
			progress.Fail("failed to parse ABS backup: " + err.Error())
			return
		}

		// Create analyzer with progress callback
		analyzer := abs.NewAnalyzer(s.store, s.logger, opts)
		analyzer.WithProgress(func(phase abs.AnalysisPhase, current, total int) {
			progress.Update(phase, current, total)
		})

		// Run analysis (use background context since the HTTP request context will be cancelled)
		result, err := analyzer.Analyze(context.Background(), backup)
		if err != nil {
			progress.Fail("analysis failed: " + err.Error())
			return
		}

		// Convert to API response and store
		resp := s.buildAnalyzeResponse(result, backup.Summary())
		progress.Complete(resp)
	}()

	out := &AsyncAnalyzeOutput{}
	out.Body.AnalysisID = analysisID
	return out, nil
}

func (s *Server) handleGetAnalysisStatus(ctx context.Context, input *AnalysisStatusInput) (*AnalysisStatusOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	progress := s.analysisTracker.Get(input.AnalysisID)
	if progress == nil {
		return nil, huma.Error404NotFound("analysis not found")
	}

	snap := progress.Snapshot()

	out := &AnalysisStatusOutput{
		Body: AnalysisStatusResponse{
			Status:  string(snap.Status),
			Phase:   string(snap.Phase),
			Current: snap.Current,
			Total:   snap.Total,
			Error:   snap.Error,
		},
	}

	if snap.Status == abs.StatusCompleted {
		if resp, ok := snap.Result.(AnalyzeABSResponse); ok {
			out.Body.Result = &resp
		}
	}

	return out, nil
}

func (s *Server) handleImportABSBackup(ctx context.Context, input *ImportABSInput) (*ImportABSOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate backup path
	if input.Body.BackupPath == "" {
		return nil, huma.Error400BadRequest("backup_path is required")
	}

	// Check file exists
	if _, err := os.Stat(input.Body.BackupPath); os.IsNotExist(err) {
		return nil, huma.Error400BadRequest("backup file not found: " + input.Body.BackupPath)
	}

	// Require at least some mappings
	if len(input.Body.UserMappings) == 0 {
		return nil, huma.Error400BadRequest("user_mappings is required (run analyze first)")
	}
	if len(input.Body.BookMappings) == 0 {
		return nil, huma.Error400BadRequest("book_mappings is required (run analyze first)")
	}

	// Parse the backup
	backup, err := abs.Parse(input.Body.BackupPath)
	if err != nil {
		return nil, huma.Error400BadRequest("failed to parse ABS backup: " + err.Error())
	}

	// Build import options
	opts := abs.ImportOptions{
		UserMappings:   input.Body.UserMappings,
		BookMappings:   input.Body.BookMappings,
		ImportSessions: input.Body.ImportSessions,
		ImportProgress: input.Body.ImportProgress,
		SkipUnmatched:  true,
	}

	// Run import
	importer := abs.NewImporter(s.store, s.sseManager, s.logger)
	result, err := importer.Import(ctx, backup, opts.UserMappings, opts.BookMappings, opts)
	if err != nil {
		return nil, huma.Error500InternalServerError("import failed", err)
	}

	// Rebuild progress if requested
	if input.Body.RebuildProgress && len(result.AffectedUserIDs) > 0 {
		if err := importer.RebuildProgressForUsers(ctx, result.AffectedUserIDs); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to rebuild progress: %v", err))
		}
	}

	return &ImportABSOutput{
		Body: ImportABSResponse{
			SessionsImported: result.SessionsImported,
			SessionsSkipped:  result.SessionsSkipped,
			ProgressImported: result.ProgressImported,
			ProgressSkipped:  result.ProgressSkipped,
			EventsCreated:    result.EventsCreated,
			AffectedUsers:    len(result.AffectedUserIDs),
			Duration:         result.Duration.String(),
			Warnings:         result.Warnings,
			Errors:           result.Errors,
		},
	}, nil
}

// withExtendedTimeout wraps a handler to extend read/write timeouts for large uploads.
// This MUST be called before any body reading occurs.
func withExtendedTimeout(next http.HandlerFunc, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		if err := rc.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			// Log but continue - some servers may not support this
			_ = err
		}
		if err := rc.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			_ = err
		}
		next(w, r)
	}
}

// handleUploadABSBackup handles multipart file uploads for ABS backups.
// This is a chi handler (not Huma) because Huma doesn't easily support multipart forms.
// Note: Must be wrapped with withExtendedTimeout when registering the route.
func (s *Server) handleUploadABSBackup(w http.ResponseWriter, r *http.Request) {
	// Check authentication via context (set by auth middleware)
	ctx := r.Context()
	if _, err := s.RequireAdmin(ctx); err != nil {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}

	// Get uploads directory from backup service
	uploadsDir, err := s.backupService.GetUploadsDir()
	if err != nil {
		s.logger.Error("failed to get uploads directory", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Limit upload size (1GB for large ABS backups with many users/sessions)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	file, header, err := r.FormFile("backup")
	if err != nil {
		s.logger.Error("failed to get form file", "error", err)
		http.Error(w, "failed to get form file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create destination path with timestamp and original extension
	filename := fmt.Sprintf("abs-upload-%d%s", time.Now().UnixNano(), filepath.Ext(header.Filename))
	destPath := filepath.Join(uploadsDir, filename)

	dest, err := os.Create(destPath)
	if err != nil {
		s.logger.Error("failed to create destination file", "error", err, "path", destPath)
		http.Error(w, "failed to create destination file", http.StatusInternalServerError)
		return
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		os.Remove(destPath)
		s.logger.Error("failed to copy file", "error", err)
		http.Error(w, "failed to copy file", http.StatusInternalServerError)
		return
	}

	s.logger.Info("ABS backup uploaded", "path", destPath, "original_filename", header.Filename)

	// Return success response in the standard envelope format (with version field)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := fmt.Sprintf(`{"v":1,"success":true,"data":{"path":%q}}`, destPath)
	w.Write([]byte(response))
}
