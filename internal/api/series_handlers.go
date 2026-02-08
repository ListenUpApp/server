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

func (s *Server) registerSeriesRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listSeries",
		Method:      http.MethodGet,
		Path:        "/api/v1/series",
		Summary:     "List series",
		Description: "Returns a paginated list of all series",
		Tags:        []string{"Series"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListSeries)

	huma.Register(s.api, huma.Operation{
		OperationID: "createSeries",
		Method:      http.MethodPost,
		Path:        "/api/v1/series",
		Summary:     "Create series",
		Description: "Creates a new series",
		Tags:        []string{"Series"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateSeries)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSeries",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}",
		Summary:     "Get series",
		Description: "Returns a series by ID",
		Tags:        []string{"Series"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSeries)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateSeries",
		Method:      http.MethodPatch,
		Path:        "/api/v1/series/{id}",
		Summary:     "Update series",
		Description: "Updates a series",
		Tags:        []string{"Series"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateSeries)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteSeries",
		Method:      http.MethodDelete,
		Path:        "/api/v1/series/{id}",
		Summary:     "Delete series",
		Description: "Soft deletes a series",
		Tags:        []string{"Series"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteSeries)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSeriesBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/books",
		Summary:     "Get series books",
		Description: "Returns all books in a series",
		Tags:        []string{"Series"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSeriesBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "mergeSeries",
		Method:      http.MethodPost,
		Path:        "/api/v1/series/merge",
		Summary:     "Merge series",
		Description: "Merges source series into target series",
		Tags:        []string{"Series"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMergeSeries)
}

// === DTOs ===

// ListSeriesInput contains parameters for listing series.
type ListSeriesInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" doc:"Items per page (default 50)"`
}

// SeriesResponse contains series data in API responses.
type SeriesResponse struct {
	ID          string    `json:"id" doc:"Series ID"`
	Name        string    `json:"name" doc:"Series name"`
	Description string    `json:"description,omitempty" doc:"Description"`
	ASIN        string    `json:"asin,omitempty" doc:"Audible ASIN"`
	CreatedAt   time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt   time.Time `json:"updated_at" doc:"Last update time"`
}

// ListSeriesResponse contains a paginated list of series.
type ListSeriesResponse struct {
	Series     []SeriesResponse `json:"series" doc:"List of series"`
	NextCursor string           `json:"next_cursor,omitempty" doc:"Next page cursor"`
	HasMore    bool             `json:"has_more" doc:"Whether more pages exist"`
}

// ListSeriesOutput wraps the list series response for Huma.
type ListSeriesOutput struct {
	Body ListSeriesResponse
}

// CreateSeriesRequest is the request body for creating a series.
type CreateSeriesRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=200" doc:"Series name"`
	Description string `json:"description,omitempty" validate:"omitempty,max=2000" doc:"Description"`
	ASIN        string `json:"asin,omitempty" validate:"omitempty,max=20" doc:"Audible ASIN"`
}

// CreateSeriesInput wraps the create series request for Huma.
type CreateSeriesInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateSeriesRequest
}

// SeriesOutput wraps the series response for Huma.
type SeriesOutput struct {
	Body SeriesResponse
}

// GetSeriesInput contains parameters for getting a series.
type GetSeriesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Series ID"`
}

// UpdateSeriesRequest is the request body for updating a series.
type UpdateSeriesRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=200" doc:"Series name"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000" doc:"Description"`
	ASIN        *string `json:"asin,omitempty" validate:"omitempty,max=20" doc:"Audible ASIN"`
}

// UpdateSeriesInput wraps the update series request for Huma.
type UpdateSeriesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Series ID"`
	Body          UpdateSeriesRequest
}

// DeleteSeriesInput contains parameters for deleting a series.
type DeleteSeriesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Series ID"`
}

// GetSeriesBooksInput contains parameters for getting books in a series.
type GetSeriesBooksInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Series ID"`
}

// SeriesBookResponse represents a book in series responses.
type SeriesBookResponse struct {
	ID             string  `json:"id" doc:"Book ID"`
	Title          string  `json:"title" doc:"Book title"`
	SeriesPosition *string `json:"series_position,omitempty" doc:"Position in series"`
	CoverPath      *string `json:"cover_path,omitempty" doc:"Cover image path"`
}

