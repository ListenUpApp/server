package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// CreateCollectionRequest represents the request body for creating a collection.
type CreateCollectionRequest struct {
	LibraryID string `json:"library_id"`
	Name      string `json:"name"`
}

// UpdateCollectionRequest represents the request body for updating a collection.
type UpdateCollectionRequest struct {
	Name string `json:"name"`
}

// AddBookRequest represents the request body for adding a book to a collection.
type AddBookRequest struct {
	BookID string `json:"book_id"`
}

// handleCreateCollection creates a new collection.
func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	var req CreateCollectionRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.LibraryID == "" {
		response.BadRequest(w, "Library ID is required", s.logger)
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "Collection name is required", s.logger)
		return
	}

	collection, err := s.collectionService.CreateCollection(ctx, userID, req.LibraryID, req.Name)
	if err != nil {
		s.logger.Error("Failed to create collection", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to create collection", s.logger)
		return
	}

	response.Success(w, collection, s.logger)
}

// handleListCollections returns all collections the user can access in a library.
func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	libraryID := r.URL.Query().Get("library_id")
	if libraryID == "" {
		response.BadRequest(w, "Library ID is required", s.logger)
		return
	}

	collections, err := s.collectionService.ListCollections(ctx, userID, libraryID)
	if err != nil {
		s.logger.Error("Failed to list collections", "error", err, "user_id", userID, "library_id", libraryID)
		response.InternalError(w, "Failed to retrieve collections", s.logger)
		return
	}

	response.Success(w, collections, s.logger)
}

// handleGetCollection returns a single collection by ID.
func (s *Server) handleGetCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Collection ID is required", s.logger)
		return
	}

	collection, err := s.collectionService.GetCollection(ctx, userID, id)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			response.NotFound(w, "Collection not found", s.logger)
			return
		}
		s.logger.Error("Failed to get collection", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve collection", s.logger)
		return
	}

	response.Success(w, collection, s.logger)
}

// handleUpdateCollection updates a collection's metadata.
func (s *Server) handleUpdateCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Collection ID is required", s.logger)
		return
	}

	var req UpdateCollectionRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "Collection name is required", s.logger)
		return
	}

	collection, err := s.collectionService.UpdateCollection(ctx, userID, id, req.Name)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			response.NotFound(w, "Collection not found", s.logger)
			return
		}
		if errors.Is(err, store.ErrPermissionDenied) {
			response.Forbidden(w, "You don't have permission to update this collection", s.logger)
			return
		}
		s.logger.Error("Failed to update collection", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to update collection", s.logger)
		return
	}

	response.Success(w, collection, s.logger)
}

// handleDeleteCollection deletes a collection.
func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Collection ID is required", s.logger)
		return
	}

	err := s.collectionService.DeleteCollection(ctx, userID, id)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			response.NotFound(w, "Collection not found", s.logger)
			return
		}
		if errors.Is(err, store.ErrPermissionDenied) {
			response.Forbidden(w, "You don't have permission to delete this collection", s.logger)
			return
		}
		s.logger.Error("Failed to delete collection", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to delete collection", s.logger)
		return
	}

	response.Success(w, map[string]string{
		"message": "Collection deleted successfully",
	}, s.logger)
}

// handleAddBookToCollection adds a book to a collection.
func (s *Server) handleAddBookToCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Collection ID is required", s.logger)
		return
	}

	var req AddBookRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.BookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	err := s.collectionService.AddBookToCollection(ctx, userID, id, req.BookID)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			response.NotFound(w, "Collection not found", s.logger)
			return
		}
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		if errors.Is(err, store.ErrPermissionDenied) {
			response.Forbidden(w, "You don't have permission to modify this collection", s.logger)
			return
		}
		s.logger.Error("Failed to add book to collection", "error", err, "collection_id", id, "book_id", req.BookID, "user_id", userID)
		response.InternalError(w, "Failed to add book to collection", s.logger)
		return
	}

	response.Success(w, map[string]string{
		"message": "Book added to collection successfully",
	}, s.logger)
}

// handleRemoveBookFromCollection removes a book from a collection.
func (s *Server) handleRemoveBookFromCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")
	bookID := chi.URLParam(r, "bookID")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Collection ID is required", s.logger)
		return
	}

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	err := s.collectionService.RemoveBookFromCollection(ctx, userID, id, bookID)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			response.NotFound(w, "Collection not found", s.logger)
			return
		}
		if errors.Is(err, store.ErrPermissionDenied) {
			response.Forbidden(w, "You don't have permission to modify this collection", s.logger)
			return
		}
		s.logger.Error("Failed to remove book from collection", "error", err, "collection_id", id, "book_id", bookID, "user_id", userID)
		response.InternalError(w, "Failed to remove book from collection", s.logger)
		return
	}

	response.Success(w, map[string]string{
		"message": "Book removed from collection successfully",
	}, s.logger)
}

// handleGetCollectionBooks returns all books in a collection.
func (s *Server) handleGetCollectionBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Collection ID is required", s.logger)
		return
	}

	books, err := s.collectionService.GetCollectionBooks(ctx, userID, id)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			response.NotFound(w, "Collection not found", s.logger)
			return
		}
		s.logger.Error("Failed to get collection books", "error", err, "collection_id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve collection books", s.logger)
		return
	}

	response.Success(w, map[string]interface{}{
		"collection_id": id,
		"books":         books,
	}, s.logger)
}