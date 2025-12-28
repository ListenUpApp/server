package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerCollectionRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listCollections",
		Method:      http.MethodGet,
		Path:        "/api/v1/libraries/{libraryId}/collections",
		Summary:     "List collections",
		Description: "Returns all collections in a library the user can access",
		Tags:        []string{"Collections"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListCollections)

	huma.Register(s.api, huma.Operation{
		OperationID: "createCollection",
		Method:      http.MethodPost,
		Path:        "/api/v1/libraries/{libraryId}/collections",
		Summary:     "Create collection",
		Description: "Creates a new collection in a library",
		Tags:        []string{"Collections"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "getCollection",
		Method:      http.MethodGet,
		Path:        "/api/v1/collections/{id}",
		Summary:     "Get collection",
		Description: "Returns a collection by ID",
		Tags:        []string{"Collections"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateCollection",
		Method:      http.MethodPatch,
		Path:        "/api/v1/collections/{id}",
		Summary:     "Update collection",
		Description: "Updates a collection",
		Tags:        []string{"Collections"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteCollection",
		Method:      http.MethodDelete,
		Path:        "/api/v1/collections/{id}",
		Summary:     "Delete collection",
		Description: "Deletes a collection",
		Tags:        []string{"Collections"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "getCollectionBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/collections/{id}/books",
		Summary:     "Get collection books",
		Description: "Returns all books in a collection",
		Tags:        []string{"Collections"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetCollectionBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "addBookToCollection",
		Method:      http.MethodPost,
		Path:        "/api/v1/collections/{id}/books/{bookId}",
		Summary:     "Add book to collection",
		Description: "Adds a book to a collection",
		Tags:        []string{"Collections"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAddBookToCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "removeBookFromCollection",
		Method:      http.MethodDelete,
		Path:        "/api/v1/collections/{id}/books/{bookId}",
		Summary:     "Remove book from collection",
		Description: "Removes a book from a collection",
		Tags:        []string{"Collections"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRemoveBookFromCollection)
}

// === DTOs ===

// ListCollectionsInput contains parameters for listing collections.
type ListCollectionsInput struct {
	Authorization string `header:"Authorization"`
	LibraryID     string `path:"libraryId" doc:"Library ID"`
}

// CollectionResponse contains collection data in API responses.
type CollectionResponse struct {
	ID        string    `json:"id" doc:"Collection ID"`
	LibraryID string    `json:"library_id" doc:"Library ID"`
	OwnerID   string    `json:"owner_id" doc:"Owner user ID"`
	Name      string    `json:"name" doc:"Collection name"`
	BookCount int       `json:"book_count" doc:"Number of books"`
	IsInbox   bool      `json:"is_inbox" doc:"Whether this is the inbox collection"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt time.Time `json:"updated_at" doc:"Last update time"`
}

// ListCollectionsResponse contains a list of collections.
type ListCollectionsResponse struct {
	Collections []CollectionResponse `json:"collections" doc:"List of collections"`
}

// ListCollectionsOutput wraps the list collections response for Huma.
type ListCollectionsOutput struct {
	Body ListCollectionsResponse
}

// CreateCollectionRequest is the request body for creating a collection.
type CreateCollectionRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100" doc:"Collection name"`
}

// CreateCollectionInput wraps the create collection request for Huma.
type CreateCollectionInput struct {
	Authorization string `header:"Authorization"`
	LibraryID     string `path:"libraryId" doc:"Library ID"`
	Body          CreateCollectionRequest
}

// CollectionOutput wraps the collection response for Huma.
type CollectionOutput struct {
	Body CollectionResponse
}

// GetCollectionInput contains parameters for getting a collection.
type GetCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
}

// UpdateCollectionRequest is the request body for updating a collection.
type UpdateCollectionRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100" doc:"New collection name"`
}

// UpdateCollectionInput wraps the update collection request for Huma.
type UpdateCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
	Body          UpdateCollectionRequest
}

// DeleteCollectionInput contains parameters for deleting a collection.
type DeleteCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
}

// GetCollectionBooksInput contains parameters for getting collection books.
type GetCollectionBooksInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
}

// CollectionBookResponse represents a book in collection responses.
type CollectionBookResponse struct {
	ID        string  `json:"id" doc:"Book ID"`
	Title     string  `json:"title" doc:"Book title"`
	CoverPath *string `json:"cover_path,omitempty" doc:"Cover image path"`
}

// CollectionBooksResponse contains books in a collection.
type CollectionBooksResponse struct {
	Books []CollectionBookResponse `json:"books" doc:"Books in collection"`
}

// CollectionBooksOutput wraps the collection books response for Huma.
type CollectionBooksOutput struct {
	Body CollectionBooksResponse
}

// AddBookToCollectionInput contains parameters for adding a book to a collection.
type AddBookToCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
	BookID        string `path:"bookId" doc:"Book ID"`
}

// RemoveBookFromCollectionInput contains parameters for removing a book from a collection.
type RemoveBookFromCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
	BookID        string `path:"bookId" doc:"Book ID"`
}

