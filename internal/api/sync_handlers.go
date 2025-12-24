package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerSyncRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncManifest",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/manifest",
		Summary:     "Get sync manifest",
		Description: "Returns library manifest with book IDs and counts for initial sync",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncManifest)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/books",
		Summary:     "Get books for sync",
		Description: "Returns paginated books with optional delta sync support",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncContributors",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/contributors",
		Summary:     "Get contributors for sync",
		Description: "Returns paginated contributors with optional delta sync support",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncContributors)

	huma.Register(s.api, huma.Operation{
		OperationID: "getSyncSeries",
		Method:      http.MethodGet,
		Path:        "/api/v1/sync/series",
		Summary:     "Get series for sync",
		Description: "Returns paginated series with optional delta sync support",
		Tags:        []string{"Sync"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetSyncSeries)

	// SSE endpoint (handled via chi directly, not huma)
	s.router.Get("/api/v1/sync/events", s.sseHandler.ServeHTTP)
}

// === DTOs ===

type GetSyncManifestInput struct {
	Authorization string `header:"Authorization"`
}

type SyncManifestCountsResponse struct {
	Books        int `json:"books" doc:"Total books"`
	Contributors int `json:"contributors" doc:"Total contributors"`
	Series       int `json:"series" doc:"Total series"`
}

type SyncManifestResponse struct {
	LibraryVersion string                     `json:"library_version" doc:"Library version timestamp"`
	Checkpoint     string                     `json:"checkpoint" doc:"Checkpoint for delta sync"`
	BookIDs        []string                   `json:"book_ids" doc:"All book IDs"`
	Counts         SyncManifestCountsResponse `json:"counts" doc:"Entity counts"`
}

type SyncManifestOutput struct {
	Body SyncManifestResponse
}

type GetSyncBooksInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" doc:"Items per page (default 50)"`
	UpdatedAfter  string `query:"updated_after" doc:"For delta sync, only return items updated after this time (RFC3339)"`
}

type SyncBooksResponse struct {
	NextCursor     string      `json:"next_cursor,omitempty" doc:"Next page cursor"`
	Books          []*dto.Book `json:"books" doc:"Books"`
	DeletedBookIDs []string    `json:"deleted_book_ids,omitempty" doc:"Deleted book IDs (for delta sync)"`
	HasMore        bool        `json:"has_more" doc:"Whether more pages exist"`
}

type SyncBooksOutput struct {
	Body SyncBooksResponse
}

type GetSyncContributorsInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" doc:"Items per page (default 50)"`
	UpdatedAfter  string `query:"updated_after" doc:"For delta sync, only return items updated after this time (RFC3339)"`
}

type SyncContributorResponse struct {
	ID        string    `json:"id" doc:"Contributor ID"`
	Name      string    `json:"name" doc:"Name"`
	SortName  string    `json:"sort_name,omitempty" doc:"Sort name"`
	Biography string    `json:"biography,omitempty" doc:"Biography"`
	ImageURL  string    `json:"image_url,omitempty" doc:"Image URL"`
	Website   string    `json:"website,omitempty" doc:"Website"`
	ASIN      string    `json:"asin,omitempty" doc:"Audible ASIN"`
	CreatedAt time.Time `json:"created_at" doc:"Created time"`
	UpdatedAt time.Time `json:"updated_at" doc:"Updated time"`
}

type SyncContributorsResponse struct {
	NextCursor            string                    `json:"next_cursor,omitempty" doc:"Next page cursor"`
	Contributors          []SyncContributorResponse `json:"contributors" doc:"Contributors"`
	DeletedContributorIDs []string                  `json:"deleted_contributor_ids,omitempty" doc:"Deleted contributor IDs (for delta sync)"`
	HasMore               bool                      `json:"has_more" doc:"Whether more pages exist"`
}

type SyncContributorsOutput struct {
	Body SyncContributorsResponse
}

type GetSyncSeriesInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	Limit         int    `query:"limit" doc:"Items per page (default 50)"`
	UpdatedAfter  string `query:"updated_after" doc:"For delta sync, only return items updated after this time (RFC3339)"`
}

type SyncSeriesItemResponse struct {
	ID          string    `json:"id" doc:"Series ID"`
	Name        string    `json:"name" doc:"Name"`
	Description string    `json:"description,omitempty" doc:"Description"`
	ASIN        string    `json:"asin,omitempty" doc:"Audible ASIN"`
	CreatedAt   time.Time `json:"created_at" doc:"Created time"`
	UpdatedAt   time.Time `json:"updated_at" doc:"Updated time"`
}

