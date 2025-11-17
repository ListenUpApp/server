package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleListSeries returns a paginated list of series.
func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	params := parsePaginationParams(r)

	series, err := s.syncService.GetSeriesForSync(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list series", "error", err)
		response.InternalError(w, "Failed to retrieve series", s.logger)
		return
	}

	response.Success(w, series, s.logger)
}

// handleGetSeries returns a single series by ID.
func (s *Server) handleGetSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

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
		s.logger.Error("Failed to get series", "error", err, "id", id)
		response.InternalError(w, "Failed to retrieve series", s.logger)
		return
	}

	response.Success(w, series, s.logger)
}

// handleGetSeriesBooks returns all books in a series.
func (s *Server) handleGetSeriesBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if id == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	books, err := s.store.GetBooksBySeries(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get series books", "error", err, "id", id)
		response.InternalError(w, "Failed to retrieve series books", s.logger)
		return
	}

	response.Success(w, map[string]interface{}{
		"series_id": id,
		"books":     books,
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
