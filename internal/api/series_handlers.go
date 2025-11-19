package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleListSeries returns a paginated list of series.
// Note: Series visibility is not filtered by ACL yet - returns all series.
// TODO: Filter to only show series with at least one accessible book.
func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	params := parsePaginationParams(r)

	series, err := s.syncService.GetSeriesForSync(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list series", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series", s.logger)
		return
	}

	response.Success(w, series, s.logger)
}

// handleGetSeries returns a single series by ID.
// Note: Series visibility is not filtered by ACL yet.
// TODO: Return 404 if user has no access to any books in this series.
func (s *Server) handleGetSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	series, err := s.store.GetSeries(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrSeriesNotFound) {
			response.NotFound(w, "Series not found", s.logger)
			return
		}
		s.logger.Error("Failed to get series", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series", s.logger)
		return
	}

	response.Success(w, series, s.logger)
}

// handleGetSeriesBooks returns all books in a series that the user can access.
func (s *Server) handleGetSeriesBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	// Get all books in the series
	allBooks, err := s.store.GetBooksBySeries(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get series books", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series books", s.logger)
		return
	}

	// Filter to only books the user can access
	accessibleBooks := make([]*domain.Book, 0, len(allBooks))
	for _, book := range allBooks {
		canAccess, err := s.store.CanUserAccessBook(ctx, userID, book.ID)
		if err != nil {
			s.logger.Warn("Failed to check book access", "book_id", book.ID, "user_id", userID, "error", err)
			continue
		}
		if canAccess {
			accessibleBooks = append(accessibleBooks, book)
		}
	}

	response.Success(w, map[string]interface{}{
		"series_id": id,
		"books":     accessibleBooks,
	}, s.logger)
}

// handleSyncSeries handles GET /api/v1/sync/series for syncing series.
func (s *Server) handleSyncSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	params := parsePaginationParams(r)

	series, err := s.syncService.GetSeriesForSync(ctx, params)
	if err != nil {
		s.logger.Error("Failed to sync series", "error", err)
		response.InternalError(w, "Failed to sync series", s.logger)
		return
	}

	response.Success(w, series, s.logger)
}
