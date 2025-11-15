package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleListContributors returns a paginated list of contributors.
func (s *Server) handleListContributors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	params := parsePaginationParams(r)

	contributors, err := s.syncService.GetContributorsForSync(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list contributors", "error", err)
		response.InternalError(w, "Failed to retrieve contributors", s.logger)
		return
	}

	response.Success(w, contributors, s.logger)
}

// handleGetContributor returns a single contributor by ID.
func (s *Server) handleGetContributor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if id == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	contributor, err := s.store.GetContributor(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrContributorNotFound) {
			response.NotFound(w, "Contributor not found", s.logger)
			return
		}
		s.logger.Error("Failed to get contributor", "error", err, "id", id)
		response.InternalError(w, "Failed to retrieve contributor", s.logger)
		return
	}

	response.Success(w, contributor, s.logger)
}

// handleGetContributorBooks returns all books by a contributor.
func (s *Server) handleGetContributorBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if id == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	books, err := s.store.GetBooksByContributor(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get contributor books", "error", err, "id", id)
		response.InternalError(w, "Failed to retrieve contributor books", s.logger)
		return
	}

	response.Success(w, map[string]interface{}{
		"contributor_id": id,
		"books":          books,
	}, s.logger)
}

// handleSyncContributors handles GET /api/v1/sync/contributors for syncing contributors.
func (s *Server) handleSyncContributors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	params := parsePaginationParams(r)

	contributors, err := s.syncService.GetContributorsForSync(ctx, params)
	if err != nil {
		s.logger.Error("Failed to sync contributors", "error", err)
		response.InternalError(w, "Failed to sync contributors", s.logger)
		return
	}

	response.Success(w, contributors, s.logger)
}