// SeriesBooksResponse contains books in a series.
type SeriesBooksResponse struct {
	Books []SeriesBookResponse `json:"books" doc:"Books in series"`
}

// SeriesBooksOutput wraps the series books response for Huma.
type SeriesBooksOutput struct {
	Body SeriesBooksResponse
}

// MergeSeriesRequest is the request body for merging series.
type MergeSeriesRequest struct {
	SourceID string `json:"source_id" validate:"required" doc:"Series to merge from"`
	TargetID string `json:"target_id" validate:"required" doc:"Series to merge into"`
}

// MergeSeriesInput wraps the merge series request for Huma.
type MergeSeriesInput struct {
	Authorization string `header:"Authorization"`
	Body          MergeSeriesRequest
}

// === Handlers ===

func (s *Server) handleListSeries(ctx context.Context, input *ListSeriesInput) (*ListSeriesOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	result, err := s.store.ListSeries(ctx, store.PaginationParams{
		Cursor: input.Cursor,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	resp := MapSlice(result.Items, mapSeriesResponse)

	return &ListSeriesOutput{
		Body: ListSeriesResponse{
			Series:     resp,
			NextCursor: result.NextCursor,
			HasMore:    result.HasMore,
		},
	}, nil
}

func (s *Server) handleCreateSeries(ctx context.Context, input *CreateSeriesInput) (*SeriesOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	seriesID, err := id.Generate("sr")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	series := &domain.Series{
		Syncable: domain.Syncable{
			ID:        seriesID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:        input.Body.Name,
		Description: input.Body.Description,
		ASIN:        input.Body.ASIN,
	}

	if err := s.store.CreateSeries(ctx, series); err != nil {
		return nil, err
	}

	return &SeriesOutput{Body: mapSeriesResponse(series)}, nil
}

func (s *Server) handleGetSeries(ctx context.Context, input *GetSeriesInput) (*SeriesOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	series, err := s.store.GetSeries(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &SeriesOutput{Body: mapSeriesResponse(series)}, nil
}

func (s *Server) handleUpdateSeries(ctx context.Context, input *UpdateSeriesInput) (*SeriesOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	series, err := s.store.GetSeries(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if input.Body.Name != nil {
		series.Name = *input.Body.Name
	}
	if input.Body.Description != nil {
		series.Description = *input.Body.Description
	}
	if input.Body.ASIN != nil {
		series.ASIN = *input.Body.ASIN
	}
	series.UpdatedAt = time.Now()

	if err := s.store.UpdateSeries(ctx, series); err != nil {
		return nil, err
	}

	return &SeriesOutput{Body: mapSeriesResponse(series)}, nil
}

func (s *Server) handleDeleteSeries(ctx context.Context, input *DeleteSeriesInput) (*MessageOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	if err := s.store.DeleteSeries(ctx, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Series deleted"}}, nil
}

func (s *Server) handleGetSeriesBooks(ctx context.Context, input *GetSeriesBooksInput) (*SeriesBooksOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	books, err := s.store.GetBooksBySeries(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	accessible, err := s.store.GetAccessibleBookIDSet(ctx, userID)
	if err != nil {
		return nil, err
	}

	var resp []SeriesBookResponse
	for _, b := range books {
		if !accessible[b.ID] {
			continue
		}
		book := SeriesBookResponse{
			ID:    b.ID,
			Title: b.Title,
		}
		// Find series position
		for _, bs := range b.Series {
			if bs.SeriesID == input.ID {
				pos := bs.Sequence
				book.SeriesPosition = &pos
				break
			}
		}
		if b.CoverImage != nil && b.CoverImage.Path != "" {
			book.CoverPath = &b.CoverImage.Path
		}
		resp = append(resp, book)
	}

	return &SeriesBooksOutput{Body: SeriesBooksResponse{Books: resp}}, nil
}

func (s *Server) handleMergeSeries(ctx context.Context, _ *MergeSeriesInput) (*SeriesOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	// TODO: Implement series merging
	return nil, huma.Error501NotImplemented("Series merging not yet implemented")
}

// === Mappers ===

func mapSeriesResponse(series *domain.Series) SeriesResponse {
	return SeriesResponse{
		ID:          series.ID,
		Name:        series.Name,
		Description: series.Description,
		ASIN:        series.ASIN,
		CreatedAt:   series.CreatedAt,
		UpdatedAt:   series.UpdatedAt,
	}
}
