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

	genres, err := s.services.Genre.ListGenres(ctx)
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

	genre, err := s.services.Genre.GetGenre(ctx, id)
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
	_ = mustGetUserID(ctx) // Validates auth

	var req service.CreateGenreRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	genre, err := s.services.Genre.CreateGenre(ctx, req)
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
	_ = mustGetUserID(ctx) // Validates auth
	id := chi.URLParam(r, "id")

	var req service.UpdateGenreRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	genre, err := s.services.Genre.UpdateGenre(ctx, id, req)
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
	_ = mustGetUserID(ctx) // Validates auth
	id := chi.URLParam(r, "id")

	err := s.services.Genre.DeleteGenre(ctx, id)
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
	_ = mustGetUserID(ctx) // Validates auth
	id := chi.URLParam(r, "id")

	var req struct {
		ParentID string `json:"parent_id"` // Empty string for root.
	}
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	genre, err := s.services.Genre.MoveGenre(ctx, id, req.ParentID)
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
	_ = mustGetUserID(ctx) // Validates auth

	var req service.MergeGenresRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if err := s.services.Genre.MergeGenres(ctx, req); err != nil {
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

	bookIDs, err := s.services.Genre.GetBooksForGenre(ctx, id, includeDescendants)
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

	unmapped, err := s.services.Genre.ListUnmappedGenres(ctx)
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
	userID := mustGetUserID(ctx)

	var req service.MapUnmappedGenreRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if err := s.services.Genre.MapUnmappedGenre(ctx, userID, req); err != nil {
		s.logger.Error("Failed to map genre", "error", err)
		response.BadRequest(w, err.Error(), s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "mapped"}, s.logger)
}

// handleGetBookGenres returns genres for a book.
func (s *Server) handleGetBookGenres(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bookID := chi.URLParam(r, "id")

	genres, err := s.services.Genre.GetGenresForBook(ctx, bookID)
	if err != nil {
		s.logger.Error("Failed to get book genres", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to get genres", s.logger)
		return
	}

	response.Success(w, genres, s.logger)
}

// handleSetBookGenres sets genres for a book.
func (s *Server) handleSetBookGenres(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = mustGetUserID(ctx) // Validates auth
	bookID := chi.URLParam(r, "id")

	var req struct {
		GenreIDs []string `json:"genre_ids"`
	}
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if err := s.services.Genre.SetBookGenres(ctx, bookID, req.GenreIDs); err != nil {
		s.logger.Error("Failed to set book genres", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to set genres", s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "ok"}, s.logger)
}
