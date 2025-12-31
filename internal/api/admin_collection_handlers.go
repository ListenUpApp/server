package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
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

func (s *Server) handleListAdminCollections(ctx context.Context, input *ListAdminCollectionsInput) (*ListAdminCollectionsOutput, error) {
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	collections, err := s.store.AdminListAllCollections(ctx)
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
	userID, err := s.authenticateAndRequireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	// Get library ID - use default library if not specified
	libraryID := input.Body.LibraryID
	if libraryID == "" {
		lib, err := s.store.GetDefaultLibrary(ctx)
		if err != nil {
			return nil, huma.Error400BadRequest("No default library found")
		}
		libraryID = lib.ID
	}

	// Generate collection ID
	collID, err := id.Generate("coll")
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to generate collection ID")
	}

	now := time.Now()
	coll := &domain.Collection{
		ID:        collID,
		LibraryID: libraryID,
		OwnerID:   userID,
		Name:      input.Body.Name,
		BookIDs:   []string{},
		IsInbox:   false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.store.CreateCollection(ctx, coll); err != nil {
		if err == store.ErrDuplicateCollection {
			return nil, huma.Error409Conflict("Collection already exists")
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
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	coll, err := s.store.AdminGetCollection(ctx, input.ID)
	if err != nil {
		if err == store.ErrCollectionNotFound {
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
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	coll, err := s.store.AdminGetCollection(ctx, input.ID)
	if err != nil {
		if err == store.ErrCollectionNotFound {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	// Apply updates
	if input.Body.Name != nil {
		coll.Name = *input.Body.Name
	}
	coll.UpdatedAt = time.Now()

	if err := s.store.AdminUpdateCollection(ctx, coll); err != nil {
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
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	// Get collection first for SSE event
	coll, err := s.store.AdminGetCollection(ctx, input.ID)
	if err != nil {
		if err == store.ErrCollectionNotFound {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	if coll.IsInbox {
		return nil, huma.Error400BadRequest("Cannot delete system collection")
	}

	collName := coll.Name

	// Identify books that will become public after deletion
	// (books only in this collection, not in any other)
	var booksBecomingPublic []string
	for _, bookID := range coll.BookIDs {
		colls, err := s.store.GetCollectionsForBook(ctx, bookID)
		if err != nil {
			continue
		}
		// If book is only in this collection, it will become public
		if len(colls) == 1 && colls[0].ID == input.ID {
			booksBecomingPublic = append(booksBecomingPublic, bookID)
		}
	}

	// Get member IDs for access grant notifications
	memberUserIDs := make(map[string]bool)
	if len(booksBecomingPublic) > 0 {
		memberUserIDs[coll.OwnerID] = true
		shares, _ := s.store.GetSharesForCollection(ctx, input.ID)
		for _, share := range shares {
			memberUserIDs[share.SharedWithUserID] = true
		}
	}

	if err := s.store.AdminDeleteCollection(ctx, input.ID); err != nil {
		return nil, err
	}

	// Emit SSE event for admins
	s.sseManager.Emit(sse.NewCollectionDeletedEvent(input.ID, collName))

	// Notify non-members about books becoming public
	for _, bookID := range booksBecomingPublic {
		book, err := s.store.GetBook(ctx, bookID, coll.OwnerID)
		if err != nil {
			continue
		}
		enrichedBook, err := s.store.EnrichBook(ctx, book)
		if err != nil {
			continue
		}
		createEvent := sse.NewBookCreatedEvent(enrichedBook)
		s.sseManager.EmitToNonMembers(memberUserIDs, createEvent)
	}

	return &MessageOutput{Body: MessageResponse{Message: "Collection deleted"}}, nil
}

func (s *Server) handleAddBooksToCollection(ctx context.Context, input *AddBooksInput) (*MessageOutput, error) {
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	// Get collection first for SSE event and validation
	coll, err := s.store.AdminGetCollection(ctx, input.ID)
	if err != nil {
		if err == store.ErrCollectionNotFound {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	collName := coll.Name

	// Get users who have access to this collection (for access revocation notifications)
	// Members = owner + anyone with shares
	memberUserIDs := make(map[string]bool)
	memberUserIDs[coll.OwnerID] = true

	shares, err := s.store.GetSharesForCollection(ctx, input.ID)
	if err == nil {
		for _, share := range shares {
			memberUserIDs[share.SharedWithUserID] = true
		}
	}

	for _, bookID := range input.Body.BookIDs {
		// Check if book was previously uncollected (public)
		// If so, adding it to a collection restricts access
		existingColls, _ := s.store.GetCollectionsForBook(ctx, bookID)
		wasUncollected := len(existingColls) == 0

		if err := s.store.AdminAddBookToCollection(ctx, bookID, input.ID); err != nil {
			// Continue on error - best effort
			s.logger.Warn("failed to add book to collection",
				"book_id", bookID,
				"collection_id", input.ID,
				"error", err)
			continue
		}

		// Emit SSE event for each book added (admin-only)
		s.sseManager.Emit(sse.NewCollectionBookAddedEvent(input.ID, collName, bookID))

		// If book was previously uncollected (visible to all), notify non-members
		// that they've lost access by sending a book.deleted event
		if wasUncollected {
			s.logger.Debug("book was uncollected, notifying non-members of access revocation",
				"book_id", bookID,
				"collection_id", input.ID,
				"member_count", len(memberUserIDs))

			deleteEvent := sse.NewBookDeletedEvent(bookID, time.Now())
			s.sseManager.EmitToNonMembers(memberUserIDs, deleteEvent)
		}
	}

	return &MessageOutput{Body: MessageResponse{Message: "Books added to collection"}}, nil
}

func (s *Server) handleRemoveBookFromAdminCollection(ctx context.Context, input *RemoveBookInput) (*MessageOutput, error) {
	if _, err := s.authenticateAndRequireAdmin(ctx, input.Authorization); err != nil {
		return nil, err
	}

	// Get collection first for SSE event
	coll, err := s.store.AdminGetCollection(ctx, input.ID)
	if err != nil {
		if err == store.ErrCollectionNotFound {
			return nil, huma.Error404NotFound("Collection not found")
		}
		return nil, err
	}

	collName := coll.Name

	// Check if book will become uncollected after removal
	// (i.e., it's only in this one collection)
	existingColls, _ := s.store.GetCollectionsForBook(ctx, input.BookID)
	willBecomePublic := len(existingColls) == 1 && existingColls[0].ID == input.ID

	// Get member IDs before removal (for access grant notifications)
	memberUserIDs := make(map[string]bool)
	if willBecomePublic {
		memberUserIDs[coll.OwnerID] = true
		shares, _ := s.store.GetSharesForCollection(ctx, input.ID)
		for _, share := range shares {
			memberUserIDs[share.SharedWithUserID] = true
		}
	}

	if err := s.store.AdminRemoveBookFromCollection(ctx, input.BookID, input.ID); err != nil {
		return nil, err
	}

	// Emit SSE event for admins
	s.sseManager.Emit(sse.NewCollectionBookRemovedEvent(input.ID, collName, input.BookID))

	// If book became uncollected (public), notify non-members that they can now access it
	if willBecomePublic {
		s.logger.Debug("book became uncollected, notifying non-members of access grant",
			"book_id", input.BookID,
			"collection_id", input.ID)

		// Get book and enrich for SSE
		// Use collection owner as userID for access (admin has global access)
		book, err := s.store.GetBook(ctx, input.BookID, coll.OwnerID)
		if err == nil {
			enrichedBook, err := s.store.EnrichBook(ctx, book)
			if err == nil {
				createEvent := sse.NewBookCreatedEvent(enrichedBook)
				s.sseManager.EmitToNonMembers(memberUserIDs, createEvent)
			}
		}
	}

	return &MessageOutput{Body: MessageResponse{Message: "Book removed from collection"}}, nil
}
