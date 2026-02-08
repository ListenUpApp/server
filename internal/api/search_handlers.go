package api

import (
	"context"
	"net/http"
	"strings"

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
	Authorization string `header:"Authorization"`
	Query         string `query:"q" validate:"required,min=1,max=200" doc:"Search query"`
	Types         string `query:"types" validate:"omitempty,max=100" doc:"Comma-separated types to search (book,contributor,series). Omit for all."`
	Limit         int    `query:"limit" validate:"omitempty,gte=1,lte=100" doc:"Max results per type (default 10)"`
	Offset        int    `query:"offset" validate:"omitempty,gte=0" doc:"Pagination offset (default 0)"`
	GenreSlugs    string `query:"genres" validate:"omitempty,max=200" doc:"Comma-separated genre slugs to filter by"`
	GenrePath     string `query:"genre_path" validate:"omitempty,max=100" doc:"Genre path prefix for hierarchical filtering (e.g. /fiction/fantasy)"`
	Facets        bool   `query:"facets" doc:"Include facets in response"`
}

// SearchHitResult contains a single search result (book, contributor, or series).
type SearchHitResult struct {
	ID         string            `json:"id" doc:"Entity ID"`
	Type       string            `json:"type" doc:"Type: book, contributor, or series"`
	Score      float64           `json:"score" doc:"Search relevance score"`
	Name       string            `json:"name" doc:"Display name (title for books)"`
	Subtitle   string            `json:"subtitle,omitempty" doc:"Subtitle (for books)"`
	Author     string            `json:"author,omitempty" doc:"Author name (for books)"`
	Narrator   string            `json:"narrator,omitempty" doc:"Narrator name (for books)"`
	SeriesName string            `json:"series_name,omitempty" doc:"Series name (for books)"`
	Duration   int64             `json:"duration,omitempty" doc:"Duration in ms (for books)"`
	BookCount  int               `json:"book_count,omitempty" doc:"Number of books (for contributors/series)"`
	GenreSlugs []string          `json:"genre_slugs,omitempty" doc:"Genre slugs (for books)"`
	Tags       []string          `json:"tags,omitempty" doc:"Tag slugs (for books)"`
	Highlights map[string]string `json:"highlights,omitempty" doc:"Highlighted matches"`
}

// SearchFacets contains facet counts for filtering.
type SearchFacets struct {
	Types     []FacetCount `json:"types,omitempty" doc:"Type facets"`
	Genres    []FacetCount `json:"genres,omitempty" doc:"Genre facets"`
	Authors   []FacetCount `json:"authors,omitempty" doc:"Author facets"`
	Narrators []FacetCount `json:"narrators,omitempty" doc:"Narrator facets"`
}

// FacetCount represents a facet value and its count.
type FacetCount struct {
	Value string `json:"value" doc:"Facet value"`
	Count int    `json:"count" doc:"Number of matches"`
}

// SearchResponse contains search results in the format expected by mobile clients.
type SearchResponse struct {
	Query  string            `json:"query" doc:"Original search query"`
	Total  int64             `json:"total" doc:"Total matches"`
	TookMs int64             `json:"took_ms" doc:"Search duration in milliseconds"`
	Hits   []SearchHitResult `json:"hits" doc:"Search results"`
	Facets *SearchFacets     `json:"facets,omitempty" doc:"Facet counts for filtering"`
}

// SearchOutput wraps the search response for Huma.
type SearchOutput struct {
	Body SearchResponse
}

// === Handlers ===

func (s *Server) handleSearch(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	s.logger.Debug("Search request received",
		"query", input.Query,
		"types", input.Types,
		"limit", limit,
	)

	// Build search params
	params := search.SearchParams{
		Query: input.Query,
		Limit: limit,
	}

	// Parse types - comma-separated string to slice
	if input.Types != "" {
		for t := range strings.SplitSeq(input.Types, ",") {
			t = strings.TrimSpace(t)
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

	// Genre filter - parse comma-separated slugs
	if input.GenreSlugs != "" {
		for g := range strings.SplitSeq(input.GenreSlugs, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				params.GenreSlugs = append(params.GenreSlugs, g)
			}
		}
	}

	// Genre path filter for hierarchical genre matching
	if input.GenrePath != "" {
		params.GenrePath = input.GenrePath
	}

	result, err := s.services.Search.Search(ctx, params)
	if err != nil {
		s.logger.Error("Search failed", "error", err, "query", input.Query)
		return nil, err
	}

	s.logger.Debug("Search completed",
		"query", input.Query,
		"total", result.Total,
		"hits", len(result.Hits),
		"took_ms", result.TookMs,
	)

	resp := SearchResponse{
		Query:  input.Query,
		Total:  int64(result.Total), //nolint:gosec // Safe: total count won't exceed int64
		TookMs: result.TookMs,
		Hits:   make([]SearchHitResult, 0, len(result.Hits)),
	}

	// Convert search hits to response format, filtering by ACL
	for i := range result.Hits {
		hit := &result.Hits[i]

		// For book results, verify user has access
		if hit.Type == search.DocTypeBook {
			if canAccess, err := s.store.CanUserAccessBook(ctx, userID, hit.ID); err != nil || !canAccess {
				continue
			}
		}

		respHit := SearchHitResult{
			ID:         hit.ID,
			Type:       string(hit.Type),
			Score:      hit.Score,
			Name:       hit.Name,
			Author:     hit.Author,
			Narrator:   hit.Narrator,
			SeriesName: hit.SeriesName,
			Duration:   hit.Duration,
			BookCount:  hit.BookCount,
			GenreSlugs: hit.GenreSlugs,
			Tags:       hit.Tags,
		}
		resp.Hits = append(resp.Hits, respHit)
	}

	// Update total to reflect filtered count
	resp.Total = int64(len(resp.Hits))

	return &SearchOutput{Body: resp}, nil
}
