package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerContributorRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listContributors",
		Method:      http.MethodGet,
		Path:        "/api/v1/contributors",
		Summary:     "List contributors",
		Description: "Returns a paginated list of all contributors",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListContributors)

	huma.Register(s.api, huma.Operation{
		OperationID: "createContributor",
		Method:      http.MethodPost,
		Path:        "/api/v1/contributors",
		Summary:     "Create contributor",
		Description: "Creates a new contributor",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateContributor)

	huma.Register(s.api, huma.Operation{
		OperationID: "getContributor",
		Method:      http.MethodGet,
		Path:        "/api/v1/contributors/{id}",
		Summary:     "Get contributor",
		Description: "Returns a contributor by ID",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetContributor)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateContributor",
		Method:      http.MethodPatch,
		Path:        "/api/v1/contributors/{id}",
		Summary:     "Update contributor",
		Description: "Updates a contributor",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateContributor)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteContributor",
		Method:      http.MethodDelete,
		Path:        "/api/v1/contributors/{id}",
		Summary:     "Delete contributor",
		Description: "Soft deletes a contributor",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteContributor)

	huma.Register(s.api, huma.Operation{
		OperationID: "getContributorBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/contributors/{id}/books",
		Summary:     "Get contributor books",
		Description: "Returns all books by a contributor",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetContributorBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "mergeContributors",
		Method:      http.MethodPost,
		Path:        "/api/v1/contributors/merge",
		Summary:     "Merge contributors",
		Description: "Merges source contributor into target contributor",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMergeContributors)

	huma.Register(s.api, huma.Operation{
		OperationID: "searchContributors",
		Method:      http.MethodGet,
		Path:        "/api/v1/contributors/search",
		Summary:     "Search contributors",
		Description: "Fast contributor search for autocomplete",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSearchContributors)
}

// === DTOs ===

type ListContributorsInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" doc:"Items per page (default 50)"`
}

type ContributorResponse struct {
	ID          string    `json:"id" doc:"Contributor ID"`
	Name        string    `json:"name" doc:"Contributor name"`
	SortName    string    `json:"sort_name,omitempty" doc:"Sort name"`
	Description string    `json:"description,omitempty" doc:"Description"`
	ImageURL    string    `json:"image_url,omitempty" doc:"Image URL"`
	Website     string    `json:"website,omitempty" doc:"Website URL"`
	AudibleASIN string    `json:"audible_asin,omitempty" doc:"Audible ASIN"`
	CreatedAt   time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt   time.Time `json:"updated_at" doc:"Last update time"`
}

type ListContributorsResponse struct {
	Contributors []ContributorResponse `json:"contributors" doc:"List of contributors"`
	NextCursor   string                `json:"next_cursor,omitempty" doc:"Next page cursor"`
	HasMore      bool                  `json:"has_more" doc:"Whether more pages exist"`
}

type ListContributorsOutput struct {
	Body ListContributorsResponse
}

type CreateContributorRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=200" doc:"Contributor name"`
	SortName    string `json:"sort_name,omitempty" doc:"Sort name (e.g., 'King, Stephen')"`
	Description string `json:"description,omitempty" doc:"Description"`
	ImageURL    string `json:"image_url,omitempty" doc:"Image URL"`
	Website     string `json:"website,omitempty" doc:"Website URL"`
	AudibleASIN string `json:"audible_asin,omitempty" doc:"Audible ASIN"`
}

type CreateContributorInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateContributorRequest
}

type ContributorOutput struct {
	Body ContributorResponse
}

type GetContributorInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Contributor ID"`
}

type UpdateContributorRequest struct {
	Name        *string `json:"name,omitempty" doc:"Contributor name"`
	SortName    *string `json:"sort_name,omitempty" doc:"Sort name"`
	Description *string `json:"description,omitempty" doc:"Description"`
	ImageURL    *string `json:"image_url,omitempty" doc:"Image URL"`
	Website     *string `json:"website,omitempty" doc:"Website URL"`
	AudibleASIN *string `json:"audible_asin,omitempty" doc:"Audible ASIN"`
}

type UpdateContributorInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Contributor ID"`
	Body          UpdateContributorRequest
}

type DeleteContributorInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Contributor ID"`
}

type GetContributorBooksInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Contributor ID"`
}

type ContributorBookResponse struct {
	ID        string   `json:"id" doc:"Book ID"`
	Title     string   `json:"title" doc:"Book title"`
	Roles     []string `json:"roles" doc:"Roles on this book"`
	CoverPath *string  `json:"cover_path,omitempty" doc:"Cover image path"`
}

type ContributorBooksResponse struct {
	Books []ContributorBookResponse `json:"books" doc:"Books by contributor"`
}

type ContributorBooksOutput struct {
	Body ContributorBooksResponse
}

type MergeContributorsRequest struct {
	SourceID string `json:"source_id" validate:"required" doc:"Contributor to merge from"`
	TargetID string `json:"target_id" validate:"required" doc:"Contributor to merge into"`
}

type MergeContributorsInput struct {
	Authorization string `header:"Authorization"`
	Body          MergeContributorsRequest
}