// === Handlers ===

func (s *Server) handleListCollections(ctx context.Context, input *ListCollectionsInput) (*ListCollectionsOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	collections, err := s.services.Collection.ListCollections(ctx, userID, input.LibraryID)
	if err != nil {
		return nil, err
	}

	resp := make([]CollectionResponse, len(collections))
	for i, c := range collections {
		resp[i] = CollectionResponse{
			ID:        c.ID,
			LibraryID: c.LibraryID,
			OwnerID:   c.OwnerID,
			Name:      c.Name,
			BookCount: len(c.BookIDs),
			IsInbox:   c.IsInbox,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		}
	}

	return &ListCollectionsOutput{Body: ListCollectionsResponse{Collections: resp}}, nil
}

func (s *Server) handleCreateCollection(ctx context.Context, input *CreateCollectionInput) (*CollectionOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	c, err := s.services.Collection.CreateCollection(ctx, userID, input.LibraryID, input.Body.Name)
	if err != nil {
		return nil, err
	}

	return &CollectionOutput{
		Body: CollectionResponse{
			ID:        c.ID,
			LibraryID: c.LibraryID,
			OwnerID:   c.OwnerID,
			Name:      c.Name,
			BookCount: len(c.BookIDs),
			IsInbox:   c.IsInbox,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleGetCollection(ctx context.Context, input *GetCollectionInput) (*CollectionOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	c, err := s.services.Collection.GetCollection(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	return &CollectionOutput{
		Body: CollectionResponse{
			ID:        c.ID,
			LibraryID: c.LibraryID,
			OwnerID:   c.OwnerID,
			Name:      c.Name,
			BookCount: len(c.BookIDs),
			IsInbox:   c.IsInbox,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleUpdateCollection(ctx context.Context, input *UpdateCollectionInput) (*CollectionOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	c, err := s.services.Collection.UpdateCollection(ctx, userID, input.ID, input.Body.Name)
	if err != nil {
		return nil, err
	}

	return &CollectionOutput{
		Body: CollectionResponse{
			ID:        c.ID,
			LibraryID: c.LibraryID,
			OwnerID:   c.OwnerID,
			Name:      c.Name,
			BookCount: len(c.BookIDs),
			IsInbox:   c.IsInbox,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleDeleteCollection(ctx context.Context, input *DeleteCollectionInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Collection.DeleteCollection(ctx, userID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Collection deleted"}}, nil
}

func (s *Server) handleGetCollectionBooks(ctx context.Context, input *GetCollectionBooksInput) (*CollectionBooksOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	books, err := s.services.Collection.GetCollectionBooks(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	resp := make([]CollectionBookResponse, len(books))
	for i, b := range books {
		book := CollectionBookResponse{
			ID:    b.ID,
			Title: b.Title,
		}
		if b.CoverImage != nil && b.CoverImage.Path != "" {
			book.CoverPath = &b.CoverImage.Path
		}
		resp[i] = book
	}

	return &CollectionBooksOutput{Body: CollectionBooksResponse{Books: resp}}, nil
}

func (s *Server) handleAddBookToCollection(ctx context.Context, input *AddBookToCollectionInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Collection.AddBookToCollection(ctx, userID, input.ID, input.BookID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Book added to collection"}}, nil
}

func (s *Server) handleRemoveBookFromCollection(ctx context.Context, input *RemoveBookFromCollectionInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Collection.RemoveBookFromCollection(ctx, userID, input.ID, input.BookID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Book removed from collection"}}, nil
}
