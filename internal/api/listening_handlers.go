package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerListeningRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "recordListeningEvent",
		Method:      http.MethodPost,
		Path:        "/api/v1/listening/events",
		Summary:     "Record listening event",
		Description: "Records a listening event and updates playback progress",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRecordListeningEvent)

	huma.Register(s.api, huma.Operation{
		OperationID: "getListeningEvents",
		Method:      http.MethodGet,
		Path:        "/api/v1/listening/events",
		Summary:     "Get listening events",
		Description: "Returns listening events for the current user, with optional since timestamp for delta sync",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetListeningEvents)

	huma.Register(s.api, huma.Operation{
		OperationID: "getProgress",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}/progress",
		Summary:     "Get book progress",
		Description: "Returns playback progress for a book",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetProgress)

	huma.Register(s.api, huma.Operation{
		OperationID: "resetProgress",
		Method:      http.MethodDelete,
		Path:        "/api/v1/books/{id}/progress",
		Summary:     "Reset book progress",
		Description: "Resets playback progress for a book",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleResetProgress)

	huma.Register(s.api, huma.Operation{
		OperationID: "markComplete",
		Method:      http.MethodPost,
		Path:        "/api/v1/books/{id}/progress/complete",
		Summary:     "Mark book as complete",
		Description: "Marks a book as finished regardless of current position",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMarkComplete)

	huma.Register(s.api, huma.Operation{
		OperationID: "discardProgress",
		Method:      http.MethodDelete,
		Path:        "/api/v1/books/{id}/progress/discard",
		Summary:     "Discard book progress",
		Description: "Removes playback progress for a book (DNF / start over)",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDiscardProgress)

	huma.Register(s.api, huma.Operation{
		OperationID: "restartBook",
		Method:      http.MethodPost,
		Path:        "/api/v1/books/{id}/progress/restart",
		Summary:     "Restart book",
		Description: "Resets a book to listen again from the beginning",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRestartBook)

	huma.Register(s.api, huma.Operation{
		OperationID: "getContinueListening",
		Method:      http.MethodGet,
		Path:        "/api/v1/listening/continue",
		Summary:     "Get continue listening",
		Description: "Returns in-progress books for continue listening",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetContinueListening)

	huma.Register(s.api, huma.Operation{
		OperationID: "getAllProgress",
		Method:      http.MethodGet,
		Path:        "/api/v1/listening/progress",
		Summary:     "Get all progress",
		Description: "Returns all playback progress for the current user (for sync)",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetAllProgress)

	huma.Register(s.api, huma.Operation{
		OperationID: "getUserStats",
		Method:      http.MethodGet,
		Path:        "/api/v1/listening/stats",
		Summary:     "Get user stats",
		Description: "Returns listening statistics for the current user",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetUserStats)

	huma.Register(s.api, huma.Operation{
		OperationID: "getBookStats",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}/stats",
		Summary:     "Get book stats",
		Description: "Returns listening statistics for a book",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBookStats)

	huma.Register(s.api, huma.Operation{
		OperationID: "endPlaybackSession",
		Method:      http.MethodPost,
		Path:        "/api/v1/listening/session/end",
		Summary:     "End playback session",
		Description: "Records activity when user pauses/stops playback. Called by client when playback ends.",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleEndPlaybackSession)
}

// === DTOs ===

// BatchListeningEventItem is a single event in a batch submission.
// Matches the client's ListeningEventRequest format.
// Uses int64 for timestamps because Huma/json/v2 doesn't handle custom UnmarshalJSON correctly.
type BatchListeningEventItem struct {
	ID              string  `json:"id" validate:"required" doc:"Client-generated event ID"`
	BookID          string  `json:"book_id" validate:"required" doc:"Book ID"`
	StartPositionMs int64   `json:"start_position_ms" validate:"gte=0" doc:"Start position in ms"`
	EndPositionMs   int64   `json:"end_position_ms" validate:"gte=0" doc:"End position in ms"`
	StartedAtMs     int64   `json:"started_at" validate:"required" doc:"When playback started (epoch ms)"`
	EndedAtMs       int64   `json:"ended_at" validate:"required" doc:"When playback ended (epoch ms)"`
	PlaybackSpeed   float32 `json:"playback_speed" validate:"gt=0,lte=4" doc:"Playback speed"`
	DeviceID        string  `json:"device_id" validate:"required,max=100" doc:"Device ID"`
	Source          string  `json:"source,omitempty" doc:"Event source: playback, import, or manual (default: playback)"`
}

// BatchListeningEventsRequest is the request body for submitting multiple listening events.
// Matches the client's ListeningEventsRequest format: {"events": [...]}.
type BatchListeningEventsRequest struct {
	Events []BatchListeningEventItem `json:"events" validate:"required,min=1,dive" doc:"List of listening events"`
}

// RecordListeningEventInput wraps the batch listening events request for Huma.
type RecordListeningEventInput struct {
	Authorization string `header:"Authorization"`
	Body          BatchListeningEventsRequest
}

// BatchListeningEventsResponse contains the result of batch event submission.
// Matches the client's ListeningEventsResponse format.
type BatchListeningEventsResponse struct {
	Acknowledged []string `json:"acknowledged" doc:"IDs of successfully processed events"`
	Failed       []string `json:"failed" doc:"IDs of events that failed to process"`
}

// RecordListeningEventOutput wraps the batch response for Huma.
type RecordListeningEventOutput struct {
	Body BatchListeningEventsResponse
}

// ListeningEventResponse contains listening event data in API responses.
type ListeningEventResponse struct {
	ID              string    `json:"id" doc:"Event ID"`
	BookID          string    `json:"book_id" doc:"Book ID"`
	StartPositionMs int64     `json:"start_position_ms" doc:"Start position"`
	EndPositionMs   int64     `json:"end_position_ms" doc:"End position"`
	DurationMs      int64     `json:"duration_ms" doc:"Duration"`
	StartedAt       time.Time `json:"started_at" doc:"Started at"`
	EndedAt         time.Time `json:"ended_at" doc:"Ended at"`
	PlaybackSpeed   float32   `json:"playback_speed" doc:"Playback speed"`
	DeviceID        string    `json:"device_id" doc:"Device ID"`
	Source          string    `json:"source" doc:"Event source: playback, import, or manual"`
}

// ProgressResponse contains playback progress data.
type ProgressResponse struct {
	UserID            string    `json:"user_id" doc:"User ID"`
	BookID            string    `json:"book_id" doc:"Book ID"`
	CurrentPositionMs int64     `json:"current_position_ms" doc:"Current position in ms"`
	TotalDurationMs   int64     `json:"total_duration_ms" doc:"Total duration in ms"`
	Progress          float64   `json:"progress" doc:"Progress 0-1"`
	TotalListenTimeMs int64     `json:"total_listen_time_ms" doc:"Total listen time"`
	StartedAt         time.Time `json:"started_at" doc:"When listening started"`
	LastPlayedAt      time.Time `json:"last_played_at" doc:"Last played time"`
	UpdatedAt         time.Time `json:"updated_at" doc:"Last update time"`
	IsFinished        bool      `json:"is_finished" doc:"Whether finished"`
}

// GetProgressInput contains parameters for getting book progress.
type GetProgressInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

// ProgressOutput wraps the progress response for Huma.
type ProgressOutput struct {
	Body ProgressResponse
}

// ResetProgressInput contains parameters for resetting book progress.
type ResetProgressInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

// MarkCompleteRequest contains the request body for marking a book complete.
type MarkCompleteRequest struct {
	FinishedAt *string `json:"finished_at,omitempty" doc:"When finished (ISO 8601). Defaults to now."`
}

// MarkCompleteInput contains parameters for marking a book complete.
type MarkCompleteInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          MarkCompleteRequest
}

// MarkCompleteResponse contains the updated progress after marking complete.
type MarkCompleteResponse struct {
	BookID            string  `json:"book_id" doc:"Book ID"`
	IsFinished        bool    `json:"is_finished" doc:"Whether finished"`
	FinishedAt        *string `json:"finished_at,omitempty" doc:"When finished (ISO 8601)"`
	CurrentPositionMs int64   `json:"current_position_ms" doc:"Current position in ms"`
	UpdatedAt         string  `json:"updated_at" doc:"Last update (ISO 8601)"`
}

// MarkCompleteOutput wraps the mark complete response for Huma.
type MarkCompleteOutput struct {
	Body MarkCompleteResponse
}

// DiscardProgressInput contains parameters for discarding book progress.
type DiscardProgressInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	KeepHistory   bool   `query:"keep_history" default:"true" doc:"Keep listening history (default true)"`
}

// RestartBookInput contains parameters for restarting a book.
type RestartBookInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

// RestartBookResponse contains the updated progress after restarting.
type RestartBookResponse struct {
	BookID            string `json:"book_id" doc:"Book ID"`
	IsFinished        bool   `json:"is_finished" doc:"Whether finished (always false)"`
	CurrentPositionMs int64  `json:"current_position_ms" doc:"Current position in ms (always 0)"`
	UpdatedAt         string `json:"updated_at" doc:"Last update (ISO 8601)"`
}

// RestartBookOutput wraps the restart book response for Huma.
type RestartBookOutput struct {
	Body RestartBookResponse
}

// GetContinueListeningInput contains parameters for getting continue listening items.
type GetContinueListeningInput struct {
	Authorization string `header:"Authorization"`
	Limit         int    `query:"limit" doc:"Max items (default 10)"`
}

// ContinueListeningItemResponse represents an item in continue listening.
type ContinueListeningItemResponse struct {
	BookID            string    `json:"book_id" doc:"Book ID"`
	Title             string    `json:"title" doc:"Book title"`
	AuthorName        string    `json:"author_name,omitempty" doc:"Author name"`
	CoverPath         *string   `json:"cover_path,omitempty" doc:"Cover path"`
	CoverBlurHash     *string   `json:"cover_blur_hash,omitempty" doc:"Cover blur hash"`
	CurrentPositionMs int64     `json:"current_position_ms" doc:"Current position"`
	TotalDurationMs   int64     `json:"total_duration_ms" doc:"Total duration"`
	Progress          float64   `json:"progress" doc:"Progress 0-1"`
	LastPlayedAt      time.Time `json:"last_played_at" doc:"Last played"`
}

// ContinueListeningResponse contains continue listening items.
type ContinueListeningResponse struct {
	Items []ContinueListeningItemResponse `json:"items" doc:"Continue listening items"`
}

// ContinueListeningOutput wraps the continue listening response for Huma.
type ContinueListeningOutput struct {
	Body ContinueListeningResponse
}

// GetAllProgressInput contains parameters for getting all progress.
type GetAllProgressInput struct {
	Authorization string `header:"Authorization"`
}

// ProgressSyncItem represents a single progress record for sync.
type ProgressSyncItem struct {
	BookID            string  `json:"book_id" doc:"Book ID"`
	CurrentPositionMs int64   `json:"current_position_ms" doc:"Current position in ms"`
	IsFinished        bool    `json:"is_finished" doc:"Whether finished"`
	FinishedAt        *string `json:"finished_at,omitempty" doc:"When finished (ISO 8601)"`
	LastPlayedAt      string  `json:"last_played_at" doc:"Last played (ISO 8601)"`
	UpdatedAt         string  `json:"updated_at" doc:"Last update (ISO 8601)"`
}

// AllProgressResponse contains all progress records for sync.
type AllProgressResponse struct {
	Items []ProgressSyncItem `json:"items" doc:"Progress items"`
}

// AllProgressOutput wraps the all progress response for Huma.
type AllProgressOutput struct {
	Body AllProgressResponse
}

// GetUserStatsInput contains parameters for getting user stats.
type GetUserStatsInput struct {
	Authorization string `header:"Authorization"`
	Period        string `query:"period" enum:"day,week,month,year,all" default:"week" doc:"Time period for stats"`
}

// DailyListeningResponse represents a day's listening for API response.
type DailyListeningResponse struct {
	Date          string `json:"date" doc:"Date in YYYY-MM-DD format"`
	ListenTimeMs  int64  `json:"listen_time_ms" doc:"Total listen time"`
	BooksListened int    `json:"books_listened" doc:"Distinct books"`
}

// GenreListeningResponse represents genre breakdown for API response.
type GenreListeningResponse struct {
	GenreSlug    string  `json:"genre_slug" doc:"Genre slug"`
	GenreName    string  `json:"genre_name" doc:"Display name"`
	ListenTimeMs int64   `json:"listen_time_ms" doc:"Time spent"`
	Percentage   float64 `json:"percentage" doc:"Percentage of total"`
}

// StreakDayResponse represents a day in the streak calendar.
type StreakDayResponse struct {
	Date         string `json:"date" doc:"Date in YYYY-MM-DD format"`
	HasListened  bool   `json:"has_listened" doc:"Met minimum threshold"`
	ListenTimeMs int64  `json:"listen_time_ms" doc:"Total time"`
	Intensity    int    `json:"intensity" doc:"0-4 for visual gradient"`
}

// UserStatsResponse contains comprehensive user listening statistics.
type UserStatsResponse struct {
	Period            string `json:"period" doc:"Query period"`
	StartDate         string `json:"start_date" doc:"Period start (RFC3339)"`
	EndDate           string `json:"end_date" doc:"Period end (RFC3339)"`
	TotalListenTimeMs int64  `json:"total_listen_time_ms" doc:"Total listen time"`
	BooksStarted      int    `json:"books_started" doc:"Books started in period"`
	BooksFinished     int    `json:"books_finished" doc:"Books finished in period"`
	CurrentStreakDays int    `json:"current_streak_days" doc:"Current streak"`
	LongestStreakDays int    `json:"longest_streak_days" doc:"Longest ever streak"`

	DailyListening []DailyListeningResponse `json:"daily_listening" doc:"Daily breakdown"`
	GenreBreakdown []GenreListeningResponse `json:"genre_breakdown" doc:"Top genres"`
	StreakCalendar []StreakDayResponse      `json:"streak_calendar,omitempty" doc:"Past 12 weeks"`
}

// UserStatsOutput wraps the user stats response for Huma.
type UserStatsOutput struct {
	Body UserStatsResponse
}

// GetBookStatsInput contains parameters for getting book stats.
type GetBookStatsInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

// GetListeningEventsInput contains parameters for fetching listening events.
type GetListeningEventsInput struct {
	Authorization string `header:"Authorization"`
	Since         int64  `query:"since" doc:"Only return events created after this timestamp (epoch ms). Use 0 for all events."`
}

// GetListeningEventsResponse contains the list of listening events.
type GetListeningEventsResponse struct {
	Events []ListeningEventResponse `json:"events" doc:"List of listening events"`
}

// GetListeningEventsOutput wraps the listening events response for Huma.
type GetListeningEventsOutput struct {
	Body GetListeningEventsResponse
}

// BookStatsResponse contains book listening statistics.
type BookStatsResponse struct {
	TotalListenTimeMs int64 `json:"total_listen_time_ms" doc:"Total listen time"`
	TotalListeners    int   `json:"total_listeners" doc:"Total listeners"`
	CompletedCount    int   `json:"completed_count" doc:"Times completed"`
}

// BookStatsOutput wraps the book stats response for Huma.
type BookStatsOutput struct {
	Body BookStatsResponse
}

// === Handlers ===

func (s *Server) handleRecordListeningEvent(ctx context.Context, input *RecordListeningEventInput) (*RecordListeningEventOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	slog.Info("listening events received",
		"user_id", userID,
		"event_count", len(input.Body.Events),
	)

	// Initialize as empty slices (not nil) so JSON marshals to [] instead of null.
	// Nil slices serialize to null which causes client parsing errors.
	acknowledged := []string{}
	failed := []string{}

	// Process each event in the batch
	for _, event := range input.Body.Events {
		// Log timestamps to debug stats calculation
		startedAt := time.UnixMilli(event.StartedAtMs)
		endedAt := time.UnixMilli(event.EndedAtMs)
		slog.Info("processing listening event",
			"event_id", event.ID,
			"book_id", event.BookID,
			"start_position_ms", event.StartPositionMs,
			"end_position_ms", event.EndPositionMs,
			"started_at_ms", event.StartedAtMs,
			"ended_at_ms", event.EndedAtMs,
			"started_at", startedAt.Format(time.RFC3339),
			"ended_at", endedAt.Format(time.RFC3339),
			"device_id", event.DeviceID,
		)
		_, err := s.services.Listening.RecordEvent(ctx, userID, service.RecordEventRequest{
			EventID:         event.ID, // Client-provided ID for idempotency
			BookID:          event.BookID,
			StartPositionMs: event.StartPositionMs,
			EndPositionMs:   event.EndPositionMs,
			StartedAt:       time.UnixMilli(event.StartedAtMs),
			EndedAt:         time.UnixMilli(event.EndedAtMs),
			PlaybackSpeed:   event.PlaybackSpeed,
			DeviceID:        event.DeviceID,
			Source:          event.Source,
		})
		if err != nil {
			// Log but don't fail the whole batch
			failed = append(failed, event.ID)
		} else {
			acknowledged = append(acknowledged, event.ID)
		}
	}

	return &RecordListeningEventOutput{
		Body: BatchListeningEventsResponse{
			Acknowledged: acknowledged,
			Failed:       failed,
		},
	}, nil
}

func (s *Server) handleGetProgress(ctx context.Context, input *GetProgressInput) (*ProgressOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	progress, err := s.services.Listening.GetProgress(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	// Get book for total duration (no access check - if user has progress, they had access)
	book, err := s.store.GetBookNoAccessCheck(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &ProgressOutput{
		Body: ProgressResponse{
			UserID:            progress.UserID,
			BookID:            progress.BookID,
			CurrentPositionMs: progress.CurrentPositionMs,
			TotalDurationMs:   book.TotalDuration,
			Progress:          progress.ComputeProgress(book.TotalDuration),
			TotalListenTimeMs: progress.TotalListenTimeMs,
			StartedAt:         progress.StartedAt,
			LastPlayedAt:      progress.LastPlayedAt,
			UpdatedAt:         progress.UpdatedAt,
			IsFinished:        progress.IsFinished,
		},
	}, nil
}

func (s *Server) handleResetProgress(ctx context.Context, input *ResetProgressInput) (*MessageOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Listening.ResetProgress(ctx, userID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Progress reset"}}, nil
}

func (s *Server) handleMarkComplete(ctx context.Context, input *MarkCompleteInput) (*MarkCompleteOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Parse optional finished_at timestamp
	var finishedAt *time.Time
	if input.Body.FinishedAt != nil && *input.Body.FinishedAt != "" {
		parsed, err := time.Parse(time.RFC3339, *input.Body.FinishedAt)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid finished_at format, expected RFC3339")
		}
		finishedAt = &parsed
	}

	state, err := s.services.Listening.MarkComplete(ctx, userID, input.ID, finishedAt)
	if err != nil {
		return nil, err
	}

	// Format response
	var finishedAtStr *string
	if state.FinishedAt != nil {
		formatted := state.FinishedAt.Format(time.RFC3339)
		finishedAtStr = &formatted
	}

	return &MarkCompleteOutput{
		Body: MarkCompleteResponse{
			BookID:            state.BookID,
			IsFinished:        state.IsFinished,
			FinishedAt:        finishedAtStr,
			CurrentPositionMs: state.CurrentPositionMs,
			UpdatedAt:         state.UpdatedAt.Format(time.RFC3339),
		},
	}, nil
}

func (s *Server) handleDiscardProgress(ctx context.Context, input *DiscardProgressInput) (*MessageOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Listening.DiscardProgress(ctx, userID, input.ID, input.KeepHistory); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Progress discarded"}}, nil
}

func (s *Server) handleRestartBook(ctx context.Context, input *RestartBookInput) (*RestartBookOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	state, err := s.services.Listening.RestartBook(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	return &RestartBookOutput{
		Body: RestartBookResponse{
			BookID:            state.BookID,
			IsFinished:        state.IsFinished,
			CurrentPositionMs: state.CurrentPositionMs,
			UpdatedAt:         state.UpdatedAt.Format(time.RFC3339),
		},
	}, nil
}

func (s *Server) handleGetContinueListening(ctx context.Context, input *GetContinueListeningInput) (*ContinueListeningOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	slog.Info("fetching continue listening",
		"user_id", userID,
		"limit", limit,
	)

	items, err := s.services.Listening.GetContinueListening(ctx, userID, limit)
	if err != nil {
		slog.Error("continue listening failed", "error", err)
		return nil, err
	}

	slog.Info("continue listening result",
		"user_id", userID,
		"item_count", len(items),
	)

	resp := make([]ContinueListeningItemResponse, len(items))
	for i, item := range items {
		resp[i] = ContinueListeningItemResponse{
			BookID:            item.BookID,
			Title:             item.Title,
			AuthorName:        item.AuthorName,
			CoverPath:         item.CoverPath,
			CoverBlurHash:     item.CoverBlurHash,
			CurrentPositionMs: item.CurrentPositionMs,
			TotalDurationMs:   item.TotalDurationMs,
			Progress:          item.Progress,
			LastPlayedAt:      item.LastPlayedAt,
		}
	}

	return &ContinueListeningOutput{Body: ContinueListeningResponse{Items: resp}}, nil
}

func (s *Server) handleGetAllProgress(ctx context.Context, input *GetAllProgressInput) (*AllProgressOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	slog.Info("fetching all state for sync", "user_id", userID)

	progressRecords, err := s.store.GetStateForUser(ctx, userID)
	if err != nil {
		slog.Error("get all progress failed", "error", err)
		return nil, err
	}

	slog.Info("all progress result",
		"user_id", userID,
		"record_count", len(progressRecords),
	)

	items := make([]ProgressSyncItem, len(progressRecords))
	for i, p := range progressRecords {
		item := ProgressSyncItem{
			BookID:            p.BookID,
			CurrentPositionMs: p.CurrentPositionMs,
			IsFinished:        p.IsFinished,
			LastPlayedAt:      p.LastPlayedAt.Format(time.RFC3339),
			UpdatedAt:         p.UpdatedAt.Format(time.RFC3339),
		}
		if p.FinishedAt != nil {
			finishedAt := p.FinishedAt.Format(time.RFC3339)
			item.FinishedAt = &finishedAt
		}
		items[i] = item
	}

	return &AllProgressOutput{Body: AllProgressResponse{Items: items}}, nil
}

func (s *Server) handleGetUserStats(ctx context.Context, input *GetUserStatsInput) (*UserStatsOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	period := domain.StatsPeriod(input.Period)
	if !period.Valid() {
		period = domain.StatsPeriodWeek
	}

	stats, err := s.services.Stats.GetUserStats(ctx, userID, period)
	if err != nil {
		return nil, err
	}

	// Convert to response format
	resp := UserStatsResponse{
		Period:            string(stats.Period),
		StartDate:         stats.StartDate.Format(time.RFC3339),
		EndDate:           stats.EndDate.Format(time.RFC3339),
		TotalListenTimeMs: stats.TotalListenTimeMs,
		BooksStarted:      stats.BooksStarted,
		BooksFinished:     stats.BooksFinished,
		CurrentStreakDays: stats.CurrentStreakDays,
		LongestStreakDays: stats.LongestStreakDays,
	}

	// Convert daily listening
	resp.DailyListening = make([]DailyListeningResponse, len(stats.DailyListening))
	for i, d := range stats.DailyListening {
		resp.DailyListening[i] = DailyListeningResponse{
			Date:          d.Date.Format("2006-01-02"),
			ListenTimeMs:  d.ListenTimeMs,
			BooksListened: d.BooksListened,
		}
	}

	// Convert genre breakdown
	resp.GenreBreakdown = make([]GenreListeningResponse, len(stats.GenreBreakdown))
	for i, g := range stats.GenreBreakdown {
		resp.GenreBreakdown[i] = GenreListeningResponse{
			GenreSlug:    g.GenreSlug,
			GenreName:    g.GenreName,
			ListenTimeMs: g.ListenTimeMs,
			Percentage:   g.Percentage,
		}
	}

	// Convert streak calendar
	if stats.StreakCalendar != nil {
		resp.StreakCalendar = make([]StreakDayResponse, len(stats.StreakCalendar))
		for i, day := range stats.StreakCalendar {
			resp.StreakCalendar[i] = StreakDayResponse{
				Date:         day.Date.Format("2006-01-02"),
				HasListened:  day.HasListened,
				ListenTimeMs: day.ListenTimeMs,
				Intensity:    day.Intensity,
			}
		}
	}

	return &UserStatsOutput{Body: resp}, nil
}

func (s *Server) handleGetBookStats(ctx context.Context, input *GetBookStatsInput) (*BookStatsOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	stats, err := s.services.Listening.GetBookStats(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &BookStatsOutput{
		Body: BookStatsResponse{
			TotalListenTimeMs: stats.TotalListenTimeMs,
			TotalListeners:    stats.TotalListeners,
			CompletedCount:    stats.CompletedCount,
		},
	}, nil
}

func (s *Server) handleGetListeningEvents(ctx context.Context, input *GetListeningEventsInput) (*GetListeningEventsOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Get events for user, optionally filtered by since timestamp
	var events []*domain.ListeningEvent
	if input.Since > 0 {
		since := time.UnixMilli(input.Since)
		events, err = s.store.GetEventsForUserInRange(ctx, userID, since, time.Now().Add(time.Hour))
	} else {
		events, err = s.store.GetEventsForUser(ctx, userID)
	}
	if err != nil {
		return nil, err
	}

	// Convert to response format
	resp := make([]ListeningEventResponse, len(events))
	for i, e := range events {
		resp[i] = ListeningEventResponse{
			ID:              e.ID,
			BookID:          e.BookID,
			StartPositionMs: e.StartPositionMs,
			EndPositionMs:   e.EndPositionMs,
			DurationMs:      e.DurationMs,
			StartedAt:       e.StartedAt,
			EndedAt:         e.EndedAt,
			PlaybackSpeed:   e.PlaybackSpeed,
			DeviceID:        e.DeviceID,
			Source:          e.Source,
		}
	}

	slog.Info("fetched listening events",
		"user_id", userID,
		"since", input.Since,
		"count", len(resp),
	)

	return &GetListeningEventsOutput{
		Body: GetListeningEventsResponse{Events: resp},
	}, nil
}

// EndPlaybackSessionRequest is the request body for ending a playback session.
type EndPlaybackSessionRequest struct {
	BookID     string `json:"book_id" validate:"required" doc:"Book that was being played"`
	DurationMs int64  `json:"duration_ms" validate:"gte=0" doc:"Duration listened in this session (ms)"`
}

// EndPlaybackSessionInput wraps the end playback session request for Huma.
type EndPlaybackSessionInput struct {
	Authorization string `header:"Authorization"`
	Body          EndPlaybackSessionRequest
}

func (s *Server) handleEndPlaybackSession(ctx context.Context, input *EndPlaybackSessionInput) (*MessageOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	slog.Info("ending playback session",
		"user_id", userID,
		"book_id", input.Body.BookID,
		"duration_ms", input.Body.DurationMs,
	)

	if err := s.services.ReadingSession.EndPlaybackSession(ctx, userID, input.Body.BookID, input.Body.DurationMs); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Playback session ended"}}, nil
}
