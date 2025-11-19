package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// CreateShareRequest represents the request body for creating a share.
type CreateShareRequest struct {
	CollectionID     string `json:"collection_id"`
	SharedWithUserID string `json:"shared_with_user_id"`
	Permission       string `json:"permission"` // "read" or "write"
}

// UpdateShareRequest represents the request body for updating a share.
type UpdateShareRequest struct {
	Permission string `json:"permission"` // "read" or "write"
}

// handleCreateShare creates a new collection share.
func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req CreateShareRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.CollectionID == "" {
		response.BadRequest(w, "Collection ID is required", s.logger)
		return
	}

	if req.SharedWithUserID == "" {
		response.BadRequest(w, "User ID to share with is required", s.logger)
		return
	}

	// Parse permission
	permission := parsePermission(req.Permission)
	if permission == -1 {
		response.BadRequest(w, "Invalid permission. Must be 'read' or 'write'", s.logger)
		return
	}

	share, err := s.sharingService.ShareCollection(ctx, userID, req.CollectionID, req.SharedWithUserID, permission)
	if err != nil {
		if errors.Is(err, store.ErrShareAlreadyExists) {
			response.Conflict(w, "Collection is already shared with this user", s.logger)
			return
		}
		s.logger.Error("Failed to create share", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to share collection", s.logger)
		return
	}

	response.Success(w, share, s.logger)
}

// handleListShares returns shares based on query parameter.
// Query param "type" can be "shared_with_me" (default) or "shared_by_collection".
// If "shared_by_collection", requires "collection_id" param.
func (s *Server) handleListShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	shareType := r.URL.Query().Get("type")
	if shareType == "" {
		shareType = "shared_with_me"
	}

	switch shareType {
	case "shared_with_me":
		shares, err := s.sharingService.ListSharedWithMe(ctx, userID)
		if err != nil {
			s.logger.Error("Failed to list shares", "error", err, "user_id", userID)
			response.InternalError(w, "Failed to retrieve shares", s.logger)
			return
		}
		response.Success(w, shares, s.logger)

	case "shared_by_collection":
		collectionID := r.URL.Query().Get("collection_id")
		if collectionID == "" {
			response.BadRequest(w, "Collection ID is required for this query type", s.logger)
			return
		}

		shares, err := s.sharingService.ListCollectionShares(ctx, userID, collectionID)
		if err != nil {
			s.logger.Error("Failed to list collection shares", "error", err, "user_id", userID, "collection_id", collectionID)
			response.InternalError(w, "Failed to retrieve collection shares", s.logger)
			return
		}
		response.Success(w, shares, s.logger)

	default:
		response.BadRequest(w, "Invalid type parameter. Must be 'shared_with_me' or 'shared_by_collection'", s.logger)
	}
}

// handleGetShare returns a single share by ID.
func (s *Server) handleGetShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Share ID is required", s.logger)
		return
	}

	share, err := s.sharingService.GetShare(ctx, userID, id)
	if err != nil {
		if errors.Is(err, store.ErrShareNotFound) {
			response.NotFound(w, "Share not found", s.logger)
			return
		}
		s.logger.Error("Failed to get share", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve share", s.logger)
		return
	}

	response.Success(w, share, s.logger)
}

// handleUpdateShare updates a share's permission.
func (s *Server) handleUpdateShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Share ID is required", s.logger)
		return
	}

	var req UpdateShareRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// Parse permission
	permission := parsePermission(req.Permission)
	if permission == -1 {
		response.BadRequest(w, "Invalid permission. Must be 'read' or 'write'", s.logger)
		return
	}

	share, err := s.sharingService.UpdateSharePermission(ctx, userID, id, permission)
	if err != nil {
		if errors.Is(err, store.ErrShareNotFound) {
			response.NotFound(w, "Share not found", s.logger)
			return
		}
		s.logger.Error("Failed to update share", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to update share", s.logger)
		return
	}

	response.Success(w, share, s.logger)
}

// handleDeleteShare deletes a share (unshares a collection).
func (s *Server) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Share ID is required", s.logger)
		return
	}

	err := s.sharingService.UnshareCollection(ctx, userID, id)
	if err != nil {
		if errors.Is(err, store.ErrShareNotFound) {
			response.NotFound(w, "Share not found", s.logger)
			return
		}
		s.logger.Error("Failed to delete share", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to unshare collection", s.logger)
		return
	}

	response.Success(w, map[string]string{
		"message": "Collection unshared successfully",
	}, s.logger)
}

// parsePermission parses a permission string to a SharePermission enum.
// Returns -1 if invalid.
func parsePermission(perm string) domain.SharePermission {
	switch perm {
	case "read":
		return domain.PermissionRead
	case "write":
		return domain.PermissionWrite
	default:
		return -1
	}
}