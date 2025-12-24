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

type ListSeriesInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" doc:"Items per page (default 50)"`
}

type SeriesResponse struct {
	ID          string    `json:"id" doc:"Series ID"`
	Name        string    `json:"name" doc:"Series name"`
	Description string    `json:"description,omitempty" doc:"Description"`
	ASIN        string    `json:"asin,omitempty" doc:"Audible ASIN"`
	CreatedAt   time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt   time.Time `json:"updated_at" doc:"Last update time"`
}

type ListSeriesResponse struct {
	Series     []SeriesResponse `json:"series" doc:"List of series"`
	NextCursor string           `json:"next_cursor,omitempty" doc:"Next page cursor"`
	HasMore    bool             `json:"has_more" doc:"Whether more pages exist"`
}

type ListSeriesOutput struct {
	Body ListSeriesResponse
}

type CreateSeriesRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=200" doc:"Series name"`
	Description string `json:"description,omitempty" doc:"Description"`
	ASIN        string `json:"asin,omitempty" doc:"Audible ASIN"`
}

type CreateSeriesInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateSeriesRequest
}

type SeriesOutput struct {
	Body SeriesResponse
}

type GetSeriesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Series ID"`
}

type UpdateSeriesRequest struct {
	Name        *string `json:"name,omitempty" doc:"Series name"`
	Description *string `json:"description,omitempty" doc:"Description"`
	ASIN        *string `json:"asin,omitempty" doc:"Audible ASIN"`
}

type UpdateSeriesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Series ID"`
	Body          UpdateSeriesRequest
}

type DeleteSeriesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Series ID"`
}

type GetSeriesBooksInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Series ID"`
}

type SeriesBookResponse struct {
	ID             string  `json:"id" doc:"Book ID"`
	Title          string  `json:"title" doc:"Book title"`
	SeriesPosition *string `json:"series_position,omitempty" doc:"Position in series"`
	CoverPath      *string `json:"cover_path,omitempty" doc:"Cover image path"`
}

type SeriesBooksResponse struct {
	Books []SeriesBookResponse `json:"books" doc:"Books in series"`
}

type SeriesBooksOutput struct {
	Body SeriesBooksResponse
}

type MergeSeriesRequest struct {
	SourceID string `json:"source_id" validate:"required" doc:"Series to merge from"`
	TargetID string `json:"target_id" validate:"required" doc:"Series to merge into"`
}

type MergeSeriesInput struct {
	Authorization string `header:"Authorization"`
	Body          MergeSeriesRequest
}

// === Handlers ===

func (s *Server) handleListSeries(ctx context.Context, input *ListSeriesInput) (*ListSeriesOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
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

	resp := make([]SeriesResponse, len(result.Items))
	for i, series := range result.Items {
		resp[i] = mapSeriesResponse(series)
	}

	return &ListSeriesOutput{
		Body: ListSeriesResponse{
			Series:     resp,
			NextCursor: result.NextCursor,
			HasMore:    result.HasMore,
		},
	}, nil
}

func (s *Server) handleCreateSeries(ctx context.Context, input *CreateSeriesInput) (*SeriesOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
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
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	series, err := s.store.GetSeries(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &SeriesOutput{Body: mapSeriesResponse(series)}, nil
}

func (s *Server) handleUpdateSeries(ctx context.Context, input *UpdateSeriesInput) (*SeriesOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
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
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	if err := s.store.DeleteSeries(ctx, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Series deleted"}}, nil
}

func (s *Server) handleGetSeriesBooks(ctx context.Context, input *GetSeriesBooksInput) (*SeriesBooksOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	books, err := s.store.GetBooksBySeries(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	resp := make([]SeriesBookResponse, len(books))
	for i, b := range books {
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
		resp[i] = book
	}

	return &SeriesBooksOutput{Body: SeriesBooksResponse{Books: resp}}, nil
}

func (s *Server) handleMergeSeries(ctx context.Context, input *MergeSeriesInput) (*SeriesOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
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
