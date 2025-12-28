package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/search"
)

func (s *Server) registerSearchRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "search",
		Method:      http.MethodGet,
		Path:        "/api/v1/search",
		Summary:     "Search library",
		Description: "Federated search across books, contributors, and series",
		Tags:        []string{"Search"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSearch)
}

// === DTOs ===

// SearchInput contains parameters for searching the library.
type SearchInput struct {
	Authorization string   `header:"Authorization"`
	Query         string   `query:"q" validate:"required,min=1,max=200" doc:"Search query"`
	Types         []string `query:"types" validate:"omitempty,max=3" doc:"Types to search (book, contributor, series). Omit for all."`
	Limit         int      `query:"limit" validate:"omitempty,gte=1,lte=100" doc:"Max results per type (default 10)"`
	GenreSlug     string   `query:"genre" validate:"omitempty,max=100" doc:"Filter by genre slug"`
}

// SearchBookResult contains book search result data.
type SearchBookResult struct {
	ID           string  `json:"id" doc:"Book ID"`
	Title        string  `json:"title" doc:"Book title"`
	AuthorName   string  `json:"author_name,omitempty" doc:"Primary author name"`
	NarratorName string  `json:"narrator_name,omitempty" doc:"Primary narrator name"`
	SeriesName   string  `json:"series_name,omitempty" doc:"Series name"`
	CoverPath    *string `json:"cover_path,omitempty" doc:"Cover image path"`
	Score        float64 `json:"score" doc:"Search relevance score"`
}

// SearchContributorResult contains contributor search result data.
type SearchContributorResult struct {
	ID        string  `json:"id" doc:"Contributor ID"`
	Name      string  `json:"name" doc:"Contributor name"`
	BookCount int     `json:"book_count" doc:"Number of books"`
	Score     float64 `json:"score" doc:"Search relevance score"`
}

// SearchSeriesResult contains series search result data.
type SearchSeriesResult struct {
	ID        string  `json:"id" doc:"Series ID"`
	Name      string  `json:"name" doc:"Series name"`
	BookCount int     `json:"book_count" doc:"Number of books"`
	Score     float64 `json:"score" doc:"Search relevance score"`
}

// SearchResponse contains search results.
type SearchResponse struct {
	Books        []SearchBookResult        `json:"books,omitempty" doc:"Book results"`
	Contributors []SearchContributorResult `json:"contributors,omitempty" doc:"Contributor results"`
	Series       []SearchSeriesResult      `json:"series,omitempty" doc:"Series results"`
	TotalHits    int                       `json:"total_hits" doc:"Total matches across all types"`
}

// SearchOutput wraps the search response for Huma.
type SearchOutput struct {
	Body SearchResponse
}

// === Handlers ===

func (s *Server) handleSearch(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	// Build search params
	params := search.SearchParams{
		Query: input.Query,
		Limit: limit,
	}

	// Parse types - use string values that match DocType constants
	if len(input.Types) > 0 {
		for _, t := range input.Types {
			switch t {
			case "book":
				params.Types = append(params.Types, string(search.DocTypeBook))
			case "contributor":
				params.Types = append(params.Types, string(search.DocTypeContributor))
			case "series":
				params.Types = append(params.Types, string(search.DocTypeSeries))
			}
		}
	}

	// Genre filter
	if input.GenreSlug != "" {
		params.GenreSlugs = []string{input.GenreSlug}
	}

	result, err := s.services.Search.Search(ctx, params)
	if err != nil {
		return nil, err
	}

	resp := SearchResponse{
		TotalHits: min(int(result.Total), int(^uint(0)>>1)), //nolint:gosec // Safe: capped at max int
	}

	// Categorize hits by type
	for i := range result.Hits {
		hit := &result.Hits[i]
		switch hit.Type {
		case search.DocTypeBook:
			resp.Books = append(resp.Books, SearchBookResult{
				ID:           hit.ID,
				Title:        hit.Name,
				AuthorName:   hit.Author,
				NarratorName: hit.Narrator,
				SeriesName:   hit.SeriesName,
				Score:        hit.Score,
			})
		case search.DocTypeContributor:
			resp.Contributors = append(resp.Contributors, SearchContributorResult{
				ID:        hit.ID,
				Name:      hit.Name,
				BookCount: hit.BookCount,
				Score:     hit.Score,
			})
		case search.DocTypeSeries:
			resp.Series = append(resp.Series, SearchSeriesResult{
				ID:        hit.ID,
				Name:      hit.Name,
				BookCount: hit.BookCount,
				Score:     hit.Score,
			})
		}
	}

	return &SearchOutput{Body: resp}, nil
}
