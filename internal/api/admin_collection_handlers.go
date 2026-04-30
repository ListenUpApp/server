package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerAdminCollectionRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listAdminCollections",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/collections",
		Summary:     "List all collections",
		Description: "Lists all collections in the system (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListAdminCollections)

	huma.Register(s.api, huma.Operation{
		OperationID: "createAdminCollection",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/collections",
		Summary:     "Create collection",
		Description: "Creates a new collection (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateAdminCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "getAdminCollection",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/collections/{id}",
		Summary:     "Get collection",
		Description: "Gets a collection by ID (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetAdminCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateAdminCollection",
		Method:      http.MethodPatch,
		Path:        "/api/v1/admin/collections/{id}",
		Summary:     "Update collection",
		Description: "Updates a collection (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateAdminCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteAdminCollection",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/collections/{id}",
		Summary:     "Delete collection",
		Description: "Deletes a collection (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteAdminCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "addBooksToCollection",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/collections/{id}/books",
		Summary:     "Add books to collection",
		Description: "Adds multiple books to a collection (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAddBooksToCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "removeBookFromAdminCollection",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/collections/{id}/books/{bookId}",
		Summary:     "Remove book from collection",
		Description: "Removes a book from a collection (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRemoveBookFromAdminCollection)
}

// === DTOs ===

// AdminCollectionResponse is the API response for a collection in admin context.
type AdminCollectionResponse struct {
	ID        string    `json:"id" doc:"Collection ID"`
	Name      string    `json:"name" doc:"Collection name"`
	BookCount int       `json:"book_count" doc:"Number of books in collection"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt time.Time `json:"updated_at" doc:"Last update time"`
}

// ListAdminCollectionsInput is the Huma input for listing collections.
type ListAdminCollectionsInput struct {
	Authorization string `header:"Authorization"`
}

// ListAdminCollectionsResponse is the API response for listing collections.
type ListAdminCollectionsResponse struct {
	Collections []AdminCollectionResponse `json:"collections" doc:"List of collections"`
	Total       int                       `json:"total" doc:"Total count"`
}

// ListAdminCollectionsOutput is the Huma output wrapper for listing collections.
type ListAdminCollectionsOutput struct {
	Body ListAdminCollectionsResponse
}

// CreateAdminCollectionRequest is the request body for creating a collection.
type CreateAdminCollectionRequest struct {
	Name      string `json:"name" validate:"required,min=1,max=100" doc:"Collection name"`
	LibraryID string `json:"library_id,omitempty" doc:"Library ID (defaults to default library)"`
}

// CreateAdminCollectionInput is the Huma input for creating a collection.
type CreateAdminCollectionInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateAdminCollectionRequest
}

// AdminCollectionOutput is the Huma output wrapper for a collection.
type AdminCollectionOutput struct {
	Body AdminCollectionResponse
}

// GetAdminCollectionInput is the Huma input for getting a collection.
type GetAdminCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
}

// UpdateAdminCollectionRequest is the request body for updating a collection.
type UpdateAdminCollectionRequest struct {
	Name *string `json:"name,omitempty" validate:"omitempty,min=1,max=100" doc:"Collection name"`
}

// UpdateAdminCollectionInput is the Huma input for updating a collection.
type UpdateAdminCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
	Body          UpdateAdminCollectionRequest
}

// DeleteAdminCollectionInput is the Huma input for deleting a collection.
type DeleteAdminCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
}

// AddBooksRequest is the request body for adding books to a collection.
type AddBooksRequest struct {
	BookIDs []string `json:"book_ids" validate:"required,min=1,dive,required" doc:"List of book IDs to add"`
}

// AddBooksInput is the Huma input for adding books to a collection.
type AddBooksInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
	Body          AddBooksRequest
}

// RemoveBookInput is the Huma input for removing a book from a collection.
type RemoveBookInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
	BookID        string `path:"bookId" doc:"Book ID"`
}

// === Handlers ===

func (s *Server) handleListAdminCollections(ctx context.Context, _ *ListAdminCollectionsInput) (*ListAdminCollectionsOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	collections, err := s.services.Collection.AdminListAllCollections(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]AdminCollectionResponse, len(collections))
	for i, c := range collections {
		resp[i] = AdminCollectionResponse{
			ID:        c.ID,
			Name:      c.Name,
			BookCount: len(c.BookIDs),
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		}
	}

	return &ListAdminCollectionsOutput{
		Body: ListAdminCollectionsResponse{Collections: resp, Total: len(resp)},
	}, nil
}

func (s *Server) handleCreateAdminCollection(ctx context.Context, input *CreateAdminCollectionInput) (*AdminCollectionOutput, error) {
	userID, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	coll, err := s.services.Collection.AdminCreateCollection(ctx, userID, input.Body.LibraryID, input.Body.Name)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateCollection) {
			return nil, huma.Error409Conflict("Collection already exists")
		}
		if errors.Is(err, store.ErrNotFound) {
			return nil, huma.Error400BadRequest("No default library found")
		}
		return nil, err
	}

	// Emit SSE event
	s.sseManager.Emit(sse.NewCollectionCreatedEvent(coll.ID, coll.Name, 0))

	return &AdminCollectionOutput{
		Body: AdminCollectionResponse{
			ID:        coll.ID,
			Name:      coll.Name,
			BookCount: 0,
			CreatedAt: coll.CreatedAt,
			UpdatedAt: coll.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleGetAdminCollection(ctx context.Context, input *GetAdminCollectionInput) (*AdminCollectionOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	coll, err := s.services.Collection.AdminGetCollection(ctx, input.ID)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	return &AdminCollectionOutput{
		Body: AdminCollectionResponse{
			ID:        coll.ID,
			Name:      coll.Name,
			BookCount: len(coll.BookIDs),
			CreatedAt: coll.CreatedAt,
			UpdatedAt: coll.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleUpdateAdminCollection(ctx context.Context, input *UpdateAdminCollectionInput) (*AdminCollectionOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	coll, err := s.services.Collection.AdminUpdateCollectionName(ctx, input.ID, input.Body.Name)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	// Emit SSE event
	s.sseManager.Emit(sse.NewCollectionUpdatedEvent(coll.ID, coll.Name, len(coll.BookIDs)))

	return &AdminCollectionOutput{
		Body: AdminCollectionResponse{
			ID:        coll.ID,
			Name:      coll.Name,
			BookCount: len(coll.BookIDs),
			CreatedAt: coll.CreatedAt,
			UpdatedAt: coll.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleDeleteAdminCollection(ctx context.Context, input *DeleteAdminCollectionInput) (*MessageOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	result, err := s.services.Collection.AdminDeleteCollection(ctx, input.ID)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	if result.Collection.IsInbox {
		return nil, huma.Error400BadRequest("Cannot delete system collection")
	}

	// Emit SSE event for admins
	s.sseManager.Emit(sse.NewCollectionDeletedEvent(input.ID, result.Collection.Name))

	// Notify non-members about books becoming public
	for _, bookID := range result.BooksBecomingPublic {
		book, err := s.services.Collection.GetBook(ctx, bookID, result.Collection.OwnerID)
		if err != nil {
			continue
		}
		enrichedBook, err := s.enricher.EnrichBook(ctx, book)
		if err != nil {
			continue
		}
		createEvent := sse.NewBookCreatedEvent(enrichedBook)
		s.sseManager.EmitToNonMembers(result.MemberUserIDs, createEvent)
	}

	return &MessageOutput{Body: MessageResponse{Message: "Collection deleted"}}, nil
}

func (s *Server) handleAddBooksToCollection(ctx context.Context, input *AddBooksInput) (*MessageOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	result, err := s.services.Collection.AdminAddBooksToCollection(ctx, input.ID, input.Body.BookIDs)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	for _, r := range result.Results {
		if !r.Added {
			continue
		}

		// Emit SSE event for each book added (admin-only)
		s.sseManager.Emit(sse.NewCollectionBookAddedEvent(input.ID, result.Collection.Name, r.BookID))

		// If book was previously uncollected (visible to all), notify non-members
		// that they've lost access by sending a book.deleted event
		if r.WasUncollected {
			s.logger.Debug("book was uncollected, notifying non-members of access revocation",
				"book_id", r.BookID,
				"collection_id", input.ID,
				"member_count", len(result.MemberUserIDs))

			deleteEvent := sse.NewBookDeletedEvent(r.BookID, time.Now())
			s.sseManager.EmitToNonMembers(result.MemberUserIDs, deleteEvent)
		}
	}

	return &MessageOutput{Body: MessageResponse{Message: "Books added to collection"}}, nil
}

func (s *Server) handleRemoveBookFromAdminCollection(ctx context.Context, input *RemoveBookInput) (*MessageOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	result, err := s.services.Collection.AdminRemoveBookFromCollection(ctx, input.ID, input.BookID)
	if err != nil {
		if errors.Is(err, store.ErrCollectionNotFound) {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	// Emit SSE event for admins
	s.sseManager.Emit(sse.NewCollectionBookRemovedEvent(input.ID, result.Collection.Name, input.BookID))

	// If book became uncollected (public), notify non-members that they can now access it
	if result.WillBecomePublic {
		s.logger.Debug("book became uncollected, notifying non-members of access grant",
			"book_id", input.BookID,
			"collection_id", input.ID)

		// Get book and enrich for SSE
		// Use collection owner as userID for access (admin has global access)
		book, err := s.services.Collection.GetBook(ctx, input.BookID, result.Collection.OwnerID)
		if err == nil {
			enrichedBook, err := s.enricher.EnrichBook(ctx, book)
			if err == nil {
				createEvent := sse.NewBookCreatedEvent(enrichedBook)
				s.sseManager.EmitToNonMembers(result.MemberUserIDs, createEvent)
			}
		}
	}

	return &MessageOutput{Body: MessageResponse{Message: "Book removed from collection"}}, nil
}
