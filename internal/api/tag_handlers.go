package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerTagRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listTags",
		Method:      http.MethodGet,
		Path:        "/api/v1/tags",
		Summary:     "List all tags",
		Description: "Returns all global tags ordered by popularity (book count)",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListTags)

	huma.Register(s.api, huma.Operation{
		OperationID: "getTagBySlug",
		Method:      http.MethodGet,
		Path:        "/api/v1/tags/{slug}",
		Summary:     "Get tag by slug",
		Description: "Returns a tag by its normalized slug",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetTagBySlug)

	huma.Register(s.api, huma.Operation{
		OperationID: "getTagBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/tags/{slug}/books",
		Summary:     "Get books with tag",
		Description: "Returns all books with a specific tag, filtered by requester's access",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetTagBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "getBookTags",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}/tags",
		Summary:     "Get tags for book",
		Description: "Returns all tags on a specific book",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBookTags)

	huma.Register(s.api, huma.Operation{
		OperationID: "addTagToBook",
		Method:      http.MethodPost,
		Path:        "/api/v1/books/{id}/tags",
		Summary:     "Add tag to book",
		Description: "Adds a tag to a book. Creates the tag if it doesn't exist. Idempotent.",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAddTagToBook)

	huma.Register(s.api, huma.Operation{
		OperationID: "removeTagFromBook",
		Method:      http.MethodDelete,
		Path:        "/api/v1/books/{id}/tags/{slug}",
		Summary:     "Remove tag from book",
		Description: "Removes a tag from a book. Idempotent if tag exists but isn't on book.",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRemoveTagFromBook)
}

// === DTOs ===

// TagDTO represents a tag in API responses.
type TagDTO struct {
	ID        string    `json:"id" doc:"Tag ID"`
	Slug      string    `json:"slug" doc:"Normalized tag slug (source of truth)"`
	BookCount int       `json:"book_count" doc:"Number of books with this tag"`
	CreatedAt time.Time `json:"created_at" doc:"Creation timestamp"`
}

// ListTagsInput contains parameters for listing tags.
type ListTagsInput struct {
	Authorization string `header:"Authorization"`
}

// ListTagsResponse contains a list of tags.
type ListTagsResponse struct {
	Tags []TagDTO `json:"tags" doc:"List of tags ordered by popularity"`
}

// ListTagsOutput wraps the list tags response for Huma.
type ListTagsOutput struct {
	Body ListTagsResponse
}

// GetTagInput contains parameters for getting a tag.
type GetTagInput struct {
	Authorization string `header:"Authorization"`
	Slug          string `path:"slug" doc:"Tag slug"`
}

// GetTagOutput wraps a single tag response.
type GetTagOutput struct {
	Body TagDTO
}

// GetTagBooksInput contains parameters for getting books with a tag.
type GetTagBooksInput struct {
	Authorization string `header:"Authorization"`
	Slug          string `path:"slug" doc:"Tag slug"`
}

// GetTagBooksResponse contains books with a tag.
type GetTagBooksResponse struct {
	Tag   TagDTO     `json:"tag" doc:"The tag"`
	Books []dto.Book `json:"books" doc:"Books with this tag (filtered by requester access)"`
	Total int        `json:"total" doc:"Total number of accessible books"`
}

// GetTagBooksOutput wraps the tag books response.
type GetTagBooksOutput struct {
	Body GetTagBooksResponse
}

// GetBookTagsInput contains parameters for getting tags on a book.
type GetBookTagsInput struct {
	Authorization string `header:"Authorization"`
	BookID        string `path:"id" doc:"Book ID"`
}

// GetBookTagsResponse contains tags on a book.
type GetBookTagsResponse struct {
	Tags []TagDTO `json:"tags" doc:"Tags on the book"`
}

// GetBookTagsOutput wraps the book tags response.
type GetBookTagsOutput struct {
	Body GetBookTagsResponse
}

// AddTagRequest is the request body for adding a tag.
type AddTagRequest struct {
	Tag string `json:"tag" minLength:"1" maxLength:"50" doc:"Raw tag input (will be normalized)"`
}

// AddTagToBookInput contains parameters for adding a tag to a book.
type AddTagToBookInput struct {
	Authorization string `header:"Authorization"`
	BookID        string `path:"id" doc:"Book ID"`
	Body          AddTagRequest
}

// AddTagToBookOutput wraps the response after adding a tag.
type AddTagToBookOutput struct {
	Body TagDTO
}

// RemoveTagFromBookInput contains parameters for removing a tag from a book.
type RemoveTagFromBookInput struct {
	Authorization string `header:"Authorization"`
	BookID        string `path:"id" doc:"Book ID"`
	Slug          string `path:"slug" doc:"Tag slug to remove"`
}

// === Handlers ===

func (s *Server) handleListTags(ctx context.Context, _ *ListTagsInput) (*ListTagsOutput, error) {
	_, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	tags, err := s.services.Tag.ListTags(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]TagDTO, len(tags))
	for i, t := range tags {
		resp[i] = TagDTO{
			ID:        t.ID,
			Slug:      t.Slug,
			BookCount: t.BookCount,
			CreatedAt: t.CreatedAt,
		}
	}

	return &ListTagsOutput{Body: ListTagsResponse{Tags: resp}}, nil
}

func (s *Server) handleGetTagBySlug(ctx context.Context, input *GetTagInput) (*GetTagOutput, error) {
	_, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	t, err := s.services.Tag.GetTagBySlug(ctx, input.Slug)
	if errors.Is(err, service.ErrTagNotFound) {
		return nil, huma.Error404NotFound("tag not found")
	}
	if err != nil {
		return nil, err
	}

	return &GetTagOutput{
		Body: TagDTO{
			ID:        t.ID,
			Slug:      t.Slug,
			BookCount: t.BookCount,
			CreatedAt: t.CreatedAt,
		},
	}, nil
}

func (s *Server) handleGetTagBooks(ctx context.Context, input *GetTagBooksInput) (*GetTagBooksOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Get the tag first
	tag, err := s.services.Tag.GetTagBySlug(ctx, input.Slug)
	if errors.Is(err, service.ErrTagNotFound) {
		return nil, huma.Error404NotFound("tag not found")
	}
	if err != nil {
		return nil, err
	}

	// Get books with this tag (filtered by user access)
	books, err := s.services.Tag.GetBooksForTag(ctx, userID, input.Slug)
	if err != nil {
		return nil, err
	}

	// Convert to DTOs (using existing dto.Book from the codebase)
	bookDTOs := make([]dto.Book, 0, len(books))
	for _, b := range books {
		enriched, err := s.store.EnrichBook(ctx, b)
		if err != nil {
			continue // Skip books that can't be enriched
		}
		bookDTOs = append(bookDTOs, *enriched)
	}

	return &GetTagBooksOutput{
		Body: GetTagBooksResponse{
			Tag: TagDTO{
				ID:        tag.ID,
				Slug:      tag.Slug,
				BookCount: tag.BookCount,
				CreatedAt: tag.CreatedAt,
			},
			Books: bookDTOs,
			Total: len(bookDTOs),
		},
	}, nil
}

func (s *Server) handleGetBookTags(ctx context.Context, input *GetBookTagsInput) (*GetBookTagsOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Check if user can access this book
	canAccess, err := s.store.CanUserAccessBook(ctx, userID, input.BookID)
	if errors.Is(err, store.ErrBookNotFound) {
		return nil, huma.Error404NotFound("book not found")
	}
	if err != nil {
		return nil, err
	}
	if !canAccess {
		return nil, huma.Error403Forbidden("access denied")
	}

	tags, err := s.services.Tag.GetTagsForBook(ctx, input.BookID)
	if err != nil {
		return nil, err
	}

	resp := make([]TagDTO, len(tags))
	for i, t := range tags {
		resp[i] = TagDTO{
			ID:        t.ID,
			Slug:      t.Slug,
			BookCount: t.BookCount,
			CreatedAt: t.CreatedAt,
		}
	}

	return &GetBookTagsOutput{Body: GetBookTagsResponse{Tags: resp}}, nil
}

func (s *Server) handleAddTagToBook(ctx context.Context, input *AddTagToBookInput) (*AddTagToBookOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	tag, _, err := s.services.Tag.AddTagToBook(ctx, userID, input.BookID, input.Body.Tag)
	if errors.Is(err, service.ErrForbidden) {
		return nil, huma.Error403Forbidden("access denied")
	}
	if errors.Is(err, service.ErrInvalidTagSlug) {
		return nil, huma.Error422UnprocessableEntity("tag is empty after normalization")
	}
	if errors.Is(err, store.ErrBookNotFound) {
		return nil, huma.Error404NotFound("book not found")
	}
	if err != nil {
		return nil, err
	}

	return &AddTagToBookOutput{
		Body: TagDTO{
			ID:        tag.ID,
			Slug:      tag.Slug,
			BookCount: tag.BookCount,
			CreatedAt: tag.CreatedAt,
		},
	}, nil
}

func (s *Server) handleRemoveTagFromBook(ctx context.Context, input *RemoveTagFromBookInput) (*MessageOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.services.Tag.RemoveTagFromBook(ctx, userID, input.BookID, input.Slug)
	if errors.Is(err, service.ErrForbidden) {
		return nil, huma.Error403Forbidden("access denied")
	}
	if errors.Is(err, service.ErrTagNotFound) {
		return nil, huma.Error404NotFound("tag not found")
	}
	if errors.Is(err, store.ErrBookNotFound) {
		return nil, huma.Error404NotFound("book not found")
	}
	if err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Tag removed"}}, nil
}
