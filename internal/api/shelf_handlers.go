package api

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func (s *Server) registerShelfRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listMyShelves",
		Method:      http.MethodGet,
		Path:        "/api/v1/shelves",
		Summary:     "List my shelves",
		Description: "Returns all shelves owned by the current user",
		Tags:        []string{"Shelves"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListMyShelves)

	huma.Register(s.api, huma.Operation{
		OperationID: "discoverShelves",
		Method:      http.MethodGet,
		Path:        "/api/v1/shelves/discover",
		Summary:     "Discover shelves",
		Description: "Returns shelves from other users containing books you can access",
		Tags:        []string{"Shelves"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListDiscoverShelves)

	huma.Register(s.api, huma.Operation{
		OperationID: "createShelf",
		Method:      http.MethodPost,
		Path:        "/api/v1/shelves",
		Summary:     "Create shelf",
		Description: "Creates a new shelf for organizing books",
		Tags:        []string{"Shelves"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateShelf)

	huma.Register(s.api, huma.Operation{
		OperationID: "getShelf",
		Method:      http.MethodGet,
		Path:        "/api/v1/shelves/{id}",
		Summary:     "Get shelf",
		Description: "Returns a shelf by ID with its books",
		Tags:        []string{"Shelves"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetShelf)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateShelf",
		Method:      http.MethodPatch,
		Path:        "/api/v1/shelves/{id}",
		Summary:     "Update shelf",
		Description: "Updates shelf metadata (owner only)",
		Tags:        []string{"Shelves"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateShelf)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteShelf",
		Method:      http.MethodDelete,
		Path:        "/api/v1/shelves/{id}",
		Summary:     "Delete shelf",
		Description: "Deletes a shelf (owner only)",
		Tags:        []string{"Shelves"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteShelf)

	huma.Register(s.api, huma.Operation{
		OperationID: "addBooksToShelf",
		Method:      http.MethodPost,
		Path:        "/api/v1/shelves/{id}/books",
		Summary:     "Add books to shelf",
		Description: "Adds one or more books to a shelf (owner only)",
		Tags:        []string{"Shelves"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAddBooksToShelf)

	huma.Register(s.api, huma.Operation{
		OperationID: "removeBookFromShelf",
		Method:      http.MethodDelete,
		Path:        "/api/v1/shelves/{id}/books/{bookId}",
		Summary:     "Remove book from shelf",
		Description: "Removes a book from a shelf (owner only)",
		Tags:        []string{"Shelves"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRemoveBookFromShelf)
}

// === DTOs ===

// ListMyShelvesInput contains parameters for listing user's shelves.
type ListMyShelvesInput struct {
	Authorization string `header:"Authorization"`
}

// ShelfOwnerResponse contains owner information in shelf responses.
type ShelfOwnerResponse struct {
	ID          string `json:"id" doc:"Owner user ID"`
	DisplayName string `json:"display_name" doc:"Owner display name"`
	AvatarColor string `json:"avatar_color" doc:"Owner avatar color"`
}

// ShelfResponse contains shelf data in API responses.
type ShelfResponse struct {
	ID            string            `json:"id" doc:"Shelf ID"`
	Name          string            `json:"name" doc:"Shelf name"`
	Description   string            `json:"description" doc:"Shelf description"`
	Owner         ShelfOwnerResponse `json:"owner" doc:"Shelf owner"`
	BookCount     int               `json:"book_count" doc:"Number of books in shelf"`
	TotalDuration int64             `json:"total_duration" doc:"Total duration in seconds"`
	CreatedAt     time.Time         `json:"created_at" doc:"Creation time"`
	UpdatedAt     time.Time         `json:"updated_at" doc:"Last update time"`
}

// ListShelvesResponse contains a list of shelves.
type ListShelvesResponse struct {
	Shelves []ShelfResponse `json:"shelves" doc:"List of shelves"`
}

// ListShelvesOutput wraps the list shelves response for Huma.
type ListShelvesOutput struct {
	Body ListShelvesResponse
}

// CreateShelfRequest is the request body for creating a shelf.
type CreateShelfRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100" doc:"Shelf name"`
	Description string `json:"description" validate:"max=500" doc:"Shelf description"`
}

// CreateShelfInput wraps the create shelf request for Huma.
type CreateShelfInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateShelfRequest
}

// ShelfOutput wraps the shelf response for Huma.
type ShelfOutput struct {
	Body ShelfResponse
}

// GetShelfInput contains parameters for getting a shelf.
type GetShelfInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Shelf ID"`
}

// ShelfBookResponse represents a book in shelf responses.
type ShelfBookResponse struct {
	ID              string   `json:"id" doc:"Book ID"`
	Title           string   `json:"title" doc:"Book title"`
	AuthorNames     []string `json:"author_names" doc:"Author names"`
	CoverPath       *string  `json:"cover_path,omitempty" doc:"Cover image path"`
	DurationSeconds int64    `json:"duration_seconds" doc:"Duration in seconds"`
}

// ShelfDetailResponse contains shelf data with books.
type ShelfDetailResponse struct {
	ID            string             `json:"id" doc:"Shelf ID"`
	Name          string             `json:"name" doc:"Shelf name"`
	Description   string             `json:"description" doc:"Shelf description"`
	Owner         ShelfOwnerResponse  `json:"owner" doc:"Shelf owner"`
	BookCount     int                `json:"book_count" doc:"Number of accessible books"`
	TotalDuration int64              `json:"total_duration" doc:"Total duration of accessible books"`
	Books         []ShelfBookResponse `json:"books" doc:"Books in shelf"`
	CreatedAt     time.Time          `json:"created_at" doc:"Creation time"`
	UpdatedAt     time.Time          `json:"updated_at" doc:"Last update time"`
}

// ShelfDetailOutput wraps the shelf detail response for Huma.
type ShelfDetailOutput struct {
	Body ShelfDetailResponse
}

// UpdateShelfRequest is the request body for updating a shelf.
type UpdateShelfRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100" doc:"New shelf name"`
	Description string `json:"description" validate:"max=500" doc:"New shelf description"`
}

// UpdateShelfInput wraps the update shelf request for Huma.
type UpdateShelfInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Shelf ID"`
	Body          UpdateShelfRequest
}

// DeleteShelfInput contains parameters for deleting a shelf.
type DeleteShelfInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Shelf ID"`
}

// AddBooksToShelfRequest is the request body for adding books to a shelf.
type AddBooksToShelfRequest struct {
	BookIDs []string `json:"book_ids" validate:"required,min=1" doc:"Book IDs to add"`
}

// AddBooksToShelfInput wraps the add books request for Huma.
type AddBooksToShelfInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Shelf ID"`
	Body          AddBooksToShelfRequest
}

// RemoveBookFromShelfInput contains parameters for removing a book from a shelf.
type RemoveBookFromShelfInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Shelf ID"`
	BookID        string `path:"bookId" doc:"Book ID"`
}

// DiscoverShelvesInput contains parameters for discovering shelves.
type DiscoverShelvesInput struct {
	Authorization string `header:"Authorization"`
}

// UserShelvesResponse contains a user's shelves for discovery.
type UserShelvesResponse struct {
	User   ShelfOwnerResponse `json:"user" doc:"User who owns the shelves"`
	Shelves []ShelfResponse    `json:"shelves" doc:"Shelves owned by this user"`
}

// DiscoverShelvesResponse contains discovered shelves grouped by user.
type DiscoverShelvesResponse struct {
	Users []UserShelvesResponse `json:"users" doc:"Users with discoverable shelves"`
}

// DiscoverShelvesOutput wraps the discover shelves response for Huma.
type DiscoverShelvesOutput struct {
	Body DiscoverShelvesResponse
}

// === Handlers ===

func (s *Server) handleListMyShelves(ctx context.Context, _ *ListMyShelvesInput) (*ListShelvesOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	shelves, err := s.services.Shelf.ListMyShelves(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get owner info (the current user)
	owner, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := make([]ShelfResponse, len(shelves))
	for i, shelf := range shelves {
		resp[i] = s.mapShelfResponse(ctx, shelf, owner, userID)
	}

	return &ListShelvesOutput{Body: ListShelvesResponse{Shelves: resp}}, nil
}

func (s *Server) handleListDiscoverShelves(ctx context.Context, _ *DiscoverShelvesInput) (*DiscoverShelvesOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	shelvesByOwner, err := s.services.Shelf.ListDiscoverShelves(ctx, userID)
	if err != nil {
		return nil, err
	}

	var users []UserShelvesResponse
	for ownerID, ownerShelves := range shelvesByOwner {
		owner, err := s.store.GetUser(ctx, ownerID)
		if err != nil {
			// Skip users we can't find
			continue
		}

		shelfResponses := make([]ShelfResponse, len(ownerShelves))
		for i, shelf := range ownerShelves {
			shelfResponses[i] = s.mapShelfResponse(ctx, shelf, owner, userID)
		}

		users = append(users, UserShelvesResponse{
			User:   s.mapShelfOwner(owner),
			Shelves: shelfResponses,
		})
	}

	// Return empty array instead of nil if no users found
	if users == nil {
		users = []UserShelvesResponse{}
	}

	return &DiscoverShelvesOutput{Body: DiscoverShelvesResponse{Users: users}}, nil
}

func (s *Server) handleCreateShelf(ctx context.Context, input *CreateShelfInput) (*ShelfOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	shelf, err := s.services.Shelf.CreateShelf(ctx, userID, input.Body.Name, input.Body.Description)
	if err != nil {
		return nil, err
	}

	owner, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &ShelfOutput{Body: s.mapShelfResponse(ctx, shelf, owner, userID)}, nil
}

func (s *Server) handleGetShelf(ctx context.Context, input *GetShelfInput) (*ShelfDetailOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	shelf, err := s.services.Shelf.GetShelf(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	owner, err := s.store.GetUser(ctx, shelf.OwnerID)
	if err != nil {
		return nil, err
	}

	// Build list of accessible books with their details
	var books []ShelfBookResponse
	var totalDuration int64

	for _, bookID := range shelf.BookIDs {
		// Check if the requesting user can access this book
		canAccess, err := s.store.CanUserAccessBook(ctx, userID, bookID)
		if err != nil {
			s.logger.Warn("failed to check book access for shelf",
				"shelf_id", input.ID,
				"book_id", bookID,
				"error", err,
			)
			continue
		}

		if !canAccess {
			continue
		}

		// Get book details - use shelf owner's userID to fetch the book
		book, err := s.store.GetBook(ctx, bookID, shelf.OwnerID)
		if err != nil {
			s.logger.Warn("failed to get book for shelf",
				"shelf_id", input.ID,
				"book_id", bookID,
				"error", err,
			)
			continue
		}

		// Enrich book to get author names
		enriched, err := s.store.EnrichBook(ctx, book)
		if err != nil {
			s.logger.Warn("failed to enrich book for shelf",
				"shelf_id", input.ID,
				"book_id", bookID,
				"error", err,
			)
			// Continue with unenriched book
			enriched = nil
		}

		bookResp := ShelfBookResponse{
			ID:              book.ID,
			Title:           book.Title,
			DurationSeconds: book.TotalDuration / 1000, // Convert ms to seconds
		}

		// Set cover path if available
		if book.CoverImage != nil && book.CoverImage.Path != "" {
			bookResp.CoverPath = &book.CoverImage.Path
		}

		// Extract author names from enriched book or contributors
		if enriched != nil && enriched.Author != "" {
			bookResp.AuthorNames = []string{enriched.Author}
		} else {
			// Fallback to extracting from contributors
			for _, c := range book.Contributors {
				if slices.Contains(c.Roles, domain.RoleAuthor) {
					// Get contributor name - for now we don't have it directly
					// The enriched book would have it, so this is a fallback

				}
			}
			if bookResp.AuthorNames == nil {
				bookResp.AuthorNames = []string{}
			}
		}

		books = append(books, bookResp)
		totalDuration += book.TotalDuration / 1000 // Convert ms to seconds
	}

	// Ensure books is not nil
	if books == nil {
		books = []ShelfBookResponse{}
	}

	resp := ShelfDetailResponse{
		ID:            shelf.ID,
		Name:          shelf.Name,
		Description:   shelf.Description,
		Owner:         s.mapShelfOwner(owner),
		BookCount:     len(books),
		TotalDuration: totalDuration,
		Books:         books,
		CreatedAt:     shelf.CreatedAt,
		UpdatedAt:     shelf.UpdatedAt,
	}

	return &ShelfDetailOutput{Body: resp}, nil
}

func (s *Server) handleUpdateShelf(ctx context.Context, input *UpdateShelfInput) (*ShelfOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	shelf, err := s.services.Shelf.UpdateShelf(ctx, userID, input.ID, input.Body.Name, input.Body.Description)
	if err != nil {
		return nil, err
	}

	owner, err := s.store.GetUser(ctx, shelf.OwnerID)
	if err != nil {
		return nil, err
	}

	return &ShelfOutput{Body: s.mapShelfResponse(ctx, shelf, owner, userID)}, nil
}

func (s *Server) handleDeleteShelf(ctx context.Context, input *DeleteShelfInput) (*MessageOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Shelf.DeleteShelf(ctx, userID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Shelf deleted"}}, nil
}

func (s *Server) handleAddBooksToShelf(ctx context.Context, input *AddBooksToShelfInput) (*MessageOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Add each book to the shelf
	for _, bookID := range input.Body.BookIDs {
		if err := s.services.Shelf.AddBookToShelf(ctx, userID, input.ID, bookID); err != nil {
			return nil, err
		}
	}

	return &MessageOutput{Body: MessageResponse{Message: "Books added to shelf"}}, nil
}

func (s *Server) handleRemoveBookFromShelf(ctx context.Context, input *RemoveBookFromShelfInput) (*MessageOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.services.Shelf.RemoveBookFromShelf(ctx, userID, input.ID, input.BookID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Book removed from shelf"}}, nil
}

// === Mappers ===

// mapShelfResponse converts a domain shelf to an API response.
func (s *Server) mapShelfResponse(ctx context.Context, shelf *domain.Shelf, owner *domain.User, requesterID string) ShelfResponse {
	// Calculate total duration from accessible books
	var totalDuration int64
	accessibleBookCount := 0

	for _, bookID := range shelf.BookIDs {
		canAccess, err := s.store.CanUserAccessBook(ctx, requesterID, bookID)
		if err != nil {
			continue
		}
		if !canAccess {
			continue
		}

		// Get book to get duration
		book, err := s.store.GetBook(ctx, bookID, shelf.OwnerID)
		if err != nil {
			continue
		}

		totalDuration += book.TotalDuration / 1000 // Convert ms to seconds
		accessibleBookCount++
	}

	return ShelfResponse{
		ID:            shelf.ID,
		Name:          shelf.Name,
		Description:   shelf.Description,
		Owner:         s.mapShelfOwner(owner),
		BookCount:     accessibleBookCount,
		TotalDuration: totalDuration,
		CreatedAt:     shelf.CreatedAt,
		UpdatedAt:     shelf.UpdatedAt,
	}
}

// mapShelfOwner converts a domain user to a shelf owner response.
func (s *Server) mapShelfOwner(user *domain.User) ShelfOwnerResponse {
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.FullName()
		if displayName == "" {
			displayName = user.Email
		}
	}

	// Default avatar color - User struct may get AvatarColor field later
	avatarColor := "#6B7280"

	return ShelfOwnerResponse{
		ID:          user.ID,
		DisplayName: displayName,
		AvatarColor: avatarColor,
	}
}