type SyncSeriesResponse struct {
	NextCursor       string                   `json:"next_cursor,omitempty" doc:"Next page cursor"`
	Series           []SyncSeriesItemResponse `json:"series" doc:"Series"`
	DeletedSeriesIDs []string                 `json:"deleted_series_ids,omitempty" doc:"Deleted series IDs (for delta sync)"`
	HasMore          bool                     `json:"has_more" doc:"Whether more pages exist"`
}

type SyncSeriesOutput struct {
	Body SyncSeriesResponse
}

// === Handlers ===

func (s *Server) handleGetSyncManifest(ctx context.Context, input *GetSyncManifestInput) (*SyncManifestOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	manifest, err := s.services.Sync.GetManifest(ctx)
	if err != nil {
		return nil, err
	}

	return &SyncManifestOutput{
		Body: SyncManifestResponse{
			LibraryVersion: manifest.LibraryVersion,
			Checkpoint:     manifest.Checkpoint,
			BookIDs:        manifest.BookIDs,
			Counts: SyncManifestCountsResponse{
				Books:        manifest.Counts.Books,
				Contributors: manifest.Counts.Contributors,
				Series:       manifest.Counts.Series,
			},
		},
	}, nil
}

func (s *Server) handleGetSyncBooks(ctx context.Context, input *GetSyncBooksInput) (*SyncBooksOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	params := store.PaginationParams{
		Cursor: input.Cursor,
		Limit:  limit,
	}
	if input.UpdatedAfter != "" {
		t, err := time.Parse(time.RFC3339, input.UpdatedAfter)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid updated_after format, expected RFC3339")
		}
		params.UpdatedAfter = t
	}

	result, err := s.services.Sync.GetBooksForSync(ctx, userID, params)
	if err != nil {
		return nil, err
	}

	return &SyncBooksOutput{
		Body: SyncBooksResponse{
			NextCursor:     result.NextCursor,
			Books:          result.Books,
			DeletedBookIDs: result.DeletedBookIDs,
			HasMore:        result.HasMore,
		},
	}, nil
}

func (s *Server) handleGetSyncContributors(ctx context.Context, input *GetSyncContributorsInput) (*SyncContributorsOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	params := store.PaginationParams{
		Cursor: input.Cursor,
		Limit:  limit,
	}
	if input.UpdatedAfter != "" {
		t, err := time.Parse(time.RFC3339, input.UpdatedAfter)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid updated_after format, expected RFC3339")
		}
		params.UpdatedAfter = t
	}

	result, err := s.services.Sync.GetContributorsForSync(ctx, userID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]SyncContributorResponse, len(result.Contributors))
	for i, c := range result.Contributors {
		resp[i] = SyncContributorResponse{
			ID:        c.ID,
			Name:      c.Name,
			SortName:  c.SortName,
			Biography: c.Biography,
			ImageURL:  c.ImageURL,
			Website:   c.Website,
			ASIN:      c.ASIN,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		}
	}

	return &SyncContributorsOutput{
		Body: SyncContributorsResponse{
			NextCursor:            result.NextCursor,
			Contributors:          resp,
			DeletedContributorIDs: result.DeletedContributorIDs,
			HasMore:               result.HasMore,
		},
	}, nil
}

func (s *Server) handleGetSyncSeries(ctx context.Context, input *GetSyncSeriesInput) (*SyncSeriesOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	params := store.PaginationParams{
		Cursor: input.Cursor,
		Limit:  limit,
	}
	if input.UpdatedAfter != "" {
		t, err := time.Parse(time.RFC3339, input.UpdatedAfter)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid updated_after format, expected RFC3339")
		}
		params.UpdatedAfter = t
	}

	result, err := s.services.Sync.GetSeriesForSync(ctx, userID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]SyncSeriesItemResponse, len(result.Series))
	for i, series := range result.Series {
		resp[i] = SyncSeriesItemResponse{
			ID:          series.ID,
			Name:        series.Name,
			Description: series.Description,
			ASIN:        series.ASIN,
			CreatedAt:   series.CreatedAt,
			UpdatedAt:   series.UpdatedAt,
		}
	}

	return &SyncSeriesOutput{
		Body: SyncSeriesResponse{
			NextCursor:       result.NextCursor,
			Series:           resp,
			DeletedSeriesIDs: result.DeletedSeriesIDs,
			HasMore:          result.HasMore,
		},
	}, nil
}
