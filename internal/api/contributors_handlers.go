package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleListContributors returns a paginated list of contributors.
// Note: Contributor visibility is not filtered by ACL yet - returns all contributors.
// TODO: Filter to only show contributors with at least one accessible book.
func (s *Server) handleListContributors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	params := parsePaginationParams(r)

	contributors, err := s.syncService.GetContributorsForSync(ctx, params)
	if err != nil {
		s.logger.Error("Failed to list contributors", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to retrieve contributors", s.logger)
		return
	}

	response.Success(w, contributors, s.logger)
}

// handleGetContributor returns a single contributor by ID.
// Note: Contributor visibility is not filtered by ACL yet.
// TODO: Return 404 if user has no access to any books by this contributor.
func (s *Server) handleGetContributor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

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
		s.logger.Error("Failed to get contributor", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve contributor", s.logger)
		return
	}

	response.Success(w, contributor, s.logger)
}

// handleGetContributorBooks returns all books by a contributor that the user can access.
func (s *Server) handleGetContributorBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	// Get all books by the contributor
	allBooks, err := s.store.GetBooksByContributor(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get contributor books", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve contributor books", s.logger)
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
		"contributor_id": id,
		"books":          accessibleBooks,
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
