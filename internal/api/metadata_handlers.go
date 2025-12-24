package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
)

func (s *Server) registerMetadataRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "searchMetadata",
		Method:      http.MethodGet,
		Path:        "/api/v1/metadata/search",
		Summary:     "Search metadata",
		Description: "Search the Audible catalog for book metadata",
		Tags:        []string{"Metadata"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSearchMetadata)

	huma.Register(s.api, huma.Operation{
		OperationID: "getMetadataBook",
		Method:      http.MethodGet,
		Path:        "/api/v1/metadata/books/{asin}",
		Summary:     "Get book metadata",
		Description: "Fetches book metadata from Audible by ASIN",
		Tags:        []string{"Metadata"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetMetadataBook)

	huma.Register(s.api, huma.Operation{
		OperationID: "getMetadataChapters",
		Method:      http.MethodGet,
		Path:        "/api/v1/metadata/books/{asin}/chapters",
		Summary:     "Get chapter metadata",
		Description: "Fetches chapter information from Audible by ASIN",
		Tags:        []string{"Metadata"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetMetadataChapters)

	huma.Register(s.api, huma.Operation{
		OperationID: "refreshMetadataBook",
		Method:      http.MethodPost,
		Path:        "/api/v1/metadata/books/{asin}/refresh",
		Summary:     "Refresh book metadata",
		Description: "Forces a fresh fetch of book metadata, bypassing cache",
		Tags:        []string{"Metadata"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRefreshMetadataBook)
}

// === DTOs ===

type SearchMetadataInput struct {
	Authorization string  `header:"Authorization"`
	Query         string  `query:"q" validate:"required,min=1" doc:"Search query"`
	Region        *string `query:"region" doc:"Audible region (us, uk, de, fr, etc.)"`
}

type MetadataContributorResponse struct {
	ASIN string `json:"asin,omitempty" doc:"Contributor ASIN"`
	Name string `json:"name" doc:"Contributor name"`
	Role string `json:"role" doc:"Contributor role"`
}

type MetadataSearchResultResponse struct {
	ASIN           string                        `json:"asin" doc:"Audible ASIN"`
	Title          string                        `json:"title" doc:"Book title"`
	Subtitle       string                        `json:"subtitle,omitempty" doc:"Book subtitle"`
	Authors        []MetadataContributorResponse `json:"authors" doc:"Authors"`
	Narrators      []MetadataContributorResponse `json:"narrators" doc:"Narrators"`
	RuntimeMinutes int                           `json:"runtime_minutes" doc:"Duration in minutes"`
	ReleaseDate    time.Time                     `json:"release_date,omitempty" doc:"Release date"`
	CoverURL       string                        `json:"cover_url,omitempty" doc:"Cover image URL"`
}

type SearchMetadataResponse struct {
	Results []MetadataSearchResultResponse `json:"results" doc:"Search results"`
	Region  string                         `json:"region" doc:"Region that returned results"`
}

type SearchMetadataOutput struct {
	Body SearchMetadataResponse
}

type GetMetadataBookInput struct {
	Authorization string  `header:"Authorization"`
	ASIN          string  `path:"asin" doc:"Audible ASIN"`
	Region        *string `query:"region" doc:"Audible region"`
}

type MetadataSeriesEntryResponse struct {
	ASIN     string `json:"asin,omitempty" doc:"Series ASIN"`
	Name     string `json:"name" doc:"Series name"`
	Position string `json:"position,omitempty" doc:"Position in series"`
}

type MetadataBookResponse struct {
	ASIN           string                        `json:"asin" doc:"Audible ASIN"`
	Title          string                        `json:"title" doc:"Book title"`
	Subtitle       string                        `json:"subtitle,omitempty" doc:"Book subtitle"`
	Authors        []MetadataContributorResponse `json:"authors" doc:"Authors"`
	Narrators      []MetadataContributorResponse `json:"narrators" doc:"Narrators"`
	Publisher      string                        `json:"publisher,omitempty" doc:"Publisher name"`
	ReleaseDate    time.Time                     `json:"release_date,omitempty" doc:"Release date"`
	RuntimeMinutes int                           `json:"runtime_minutes" doc:"Duration in minutes"`
	Description    string                        `json:"description,omitempty" doc:"Book description"`
	CoverURL       string                        `json:"cover_url,omitempty" doc:"Cover image URL"`
	Series         []MetadataSeriesEntryResponse `json:"series,omitempty" doc:"Series entries"`
	Genres         []string                      `json:"genres,omitempty" doc:"Genre names"`
	Language       string                        `json:"language,omitempty" doc:"Language"`
	Rating         float32                       `json:"rating,omitempty" doc:"Average rating"`
	RatingCount    int                           `json:"rating_count,omitempty" doc:"Number of ratings"`
}

type MetadataBookOutput struct {
	Body MetadataBookResponse
}

type GetMetadataChaptersInput struct {
	Authorization string  `header:"Authorization"`
	ASIN          string  `path:"asin" doc:"Audible ASIN"`
	Region        *string `query:"region" doc:"Audible region"`
}

type MetadataChapterResponse struct {
	Title      string `json:"title" doc:"Chapter title"`
	StartMs    int64  `json:"start_ms" doc:"Start offset in milliseconds"`
	DurationMs int64  `json:"duration_ms" doc:"Duration in milliseconds"`
}

type MetadataChaptersResponse struct {
	Chapters []MetadataChapterResponse `json:"chapters" doc:"Chapter list"`
}

type MetadataChaptersOutput struct {
	Body MetadataChaptersResponse
}

type RefreshMetadataBookInput struct {
	Authorization string `header:"Authorization"`
	ASIN          string `path:"asin" doc:"Audible ASIN"`
	Region        string `query:"region" validate:"required" doc:"Audible region"`
}

// === Handlers ===

func (s *Server) handleSearchMetadata(ctx context.Context, input *SearchMetadataInput) (*SearchMetadataOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	params := audible.SearchParams{Keywords: input.Query}

	results, region, err := s.services.Metadata.SearchWithFallback(ctx, params)
	if err != nil {
		return nil, err
	}

	resp := make([]MetadataSearchResultResponse, len(results))
	for i, r := range results {
		resp[i] = mapSearchResult(r)
	}

	return &SearchMetadataOutput{
		Body: SearchMetadataResponse{
			Results: resp,
			Region:  string(region),
		},
	}, nil
}

func (s *Server) handleGetMetadataBook(ctx context.Context, input *GetMetadataBookInput) (*MetadataBookOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	var region *audible.Region
	if input.Region != nil {
		r := audible.Region(*input.Region)
		region = &r
	}

	book, err := s.services.Metadata.GetBook(ctx, region, input.ASIN)
	if err != nil {
		return nil, err
	}

	return &MetadataBookOutput{Body: mapMetadataBook(book)}, nil
}

func (s *Server) handleGetMetadataChapters(ctx context.Context, input *GetMetadataChaptersInput) (*MetadataChaptersOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	var region *audible.Region
	if input.Region != nil {
		r := audible.Region(*input.Region)
		region = &r
	}

	chapters, err := s.services.Metadata.GetChapters(ctx, region, input.ASIN)
	if err != nil {
		return nil, err
	}

	resp := make([]MetadataChapterResponse, len(chapters))
	for i, ch := range chapters {
		resp[i] = MetadataChapterResponse{
			Title:      ch.Title,
			StartMs:    ch.StartMs,
			DurationMs: ch.DurationMs,
		}
	}

	return &MetadataChaptersOutput{Body: MetadataChaptersResponse{Chapters: resp}}, nil
}

func (s *Server) handleRefreshMetadataBook(ctx context.Context, input *RefreshMetadataBookInput) (*MetadataBookOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	region := audible.Region(input.Region)
	book, err := s.services.Metadata.RefreshBook(ctx, region, input.ASIN)
	if err != nil {
		return nil, err
	}

	return &MetadataBookOutput{Body: mapMetadataBook(book)}, nil
}

// === Mappers ===

func mapContributors(contributors []audible.Contributor) []MetadataContributorResponse {
	result := make([]MetadataContributorResponse, len(contributors))
	for i, c := range contributors {
		result[i] = MetadataContributorResponse{
			ASIN: c.ASIN,
			Name: c.Name,
			Role: c.Role,
		}
	}
	return result
}

func mapSearchResult(r audible.SearchResult) MetadataSearchResultResponse {
	return MetadataSearchResultResponse{
		ASIN:           r.ASIN,
		Title:          r.Title,
		Subtitle:       r.Subtitle,
		Authors:        mapContributors(r.Authors),
		Narrators:      mapContributors(r.Narrators),
		RuntimeMinutes: r.RuntimeMinutes,
		ReleaseDate:    r.ReleaseDate,
		CoverURL:       r.CoverURL,
	}
}

func mapMetadataBook(b *audible.Book) MetadataBookResponse {
	series := make([]MetadataSeriesEntryResponse, len(b.Series))
	for i, s := range b.Series {
		series[i] = MetadataSeriesEntryResponse{
			ASIN:     s.ASIN,
			Name:     s.Name,
			Position: s.Position,
		}
	}

	return MetadataBookResponse{
		ASIN:           b.ASIN,
		Title:          b.Title,
		Subtitle:       b.Subtitle,
		Authors:        mapContributors(b.Authors),
		Narrators:      mapContributors(b.Narrators),
		Publisher:      b.Publisher,
		ReleaseDate:    b.ReleaseDate,
		RuntimeMinutes: b.RuntimeMinutes,
		Description:    b.Description,
		CoverURL:       b.CoverURL,
		Series:         series,
		Genres:         b.Genres,
		Language:       b.Language,
		Rating:         b.Rating,
		RatingCount:    b.RatingCount,
	}
}
