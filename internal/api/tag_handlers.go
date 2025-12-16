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

// handleListTags returns all tags for the current user.
func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	tags, err := s.services.Tag.ListTags(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to list tags", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to list tags", s.logger)
		return
	}

	response.Success(w, tags, s.logger)
}

// handleGetTag returns a single tag.
func (s *Server) handleGetTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	tag, err := s.services.Tag.GetTag(ctx, userID, id)
	if errors.Is(err, store.ErrTagNotFound) {
		response.NotFound(w, "Tag not found", s.logger)
		return
	}
	if err != nil {
		s.logger.Error("Failed to get tag", "error", err, "id", id)
		response.InternalError(w, "Failed to get tag", s.logger)
		return
	}

	response.Success(w, tag, s.logger)
}

// handleCreateTag creates a new tag.
func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req service.CreateTagRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	tag, err := s.services.Tag.CreateTag(ctx, userID, req)
	if err != nil {
		s.logger.Error("Failed to create tag", "error", err, "user_id", userID)
		response.BadRequest(w, err.Error(), s.logger)
		return
	}

	response.Created(w, tag, s.logger)
}

// handleUpdateTag updates a tag.
func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req service.UpdateTagRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	tag, err := s.services.Tag.UpdateTag(ctx, userID, id, req)
	if errors.Is(err, store.ErrTagNotFound) {
		response.NotFound(w, "Tag not found", s.logger)
		return
	}
	if err != nil {
		s.logger.Error("Failed to update tag", "error", err, "id", id)
		response.InternalError(w, "Failed to update tag", s.logger)
		return
	}

	response.Success(w, tag, s.logger)
}

// handleDeleteTag deletes a tag.
func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	err := s.services.Tag.DeleteTag(ctx, userID, id)
	if errors.Is(err, store.ErrTagNotFound) {
		response.NotFound(w, "Tag not found", s.logger)
		return
	}
	if err != nil {
		s.logger.Error("Failed to delete tag", "error", err, "id", id)
		response.InternalError(w, "Failed to delete tag", s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "deleted"}, s.logger)
}

// handleAddBookTag adds a tag to a book.
func (s *Server) handleAddBookTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req struct {
		TagID string `json:"tag_id"`
	}
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if err := s.services.Tag.AddTagToBook(ctx, userID, bookID, req.TagID); err != nil {
		s.logger.Error("Failed to add tag to book", "error", err)
		response.BadRequest(w, err.Error(), s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "ok"}, s.logger)
}

// handleRemoveBookTag removes a tag from a book.
func (s *Server) handleRemoveBookTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")
	tagID := chi.URLParam(r, "tagID")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if err := s.services.Tag.RemoveTagFromBook(ctx, userID, bookID, tagID); err != nil {
		s.logger.Error("Failed to remove tag from book", "error", err)
		response.InternalError(w, "Failed to remove tag", s.logger)
		return
	}

	response.Success(w, map[string]string{"status": "ok"}, s.logger)
}

// handleGetBookTags returns tags for a book.
func (s *Server) handleGetBookTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	tags, err := s.services.Tag.GetTagsForBook(ctx, userID, bookID)
	if err != nil {
		s.logger.Error("Failed to get book tags", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to get tags", s.logger)
		return
	}

	response.Success(w, tags, s.logger)
}

// handleGetTagBooks returns books with a specific tag.
func (s *Server) handleGetTagBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	tagID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	bookIDs, err := s.services.Tag.GetBooksForTag(ctx, userID, tagID)
	if err != nil {
		s.logger.Error("Failed to get tag books", "error", err, "tag_id", tagID)
		response.InternalError(w, "Failed to get books", s.logger)
		return
	}

	response.Success(w, map[string]interface{}{
		"tag_id":   tagID,
		"book_ids": bookIDs,
		"count":    len(bookIDs),
	}, s.logger)
}
