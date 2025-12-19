package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
)

// millisToTime converts milliseconds since epoch to time.Time.
func millisToTime(millis int64) time.Time {
	return time.UnixMilli(millis)
}

// BulkEventsRequest wraps multiple listening events for batch submission.
type BulkEventsRequest struct {
	Events []BulkEventItem `json:"events"`
}

// BulkEventItem represents a single event in a bulk submission.
type BulkEventItem struct {
	ID              string  `json:"id"`
	BookID          string  `json:"book_id"`
	StartPositionMs int64   `json:"start_position_ms"`
	EndPositionMs   int64   `json:"end_position_ms"`
	StartedAt       int64   `json:"started_at"`
	EndedAt         int64   `json:"ended_at"`
	PlaybackSpeed   float32 `json:"playback_speed"`
	DeviceID        string  `json:"device_id"`
}

// BulkEventsResponse contains acknowledged and failed event IDs.
type BulkEventsResponse struct {
	Acknowledged []string `json:"acknowledged"`
	Failed       []string `json:"failed"`
}

// handleRecordEvent records listening events (supports both single and bulk).
func (s *Server) handleRecordEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	// Try to parse as bulk request first (client sends { "events": [...] })
	var bulkReq BulkEventsRequest
	if err := json.UnmarshalRead(r.Body, &bulkReq); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// If we have events, process as bulk
	if len(bulkReq.Events) > 0 {
		s.handleBulkEvents(w, r, userID, bulkReq)
		return
	}

	// Empty events array - nothing to do
	response.Success(w, BulkEventsResponse{
		Acknowledged: []string{},
		Failed:       []string{},
	}, s.logger)
}

// handleBulkEvents processes multiple events and returns acknowledged/failed IDs.
func (s *Server) handleBulkEvents(w http.ResponseWriter, r *http.Request, userID string, bulkReq BulkEventsRequest) {
	ctx := r.Context()
	resp := BulkEventsResponse{
		Acknowledged: make([]string, 0, len(bulkReq.Events)),
		Failed:       make([]string, 0),
	}

	for _, event := range bulkReq.Events {
		// Convert bulk item to service request
		req := service.RecordEventRequest{
			BookID:          event.BookID,
			StartPositionMs: event.StartPositionMs,
			EndPositionMs:   event.EndPositionMs,
			StartedAt:       millisToTime(event.StartedAt),
			EndedAt:         millisToTime(event.EndedAt),
			PlaybackSpeed:   event.PlaybackSpeed,
			DeviceID:        event.DeviceID,
			DeviceName:      "", // Client doesn't send this in bulk format
		}

		_, err := s.services.Listening.RecordEvent(ctx, userID, req)
		if err != nil {
			s.logger.Warn("Failed to record listening event",
				"error", err,
				"user_id", userID,
				"event_id", event.ID,
				"book_id", event.BookID,
			)
			resp.Failed = append(resp.Failed, event.ID)
		} else {
			resp.Acknowledged = append(resp.Acknowledged, event.ID)
		}
	}

	response.Success(w, resp, s.logger)
}

// handleGetProgress retrieves playback progress for a book.
func (s *Server) handleGetProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	bookID := chi.URLParam(r, "bookID")
	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	progress, err := s.services.Listening.GetProgress(ctx, userID, bookID)
	if errors.Is(err, store.ErrProgressNotFound) {
		response.NotFound(w, "Progress not found", s.logger)
		return
	}
	if err != nil {
		s.logger.Error("Failed to get progress", "error", err, "user_id", userID, "book_id", bookID)
		response.InternalError(w, "Failed to get progress", s.logger)
		return
	}

	response.Success(w, progress, s.logger)
}

// handleGetContinueListening returns books the user is currently listening to.
func (s *Server) handleGetContinueListening(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	// Parse limit from query params (default 10)
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	progress, err := s.services.Listening.GetContinueListening(ctx, userID, limit)
	if err != nil {
		s.logger.Error("Failed to get continue listening", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to get continue listening", s.logger)
		return
	}

	response.Success(w, progress, s.logger)
}

// handleResetProgress removes progress for a book.
func (s *Server) handleResetProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	bookID := chi.URLParam(r, "bookID")
	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	if err := s.services.Listening.ResetProgress(ctx, userID, bookID); err != nil {
		s.logger.Error("Failed to reset progress", "error", err, "user_id", userID, "book_id", bookID)
		response.InternalError(w, "Failed to reset progress", s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "ok"}, s.logger)
}

// handleGetUserSettings retrieves user playback settings.
func (s *Server) handleGetUserSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	settings, err := s.services.Listening.GetUserSettings(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user settings", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to get settings", s.logger)
		return
	}

	response.Success(w, settings, s.logger)
}

// handleUpdateUserSettings updates user playback settings.
func (s *Server) handleUpdateUserSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req service.UpdateUserSettingsRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	settings, err := s.services.Listening.UpdateUserSettings(ctx, userID, req)
	if err != nil {
		s.logger.Error("Failed to update user settings", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to update settings", s.logger)
		return
	}

	response.Success(w, settings, s.logger)
}

// handleGetBookPreferences retrieves per-book preferences.
func (s *Server) handleGetBookPreferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	bookID := chi.URLParam(r, "bookID")
	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	prefs, err := s.services.Listening.GetBookPreferences(ctx, userID, bookID)
	if err != nil {
		s.logger.Error("Failed to get book preferences", "error", err, "user_id", userID, "book_id", bookID)
		response.InternalError(w, "Failed to get preferences", s.logger)
		return
	}

	response.Success(w, prefs, s.logger)
}

// handleUpdateBookPreferences updates per-book preferences.
func (s *Server) handleUpdateBookPreferences(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	bookID := chi.URLParam(r, "bookID")
	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	var req service.UpdateBookPreferencesRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	prefs, err := s.services.Listening.UpdateBookPreferences(ctx, userID, bookID, req)
	if err != nil {
		s.logger.Error("Failed to update book preferences", "error", err, "user_id", userID, "book_id", bookID)
		response.InternalError(w, "Failed to update preferences", s.logger)
		return
	}

	response.Success(w, prefs, s.logger)
}

// handleGetUserStats retrieves listening statistics for the user.
func (s *Server) handleGetUserStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	stats, err := s.services.Listening.GetUserStats(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user stats", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to get stats", s.logger)
		return
	}

	response.Success(w, stats, s.logger)
}

// handleGetBookStats retrieves listening statistics for a book.
func (s *Server) handleGetBookStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	bookID := chi.URLParam(r, "bookID")
	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	stats, err := s.services.Listening.GetBookStats(ctx, bookID)
	if err != nil {
		s.logger.Error("Failed to get book stats", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to get stats", s.logger)
		return
	}

	response.Success(w, stats, s.logger)
}
