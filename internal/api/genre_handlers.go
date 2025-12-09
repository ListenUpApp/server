package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleListGenres returns all genres (tree structure).
func (s *Server) handleListGenres(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	genres, err := s.genreService.ListGenres(ctx)
	if err != nil {
		s.logger.Error("Failed to list genres", "error", err)
		response.InternalError(w, "Failed to list genres", s.logger)
		return
	}

	response.Success(w, genres, s.logger)
}

// handleGetGenre returns a single genre.
func (s *Server) handleGetGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	genre, err := s.genreService.GetGenre(ctx, id)
	if errors.Is(err, store.ErrGenreNotFound) {
		response.NotFound(w, "Genre not found", s.logger)
		return
	}
	if err != nil {
		s.logger.Error("Failed to get genre", "error", err, "id", id)
		response.InternalError(w, "Failed to get genre", s.logger)
		return
	}

	response.Success(w, genre, s.logger)
}

// handleCreateGenre creates a new genre.
func (s *Server) handleCreateGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req service.CreateGenreRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	genre, err := s.genreService.CreateGenre(ctx, req)
	if err != nil {
		s.logger.Error("Failed to create genre", "error", err)
		response.BadRequest(w, err.Error(), s.logger)
		return
	}

	response.Created(w, genre, s.logger)
}

// handleUpdateGenre updates a genre.
func (s *Server) handleUpdateGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req service.UpdateGenreRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	genre, err := s.genreService.UpdateGenre(ctx, id, req)
	if errors.Is(err, store.ErrGenreNotFound) {
		response.NotFound(w, "Genre not found", s.logger)
		return
	}
	if err != nil {
		s.logger.Error("Failed to update genre", "error", err, "id", id)
		response.InternalError(w, "Failed to update genre", s.logger)
		return
	}

	response.Success(w, genre, s.logger)
}

// handleDeleteGenre deletes a genre.
func (s *Server) handleDeleteGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	err := s.genreService.DeleteGenre(ctx, id)
	if errors.Is(err, store.ErrGenreNotFound) {
		response.NotFound(w, "Genre not found", s.logger)
		return
	}
	if errors.Is(err, store.ErrGenreHasChildren) {
		response.BadRequest(w, "Cannot delete genre with children", s.logger)
		return
	}
	if errors.Is(err, store.ErrCannotDeleteSystem) {
		response.BadRequest(w, "Cannot delete system genre", s.logger)
		return
	}
	if err != nil {
		s.logger.Error("Failed to delete genre", "error", err, "id", id)
		response.InternalError(w, "Failed to delete genre", s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "deleted"}, s.logger)
}

// handleMoveGenre changes a genre's parent.
func (s *Server) handleMoveGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req struct {
		ParentID string `json:"parent_id"` // Empty string for root.
	}
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	genre, err := s.genreService.MoveGenre(ctx, id, req.ParentID)
	if err != nil {
		s.logger.Error("Failed to move genre", "error", err, "id", id)
		response.BadRequest(w, err.Error(), s.logger)
		return
	}

	response.Success(w, genre, s.logger)
}

// handleMergeGenres merges two genres.
func (s *Server) handleMergeGenres(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req service.MergeGenresRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if err := s.genreService.MergeGenres(ctx, req); err != nil {
		s.logger.Error("Failed to merge genres", "error", err)
		response.BadRequest(w, err.Error(), s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "merged"}, s.logger)
}

// handleGetGenreBooks returns books in a genre.
func (s *Server) handleGetGenreBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	includeDescendants := r.URL.Query().Get("include_descendants") == "true"

	bookIDs, err := s.genreService.GetBooksForGenre(ctx, id, includeDescendants)
	if err != nil {
		s.logger.Error("Failed to get genre books", "error", err, "id", id)
		response.InternalError(w, "Failed to get books", s.logger)
		return
	}

	response.Success(w, map[string]interface{}{
		"genre_id": id,
		"book_ids": bookIDs,
		"count":    len(bookIDs),
	}, s.logger)
}

// handleListUnmappedGenres returns unmapped genre strings.
func (s *Server) handleListUnmappedGenres(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	unmapped, err := s.genreService.ListUnmappedGenres(ctx)
	if err != nil {
		s.logger.Error("Failed to list unmapped genres", "error", err)
		response.InternalError(w, "Failed to list unmapped genres", s.logger)
		return
	}

	response.Success(w, unmapped, s.logger)
}

// handleMapUnmappedGenre creates an alias for an unmapped genre.
func (s *Server) handleMapUnmappedGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req service.MapUnmappedGenreRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if err := s.genreService.MapUnmappedGenre(ctx, userID, req); err != nil {
		s.logger.Error("Failed to map genre", "error", err)
		response.BadRequest(w, err.Error(), s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "mapped"}, s.logger)
}

// handleSetBookGenres sets genres for a book.
func (s *Server) handleSetBookGenres(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req struct {
		GenreIDs []string `json:"genre_ids"`
	}
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if err := s.genreService.SetBookGenres(ctx, bookID, req.GenreIDs); err != nil {
		s.logger.Error("Failed to set book genres", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to set genres", s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "ok"}, s.logger)
}
