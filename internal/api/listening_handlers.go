package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
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
		OperationID: "getContinueListening",
		Method:      http.MethodGet,
		Path:        "/api/v1/listening/continue",
		Summary:     "Get continue listening",
		Description: "Returns in-progress books for continue listening",
		Tags:        []string{"Listening"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetContinueListening)

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
}

// === DTOs ===

// RecordListeningEventRequest is the request body for recording a listening event.
type RecordListeningEventRequest struct {
	BookID          string   `json:"book_id" validate:"required" doc:"Book ID"`
	StartPositionMs int64    `json:"start_position_ms" validate:"gte=0" doc:"Start position in ms"`
	EndPositionMs   int64    `json:"end_position_ms" validate:"gte=0,gtefield=StartPositionMs" doc:"End position in ms"`
	StartedAt       FlexTime `json:"started_at" validate:"required" doc:"When playback started (RFC3339 or epoch ms)"`
	EndedAt         FlexTime `json:"ended_at" validate:"required" doc:"When playback ended (RFC3339 or epoch ms)"`
	PlaybackSpeed   float32  `json:"playback_speed" validate:"gt=0,lte=4" doc:"Playback speed"`
	DeviceID        string   `json:"device_id" validate:"required,max=100" doc:"Device ID"`
	DeviceName      string   `json:"device_name" validate:"omitempty,max=100" doc:"Device name"`
}

// RecordListeningEventInput wraps the record listening event request for Huma.
type RecordListeningEventInput struct {
	Authorization string `header:"Authorization"`
	Body          RecordListeningEventRequest
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

// RecordListeningEventResponse contains the event and updated progress.
type RecordListeningEventResponse struct {
	Event    ListeningEventResponse `json:"event" doc:"Created event"`
	Progress ProgressResponse       `json:"progress" doc:"Updated progress"`
}

// RecordListeningEventOutput wraps the record listening event response for Huma.
type RecordListeningEventOutput struct {
	Body RecordListeningEventResponse
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

// GetUserStatsInput contains parameters for getting user stats.
type GetUserStatsInput struct {
	Authorization string `header:"Authorization"`
}

// UserStatsResponse contains user listening statistics.
type UserStatsResponse struct {
	TotalListenTimeMs int64 `json:"total_listen_time_ms" doc:"Total listen time"`
	BooksStarted      int   `json:"books_started" doc:"Books started"`
	BooksFinished     int   `json:"books_finished" doc:"Books finished"`
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	result, err := s.services.Listening.RecordEvent(ctx, userID, service.RecordEventRequest{
		BookID:          input.Body.BookID,
		StartPositionMs: input.Body.StartPositionMs,
		EndPositionMs:   input.Body.EndPositionMs,
		StartedAt:       input.Body.StartedAt.ToTime(),
		EndedAt:         input.Body.EndedAt.ToTime(),
		PlaybackSpeed:   input.Body.PlaybackSpeed,
		DeviceID:        input.Body.DeviceID,
		DeviceName:      input.Body.DeviceName,
	})
	if err != nil {
		return nil, err
	}

	// Get book for total duration
	book, err := s.store.GetBook(ctx, input.Body.BookID, userID)
	if err != nil {
		return nil, err
	}

	return &RecordListeningEventOutput{
		Body: RecordListeningEventResponse{
			Event: ListeningEventResponse{
				ID:              result.Event.ID,
				BookID:          result.Event.BookID,
				StartPositionMs: result.Event.StartPositionMs,
				EndPositionMs:   result.Event.EndPositionMs,
				DurationMs:      result.Event.DurationMs,
				StartedAt:       result.Event.StartedAt,
				EndedAt:         result.Event.EndedAt,
				PlaybackSpeed:   result.Event.PlaybackSpeed,
				DeviceID:        result.Event.DeviceID,
			},
			Progress: ProgressResponse{
				UserID:            result.Progress.UserID,
				BookID:            result.Progress.BookID,
				CurrentPositionMs: result.Progress.CurrentPositionMs,
				TotalDurationMs:   book.TotalDuration,
				Progress:          result.Progress.Progress,
				TotalListenTimeMs: result.Progress.TotalListenTimeMs,
				StartedAt:         result.Progress.StartedAt,
				LastPlayedAt:      result.Progress.LastPlayedAt,
				UpdatedAt:         result.Progress.UpdatedAt,
				IsFinished:        result.Progress.IsFinished,
			},
		},
	}, nil
}

func (s *Server) handleGetProgress(ctx context.Context, input *GetProgressInput) (*ProgressOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	progress, err := s.services.Listening.GetProgress(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	// Get book for total duration
	book, err := s.store.GetBook(ctx, input.ID, userID)
	if err != nil {
		return nil, err
	}

	return &ProgressOutput{
		Body: ProgressResponse{
			UserID:            progress.UserID,
			BookID:            progress.BookID,
			CurrentPositionMs: progress.CurrentPositionMs,
			TotalDurationMs:   book.TotalDuration,
			Progress:          progress.Progress,
			TotalListenTimeMs: progress.TotalListenTimeMs,
			StartedAt:         progress.StartedAt,
			LastPlayedAt:      progress.LastPlayedAt,
			UpdatedAt:         progress.UpdatedAt,
			IsFinished:        progress.IsFinished,
		},
	}, nil
}

func (s *Server) handleResetProgress(ctx context.Context, input *ResetProgressInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Listening.ResetProgress(ctx, userID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Progress reset"}}, nil
}

func (s *Server) handleGetContinueListening(ctx context.Context, input *GetContinueListeningInput) (*ContinueListeningOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	items, err := s.services.Listening.GetContinueListening(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

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

func (s *Server) handleGetUserStats(ctx context.Context, input *GetUserStatsInput) (*UserStatsOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	stats, err := s.services.Listening.GetUserStats(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserStatsOutput{
		Body: UserStatsResponse{
			TotalListenTimeMs: stats.TotalListenTimeMs,
			BooksStarted:      stats.BooksStarted,
			BooksFinished:     stats.BooksFinished,
		},
	}, nil
}

func (s *Server) handleGetBookStats(ctx context.Context, input *GetBookStatsInput) (*BookStatsOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
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
