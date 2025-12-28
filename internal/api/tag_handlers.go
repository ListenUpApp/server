package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerTagRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listTags",
		Method:      http.MethodGet,
		Path:        "/api/v1/tags",
		Summary:     "List tags",
		Description: "Returns all tags for the current user",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListTags)

	huma.Register(s.api, huma.Operation{
		OperationID: "createTag",
		Method:      http.MethodPost,
		Path:        "/api/v1/tags",
		Summary:     "Create tag",
		Description: "Creates a new tag",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateTag)

	huma.Register(s.api, huma.Operation{
		OperationID: "getTag",
		Method:      http.MethodGet,
		Path:        "/api/v1/tags/{id}",
		Summary:     "Get tag",
		Description: "Returns a tag by ID",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetTag)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateTag",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tags/{id}",
		Summary:     "Update tag",
		Description: "Updates a tag",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateTag)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteTag",
		Method:      http.MethodDelete,
		Path:        "/api/v1/tags/{id}",
		Summary:     "Delete tag",
		Description: "Deletes a tag",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteTag)

	huma.Register(s.api, huma.Operation{
		OperationID: "getTagBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/tags/{id}/books",
		Summary:     "Get tag books",
		Description: "Returns book IDs with this tag",
		Tags:        []string{"Tags"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetTagBooks)
}

// === DTOs ===

// ListTagsInput contains parameters for listing tags.
type ListTagsInput struct {
	Authorization string `header:"Authorization"`
}

// TagResponse contains tag data in API responses.
type TagResponse struct {
	ID        string    `json:"id" doc:"Tag ID"`
	Name      string    `json:"name" doc:"Tag name"`
	Slug      string    `json:"slug" doc:"URL-safe slug"`
	Color     string    `json:"color,omitempty" doc:"Display color"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt time.Time `json:"updated_at" doc:"Last update time"`
}

// ListTagsResponse contains a list of tags.
type ListTagsResponse struct {
	Tags []TagResponse `json:"tags" doc:"List of tags"`
}

// ListTagsOutput wraps the list tags response for Huma.
type ListTagsOutput struct {
	Body ListTagsResponse
}

// CreateTagRequest is the request body for creating a tag.
type CreateTagRequest struct {
	Name  string `json:"name" validate:"required,min=1,max=50" doc:"Tag name"`
	Color string `json:"color,omitempty" validate:"omitempty,max=20" doc:"Display color"`
}

// CreateTagInput wraps the create tag request for Huma.
type CreateTagInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateTagRequest
}

// TagOutput wraps the tag response for Huma.
type TagOutput struct {
	Body TagResponse
}

// GetTagInput contains parameters for getting a tag.
type GetTagInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Tag ID"`
}

// UpdateTagRequest is the request body for updating a tag.
type UpdateTagRequest struct {
	Name  *string `json:"name,omitempty" validate:"omitempty,min=1,max=50" doc:"Tag name"`
	Color *string `json:"color,omitempty" validate:"omitempty,max=20" doc:"Display color"`
}

// UpdateTagInput wraps the update tag request for Huma.
type UpdateTagInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Tag ID"`
	Body          UpdateTagRequest
}

// DeleteTagInput contains parameters for deleting a tag.
type DeleteTagInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Tag ID"`
}

// GetTagBooksInput contains parameters for getting tag books.
type GetTagBooksInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Tag ID"`
}

// TagBooksResponse contains book IDs with a tag.
type TagBooksResponse struct {
	BookIDs []string `json:"book_ids" doc:"Book IDs with this tag"`
}

// TagBooksOutput wraps the tag books response for Huma.
type TagBooksOutput struct {
	Body TagBooksResponse
}

// === Handlers ===

func (s *Server) handleListTags(ctx context.Context, input *ListTagsInput) (*ListTagsOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	tags, err := s.services.Tag.ListTags(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := make([]TagResponse, len(tags))
	for i, t := range tags {
		resp[i] = TagResponse{
			ID:        t.ID,
			Name:      t.Name,
			Slug:      t.Slug,
			Color:     t.Color,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
		}
	}

	return &ListTagsOutput{Body: ListTagsResponse{Tags: resp}}, nil
}

func (s *Server) handleCreateTag(ctx context.Context, input *CreateTagInput) (*TagOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	t, err := s.services.Tag.CreateTag(ctx, userID, service.CreateTagRequest{
		Name:  input.Body.Name,
		Color: input.Body.Color,
	})
	if err != nil {
		return nil, err
	}

	return &TagOutput{
		Body: TagResponse{
			ID:        t.ID,
			Name:      t.Name,
			Slug:      t.Slug,
			Color:     t.Color,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleGetTag(ctx context.Context, input *GetTagInput) (*TagOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	t, err := s.services.Tag.GetTag(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	return &TagOutput{
		Body: TagResponse{
			ID:        t.ID,
			Name:      t.Name,
			Slug:      t.Slug,
			Color:     t.Color,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleUpdateTag(ctx context.Context, input *UpdateTagInput) (*TagOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	t, err := s.services.Tag.UpdateTag(ctx, userID, input.ID, service.UpdateTagRequest{
		Name:  input.Body.Name,
		Color: input.Body.Color,
	})
	if err != nil {
		return nil, err
	}

	return &TagOutput{
		Body: TagResponse{
			ID:        t.ID,
			Name:      t.Name,
			Slug:      t.Slug,
			Color:     t.Color,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleDeleteTag(ctx context.Context, input *DeleteTagInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Tag.DeleteTag(ctx, userID, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Tag deleted"}}, nil
}

func (s *Server) handleGetTagBooks(ctx context.Context, input *GetTagBooksInput) (*TagBooksOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	bookIDs, err := s.services.Tag.GetBooksForTag(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	return &TagBooksOutput{Body: TagBooksResponse{BookIDs: bookIDs}}, nil
}
