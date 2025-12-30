package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func (s *Server) registerLensRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listMyLenses",
		Method:      http.MethodGet,
		Path:        "/api/v1/lenses",
		Summary:     "List my lenses",
		Description: "Returns all lenses owned by the current user",
		Tags:        []string{"Lenses"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListMyLenses)

	huma.Register(s.api, huma.Operation{
		OperationID: "discoverLenses",
		Method:      http.MethodGet,
		Path:        "/api/v1/lenses/discover",
		Summary:     "Discover lenses",
		Description: "Returns lenses from other users containing books you can access",
		Tags:        []string{"Lenses"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListDiscoverLenses)

	huma.Register(s.api, huma.Operation{
		OperationID: "createLens",
		Method:      http.MethodPost,
		Path:        "/api/v1/lenses",
		Summary:     "Create lens",
		Description: "Creates a new lens for organizing books",
		Tags:        []string{"Lenses"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateLens)

	huma.Register(s.api, huma.Operation{
		OperationID: "getLens",
		Method:      http.MethodGet,
		Path:        "/api/v1/lenses/{id}",
		Summary:     "Get lens",
		Description: "Returns a lens by ID with its books",
		Tags:        []string{"Lenses"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetLens)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateLens",
		Method:      http.MethodPatch,
		Path:        "/api/v1/lenses/{id}",
		Summary:     "Update lens",
		Description: "Updates lens metadata (owner only)",
		Tags:        []string{"Lenses"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateLens)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteLens",
		Method:      http.MethodDelete,
		Path:        "/api/v1/lenses/{id}",
		Summary:     "Delete lens",
		Description: "Deletes a lens (owner only)",
		Tags:        []string{"Lenses"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteLens)

	huma.Register(s.api, huma.Operation{
		OperationID: "addBooksToLens",
		Method:      http.MethodPost,
		Path:        "/api/v1/lenses/{id}/books",
		Summary:     "Add books to lens",
		Description: "Adds one or more books to a lens (owner only)",
		Tags:        []string{"Lenses"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAddBooksToLens)

	huma.Register(s.api, huma.Operation{
		OperationID: "removeBookFromLens",
		Method:      http.MethodDelete,
		Path:        "/api/v1/lenses/{id}/books/{bookId}",
		Summary:     "Remove book from lens",
		Description: "Removes a book from a lens (owner only)",
		Tags:        []string{"Lenses"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRemoveBookFromLens)
}

// === DTOs ===

// ListMyLensesInput contains parameters for listing user's lenses.
type ListMyLensesInput struct {
	Authorization string `header:"Authorization"`
}

// LensOwnerResponse contains owner information in lens responses.
type LensOwnerResponse struct {
	ID          string `json:"id" doc:"Owner user ID"`
	DisplayName string `json:"display_name" doc:"Owner display name"`
	AvatarColor string `json:"avatar_color" doc:"Owner avatar color"`
}

// LensResponse contains lens data in API responses.
type LensResponse struct {
	ID            string            `json:"id" doc:"Lens ID"`
	Name          string            `json:"name" doc:"Lens name"`
	Description   string            `json:"description" doc:"Lens description"`
	Owner         LensOwnerResponse `json:"owner" doc:"Lens owner"`
	BookCount     int               `json:"book_count" doc:"Number of books in lens"`
	TotalDuration int64             `json:"total_duration" doc:"Total duration in seconds"`
	CreatedAt     time.Time         `json:"created_at" doc:"Creation time"`
	UpdatedAt     time.Time         `json:"updated_at" doc:"Last update time"`
}

// ListLensesResponse contains a list of lenses.
type ListLensesResponse struct {
	Lenses []LensResponse `json:"lenses" doc:"List of lenses"`
}

// ListLensesOutput wraps the list lenses response for Huma.
type ListLensesOutput struct {
	Body ListLensesResponse
}

// CreateLensRequest is the request body for creating a lens.
type CreateLensRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100" doc:"Lens name"`
	Description string `json:"description" validate:"max=500" doc:"Lens description"`
}

// CreateLensInput wraps the create lens request for Huma.
type CreateLensInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateLensRequest
}

// LensOutput wraps the lens response for Huma.
type LensOutput struct {
	Body LensResponse
}

// GetLensInput contains parameters for getting a lens.
type GetLensInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Lens ID"`
}

// LensBookResponse represents a book in lens responses.
type LensBookResponse struct {
	ID              string   `json:"id" doc:"Book ID"`
	Title           string   `json:"title" doc:"Book title"`
	AuthorNames     []string `json:"author_names" doc:"Author names"`
	CoverPath       *string  `json:"cover_path,omitempty" doc:"Cover image path"`
	DurationSeconds int64    `json:"duration_seconds" doc:"Duration in seconds"`
}

// LensDetailResponse contains lens data with books.
type LensDetailResponse struct {
	ID            string             `json:"id" doc:"Lens ID"`
	Name          string             `json:"name" doc:"Lens name"`
	Description   string             `json:"description" doc:"Lens description"`
	Owner         LensOwnerResponse  `json:"owner" doc:"Lens owner"`
	BookCount     int                `json:"book_count" doc:"Number of accessible books"`
	TotalDuration int64              `json:"total_duration" doc:"Total duration of accessible books"`
	Books         []LensBookResponse `json:"books" doc:"Books in lens"`
	CreatedAt     time.Time          `json:"created_at" doc:"Creation time"`
	UpdatedAt     time.Time          `json:"updated_at" doc:"Last update time"`
}

// LensDetailOutput wraps the lens detail response for Huma.
type LensDetailOutput struct {
	Body LensDetailResponse
}

// UpdateLensRequest is the request body for updating a lens.
type UpdateLensRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100" doc:"New lens name"`
	Description string `json:"description" validate:"max=500" doc:"New lens description"`
}

// UpdateLensInput wraps the update lens request for Huma.
type UpdateLensInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Lens ID"`
	Body          UpdateLensRequest
}

// DeleteLensInput contains parameters for deleting a lens.
type DeleteLensInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Lens ID"`
}

// AddBooksToLensRequest is the request body for adding books to a lens.
type AddBooksToLensRequest struct {
	BookIDs []string `json:"book_ids" validate:"required,min=1" doc:"Book IDs to add"`
}

// AddBooksToLensInput wraps the add books request for Huma.
type AddBooksToLensInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Lens ID"`
	Body          AddBooksToLensRequest
}

// RemoveBookFromLensInput contains parameters for removing a book from a lens.
type RemoveBookFromLensInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Lens ID"`
	BookID        string `path:"bookId" doc:"Book ID"`
}

// DiscoverLensesInput contains parameters for discovering lenses.
type DiscoverLensesInput struct {
	Authorization string `header:"Authorization"`
}

// UserLensesResponse contains a user's lenses for discovery.
type UserLensesResponse struct {
	User   LensOwnerResponse `json:"user" doc:"User who owns the lenses"`
	Lenses []LensResponse    `json:"lenses" doc:"Lenses owned by this user"`
}

// DiscoverLensesResponse contains discovered lenses grouped by user.
type DiscoverLensesResponse struct {
	Users []UserLensesResponse `json:"users" doc:"Users with discoverable lenses"`
}

// DiscoverLensesOutput wraps the discover lenses response for Huma.
type DiscoverLensesOutput struct {
	Body DiscoverLensesResponse
}

// === Handlers ===

func (s *Server) handleListMyLenses(ctx context.Context, input *ListMyLensesInput) (*ListLensesOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	lenses, err := s.services.Lens.ListMyLenses(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Get owner info (the current user)
	owner, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := make([]LensResponse, len(lenses))
	for i, lens := range lenses {
		resp[i] = s.mapLensResponse(ctx, lens, owner, userID)
	}

	return &ListLensesOutput{Body: ListLensesResponse{Lenses: resp}}, nil
}

func (s *Server) handleListDiscoverLenses(ctx context.Context, input *DiscoverLensesInput) (*DiscoverLensesOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	lensesByOwner, err := s.services.Lens.ListDiscoverLenses(ctx, userID)
	if err != nil {
		return nil, err
	}

	var users []UserLensesResponse
	for ownerID, ownerLenses := range lensesByOwner {
		owner, err := s.store.GetUser(ctx, ownerID)
		if err != nil {
			// Skip users we can't find
			continue
		}

		lensResponses := make([]LensResponse, len(ownerLenses))
		for i, lens := range ownerLenses {
			lensResponses[i] = s.mapLensResponse(ctx, lens, owner, userID)
		}

		users = append(users, UserLensesResponse{
			User:   s.mapLensOwner(owner),
			Lenses: lensResponses,
		})
	}

	// Return empty array instead of nil if no users found
	if users == nil {
		users = []UserLensesResponse{}
	}

	return &DiscoverLensesOutput{Body: DiscoverLensesResponse{Users: users}}, nil
}

func (s *Server) handleCreateLens(ctx context.Context, input *CreateLensInput) (*LensOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	lens, err := s.services.Lens.CreateLens(ctx, userID, input.Body.Name, input.Body.Description)
	if err != nil {
		return nil, err
	}

	owner, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &LensOutput{Body: s.mapLensResponse(ctx, lens, owner, userID)}, nil
}

func (s *Server) handleGetLens(ctx context.Context, input *GetLensInput) (*LensDetailOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	lens, err := s.services.Lens.GetLens(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	owner, err := s.store.GetUser(ctx, lens.OwnerID)
	if err != nil {
		return nil, err
	}

	// Build list of accessible books with their details
	var books []LensBookResponse
	var totalDuration int64

	for _, bookID := range lens.BookIDs {
		// Check if the requesting user can access this book
		canAccess, err := s.store.CanUserAccessBook(ctx, userID, bookID)
		if err != nil {
			s.logger.Warn("failed to check book access for lens",
				"lens_id", input.ID,
				"book_id", bookID,
				"error", err,
			)
			continue
		}

		if !canAccess {
			continue
		}

		// Get book details - use lens owner's userID to fetch the book
		book, err := s.store.GetBook(ctx, bookID, lens.OwnerID)
		if err != nil {
			s.logger.Warn("failed to get book for lens",
				"lens_id", input.ID,
				"book_id", bookID,
				"error", err,
			)
			continue
		}

		// Enrich book to get author names
		enriched, err := s.store.EnrichBook(ctx, book)
		if err != nil {
			s.logger.Warn("failed to enrich book for lens",
				"lens_id", input.ID,
				"book_id", bookID,
				"error", err,
			)
			// Continue with unenriched book
			enriched = nil
		}

		bookResp := LensBookResponse{
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
				for _, role := range c.Roles {
					if role == domain.RoleAuthor {
						// Get contributor name - for now we don't have it directly
						// The enriched book would have it, so this is a fallback
						break
					}
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
		books = []LensBookResponse{}
	}

	resp := LensDetailResponse{
		ID:            lens.ID,
		Name:          lens.Name,
		Description:   lens.Description,
		Owner:         s.mapLensOwner(owner),
		BookCount:     len(books),
		TotalDuration: totalDuration,
		Books:         books,
		CreatedAt:     lens.CreatedAt,
		UpdatedAt:     lens.UpdatedAt,
	}

	return &LensDetailOutput{Body: resp}, nil
}

func (s *Server) handleUpdateLens(ctx context.Context, input *UpdateLensInput) (*LensOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	lens, err := s.services.Lens.UpdateLens(ctx, userID, input.ID, input.Body.Name, input.Body.Description)
	if err != nil {
		return nil, err
	}

	owner, err := s.store.GetUser(ctx, lens.OwnerID)
	if err != nil {
		return nil, err
	}

	return &LensOutput{Body: s.mapLensResponse(ctx, lens, owner, userID)}, nil
}

func (s *Server) handleDeleteLens(ctx context.Context, input *DeleteLensInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Lens.DeleteLens(ctx, userID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Lens deleted"}}, nil
}

func (s *Server) handleAddBooksToLens(ctx context.Context, input *AddBooksToLensInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	// Add each book to the lens
	for _, bookID := range input.Body.BookIDs {
		if err := s.services.Lens.AddBookToLens(ctx, userID, input.ID, bookID); err != nil {
			return nil, err
		}
	}

	return &MessageOutput{Body: MessageResponse{Message: "Books added to lens"}}, nil
}

func (s *Server) handleRemoveBookFromLens(ctx context.Context, input *RemoveBookFromLensInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Lens.RemoveBookFromLens(ctx, userID, input.ID, input.BookID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Book removed from lens"}}, nil
}

// === Mappers ===

// mapLensResponse converts a domain lens to an API response.
func (s *Server) mapLensResponse(ctx context.Context, lens *domain.Lens, owner *domain.User, requesterID string) LensResponse {
	// Calculate total duration from accessible books
	var totalDuration int64
	accessibleBookCount := 0

	for _, bookID := range lens.BookIDs {
		canAccess, err := s.store.CanUserAccessBook(ctx, requesterID, bookID)
		if err != nil {
			continue
		}
		if !canAccess {
			continue
		}

		// Get book to get duration
		book, err := s.store.GetBook(ctx, bookID, lens.OwnerID)
		if err != nil {
			continue
		}

		totalDuration += book.TotalDuration / 1000 // Convert ms to seconds
		accessibleBookCount++
	}

	return LensResponse{
		ID:            lens.ID,
		Name:          lens.Name,
		Description:   lens.Description,
		Owner:         s.mapLensOwner(owner),
		BookCount:     accessibleBookCount,
		TotalDuration: totalDuration,
		CreatedAt:     lens.CreatedAt,
		UpdatedAt:     lens.UpdatedAt,
	}
}

// mapLensOwner converts a domain user to a lens owner response.
func (s *Server) mapLensOwner(user *domain.User) LensOwnerResponse {
	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.FullName()
		if displayName == "" {
			displayName = user.Email
		}
	}

	// Default avatar color - User struct may get AvatarColor field later
	avatarColor := "#6B7280"

	return LensOwnerResponse{
		ID:          user.ID,
		DisplayName: displayName,
		AvatarColor: avatarColor,
	}
}
