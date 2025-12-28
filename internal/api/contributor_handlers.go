package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
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
		OperationID: "unmergeContributor",
		Method:      http.MethodPost,
		Path:        "/api/v1/contributors/unmerge",
		Summary:     "Unmerge contributor alias",
		Description: "Splits an alias back into a separate contributor",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUnmergeContributor)

	huma.Register(s.api, huma.Operation{
		OperationID: "searchContributors",
		Method:      http.MethodGet,
		Path:        "/api/v1/contributors/search",
		Summary:     "Search contributors",
		Description: "Fast contributor search for autocomplete",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSearchContributors)

	huma.Register(s.api, huma.Operation{
		OperationID: "applyContributorMetadata",
		Method:      http.MethodPost,
		Path:        "/api/v1/contributors/{id}/metadata",
		Summary:     "Apply Audible metadata",
		Description: "Fetches and applies metadata from Audible to a contributor",
		Tags:        []string{"Contributors"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleApplyContributorMetadata)

	// Direct chi route for contributor image serving (no auth required)
	s.router.Get("/api/v1/contributors/{id}/image", s.handleServeContributorImage)
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
	SortName    string `json:"sort_name,omitempty" validate:"omitempty,max=200" doc:"Sort name (e.g., 'King, Stephen')"`
	Description string `json:"description,omitempty" validate:"omitempty,max=5000" doc:"Description"`
	ImageURL    string `json:"image_url,omitempty" validate:"omitempty,url,max=2000" doc:"Image URL"`
	Website     string `json:"website,omitempty" validate:"omitempty,url,max=2000" doc:"Website URL"`
	AudibleASIN string `json:"audible_asin,omitempty" validate:"omitempty,max=20" doc:"Audible ASIN"`
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
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=200" doc:"Contributor name"`
	SortName    *string `json:"sort_name,omitempty" validate:"omitempty,max=200" doc:"Sort name"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=5000" doc:"Description"`
	ImageURL    *string `json:"image_url,omitempty" validate:"omitempty,url,max=2000" doc:"Image URL"`
	Website     *string `json:"website,omitempty" validate:"omitempty,url,max=2000" doc:"Website URL"`
	AudibleASIN *string `json:"audible_asin,omitempty" validate:"omitempty,max=20" doc:"Audible ASIN"`
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

type UnmergeContributorRequest struct {
	SourceID  string `json:"source_id" validate:"required" doc:"Contributor with the alias to split"`
	AliasName string `json:"alias_name" validate:"required,min=1,max=200" doc:"Alias name to split into a new contributor"`
}

type UnmergeContributorInput struct {
	Authorization string `header:"Authorization"`
	Body          UnmergeContributorRequest
}

type SearchContributorsInput struct {
	Authorization string `header:"Authorization"`
	Query         string `query:"q" validate:"required,min=1,max=200" doc:"Search query"`
	Limit         int    `query:"limit" validate:"omitempty,gte=1,lte=100" doc:"Max results (default 10)"`
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

type ApplyContributorMetadataRequest struct {
	ASIN     string                    `json:"asin" doc:"Audible ASIN"`
	ImageURL string                    `json:"image_url,omitempty" doc:"Image URL from search results"`
	Fields   ContributorMetadataFields `json:"fields" doc:"Which fields to apply"`
}

type ContributorMetadataFields struct {
	Name      bool `json:"name" doc:"Apply name from Audible"`
	Biography bool `json:"biography" doc:"Apply biography from Audible"`
	Image     bool `json:"image" doc:"Download and apply image"`
}

type ApplyContributorMetadataInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Contributor ID"`
	Body          ApplyContributorMetadataRequest
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

func (s *Server) handleUnmergeContributor(ctx context.Context, input *UnmergeContributorInput) (*ContributorOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	c, err := s.store.UnmergeContributor(ctx, input.Body.SourceID, input.Body.AliasName)
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

// searchContributorByName searches Audible for contributors by name and returns 409 Conflict with candidates.
// If an ASIN is provided, returns 404 since the contributor doesn't exist locally yet.
func (s *Server) searchContributorByName(ctx context.Context, name string, asin string) (*ContributorOutput, error) {
	// If ASIN provided, the contributor just doesn't exist locally yet - return 404
	if asin != "" {
		return nil, &APIError{
			status:  http.StatusNotFound,
			Code:    "contributor_not_synced",
			Message: "Contributor not found locally. Please wait for sync to complete, then try again.",
		}
	}

	// Search Audible by name
	results, region, err := s.services.Metadata.SearchContributors(ctx, name)
	if err != nil {
		return nil, err
	}

	// Return 409 Conflict with candidates
	candidates := make([]MetadataContributorSearchResultResponse, len(results))
	for i, r := range results {
		candidates[i] = MetadataContributorSearchResultResponse{
			ASIN:        r.ASIN,
			Name:        r.Name,
			ImageURL:    r.ImageURL,
			Description: r.Description,
		}
	}

	var message string
	if len(results) == 0 {
		message = fmt.Sprintf("No matches found for \"%s\" on Audible. Try searching with a different name.", name)
	} else if len(results) == 1 {
		message = fmt.Sprintf("Found contributor \"%s\" on Audible (%s region).", results[0].Name, region)
	} else {
		message = fmt.Sprintf("Multiple contributors found in %s region. Select a contributor and retry with ?asin={asin}", region)
	}

	return nil, &APIError{
		status:  http.StatusConflict,
		Code:    "disambiguation_required",
		Message: message,
		Details: map[string]any{
			"candidates":    candidates,
			"searched_name": name,
		},
	}
}

func (s *Server) handleApplyContributorMetadata(ctx context.Context, input *ApplyContributorMetadataInput) (*ContributorOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	fields := input.Body.Fields
	s.logger.Debug("applying contributor metadata",
		"contributor_id", input.ID,
		"asin", input.Body.ASIN,
		"image_url", input.Body.ImageURL,
		"fields.name", fields.Name,
		"fields.biography", fields.Biography,
		"fields.image", fields.Image,
	)

	// Validate at least one field is selected
	if !fields.Name && !fields.Biography && !fields.Image {
		return nil, &APIError{
			status:  http.StatusBadRequest,
			Code:    "no_fields_selected",
			Message: "At least one field must be selected to apply",
		}
	}

	// Get existing contributor
	contributor, err := s.store.GetContributor(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	// ASIN is required
	asin := input.Body.ASIN
	if asin == "" {
		return nil, &APIError{
			status:  http.StatusBadRequest,
			Code:    "missing_asin",
			Message: "ASIN is required",
		}
	}

	// Fetch profile from Audible
	profile, _, err := s.services.Metadata.GetContributorProfileWithFallback(ctx, asin)
	if err != nil {
		return nil, err
	}

	// Always update ASIN to track the match
	contributor.ASIN = profile.ASIN

	// Apply selected fields only
	if fields.Name && profile.Name != "" {
		contributor.Name = profile.Name
	}
	if fields.Biography && profile.Biography != "" {
		contributor.Biography = profile.Biography
	}

	// Download and store image if selected
	if fields.Image {
		// Prefer profile.ImageURL, fall back to imageUrl from search results
		imageURL := profile.ImageURL
		if imageURL == "" {
			imageURL = input.Body.ImageURL
		}
		if imageURL != "" {
			if err := s.downloadContributorImage(ctx, contributor.ID, imageURL); err != nil {
				s.logger.Warn("Failed to download contributor image",
					"error", err,
					"contributor_id", contributor.ID,
					"image_url", imageURL,
				)
				// Continue without image
			} else {
				contributor.ImageURL = fmt.Sprintf("/api/v1/contributors/%s/image", contributor.ID)
			}
		}
	}

	// Update contributor
	contributor.UpdatedAt = time.Now()
	if err := s.store.UpdateContributor(ctx, contributor); err != nil {
		return nil, err
	}

	return &ContributorOutput{Body: mapContributorResponse(contributor)}, nil
}

func (s *Server) downloadContributorImage(ctx context.Context, contributorID, imageURL string) error {
	downloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, imageURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download image: status %d", resp.StatusCode)
	}

	const maxImageSize = 10 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize))
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}

	return s.storage.ContributorImages.Save(contributorID, data)
}

func (s *Server) handleServeContributorImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	data, err := s.storage.ContributorImages.Get(id)
	if err != nil {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
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
