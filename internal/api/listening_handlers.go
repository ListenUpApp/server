package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleRecordEvent records a listening event.
func (s *Server) handleRecordEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req service.RecordEventRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	resp, err := s.listeningService.RecordEvent(ctx, userID, req)
	if err != nil {
		s.logger.Error("Failed to record listening event", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to record event", s.logger)
		return
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

	progress, err := s.listeningService.GetProgress(ctx, userID, bookID)
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

	progress, err := s.listeningService.GetContinueListening(ctx, userID, limit)
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

	if err := s.listeningService.ResetProgress(ctx, userID, bookID); err != nil {
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

	settings, err := s.listeningService.GetUserSettings(ctx, userID)
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

	settings, err := s.listeningService.UpdateUserSettings(ctx, userID, req)
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

	prefs, err := s.listeningService.GetBookPreferences(ctx, userID, bookID)
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

	prefs, err := s.listeningService.UpdateBookPreferences(ctx, userID, bookID, req)
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

	stats, err := s.listeningService.GetUserStats(ctx, userID)
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

	stats, err := s.listeningService.GetBookStats(ctx, bookID)
	if err != nil {
		s.logger.Error("Failed to get book stats", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to get stats", s.logger)
		return
	}

	response.Success(w, stats, s.logger)
}