type SearchContributorsInput struct {
	Authorization string `header:"Authorization"`
	Query         string `query:"q" validate:"required,min=1" doc:"Search query"`
	Limit         int    `query:"limit" doc:"Max results (default 10)"`
}

type ContributorSearchResult struct {
	ID        string `json:"id" doc:"Contributor ID"`
	Name      string `json:"name" doc:"Contributor name"`
	BookCount int    `json:"book_count" doc:"Number of books"`
}

type SearchContributorsResponse struct {
	Results []ContributorSearchResult `json:"results" doc:"Search results"`
}

type SearchContributorsOutput struct {
	Body SearchContributorsResponse
}

// === Handlers ===

func (s *Server) handleListContributors(ctx context.Context, input *ListContributorsInput) (*ListContributorsOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	result, err := s.store.ListContributors(ctx, store.PaginationParams{
		Cursor: input.Cursor,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	resp := make([]ContributorResponse, len(result.Items))
	for i, c := range result.Items {
		resp[i] = mapContributorResponse(c)
	}

	return &ListContributorsOutput{
		Body: ListContributorsResponse{
			Contributors: resp,
			NextCursor:   result.NextCursor,
			HasMore:      result.HasMore,
		},
	}, nil
}

func (s *Server) handleCreateContributor(ctx context.Context, input *CreateContributorInput) (*ContributorOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	contribID, err := id.Generate("ct")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	c := &domain.Contributor{
		Syncable: domain.Syncable{
			ID:        contribID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:      input.Body.Name,
		SortName:  input.Body.SortName,
		Biography: input.Body.Description,
		ImageURL:  input.Body.ImageURL,
		Website:   input.Body.Website,
		ASIN:      input.Body.AudibleASIN,
	}

	if err := s.store.CreateContributor(ctx, c); err != nil {
		return nil, err
	}

	return &ContributorOutput{Body: mapContributorResponse(c)}, nil
}

func (s *Server) handleGetContributor(ctx context.Context, input *GetContributorInput) (*ContributorOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	c, err := s.store.GetContributor(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &ContributorOutput{Body: mapContributorResponse(c)}, nil
}

func (s *Server) handleUpdateContributor(ctx context.Context, input *UpdateContributorInput) (*ContributorOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	c, err := s.store.GetContributor(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if input.Body.Name != nil {
		c.Name = *input.Body.Name
	}
	if input.Body.SortName != nil {
		c.SortName = *input.Body.SortName
	}
	if input.Body.Description != nil {
		c.Biography = *input.Body.Description
	}
	if input.Body.ImageURL != nil {
		c.ImageURL = *input.Body.ImageURL
	}
	if input.Body.Website != nil {
		c.Website = *input.Body.Website
	}
	if input.Body.AudibleASIN != nil {
		c.ASIN = *input.Body.AudibleASIN
	}

	if err := s.store.UpdateContributor(ctx, c); err != nil {
		return nil, err
	}

	return &ContributorOutput{Body: mapContributorResponse(c)}, nil
}

func (s *Server) handleDeleteContributor(ctx context.Context, input *DeleteContributorInput) (*MessageOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	if err := s.store.DeleteContributor(ctx, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Contributor deleted"}}, nil
}

func (s *Server) handleGetContributorBooks(ctx context.Context, input *GetContributorBooksInput) (*ContributorBooksOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	books, err := s.store.GetBooksByContributor(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	resp := make([]ContributorBookResponse, len(books))
	for i, b := range books {
		book := ContributorBookResponse{
			ID:    b.ID,
			Title: b.Title,
		}
		// Find roles for this contributor
		for _, bc := range b.Contributors {
			if bc.ContributorID == input.ID {
				roles := make([]string, len(bc.Roles))
				for j, r := range bc.Roles {
					roles[j] = string(r)
				}
				book.Roles = roles
				break
			}
		}
		if b.CoverImage != nil && b.CoverImage.Path != "" {
			book.CoverPath = &b.CoverImage.Path
		}
		resp[i] = book
	}

	return &ContributorBooksOutput{Body: ContributorBooksResponse{Books: resp}}, nil
}

func (s *Server) handleMergeContributors(ctx context.Context, input *MergeContributorsInput) (*ContributorOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	c, err := s.store.MergeContributors(ctx, input.Body.SourceID, input.Body.TargetID)
	if err != nil {
		return nil, err
	}

	return &ContributorOutput{Body: mapContributorResponse(c)}, nil
}

func (s *Server) handleSearchContributors(ctx context.Context, input *SearchContributorsInput) (*SearchContributorsOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	results, err := s.services.Search.SearchContributors(ctx, input.Query, limit)
	if err != nil {
		return nil, err
	}

	resp := make([]ContributorSearchResult, len(results))
	for i, r := range results {
		resp[i] = ContributorSearchResult{
			ID:        r.ID,
			Name:      r.Name,
			BookCount: r.BookCount,
		}
	}

	return &SearchContributorsOutput{Body: SearchContributorsResponse{Results: resp}}, nil
}

// === Mappers ===

func mapContributorResponse(c *domain.Contributor) ContributorResponse {
	return ContributorResponse{
		ID:          c.ID,
		Name:        c.Name,
		SortName:    c.SortName,
		Description: c.Biography,
		ImageURL:    c.ImageURL,
		Website:     c.Website,
		AudibleASIN: c.ASIN,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}
