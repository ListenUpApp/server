package search

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
)

// SearchParams configures a search query.
type SearchParams struct {
	Query string   // User's search query
	Types []string // Document types to include (empty = all)

	// Filters
	GenreSlugs  []string // Filter by exact genre slugs
	GenrePath   string   // Filter by genre path prefix (hierarchical)
	MinDuration int64    // Minimum duration in ms (books only)
	MaxDuration int64    // Maximum duration in ms (books only)
	MinYear     int      // Minimum publish year
	MaxYear     int      // Maximum publish year

	// Pagination
	Limit  int
	Offset int

	// Sorting
	SortBy    string // "relevance", "title", "author", "recent", "duration"
	SortOrder string // "asc", "desc"

	// Options
	IncludeFacets bool     // Include facet counts in results
	FacetFields   []string // Which fields to facet on
	Highlight     bool     // Include match highlighting
}

// DefaultSearchParams returns sensible defaults.
func DefaultSearchParams() SearchParams {
	return SearchParams{
		Limit:         20,
		Offset:        0,
		SortBy:        "relevance",
		SortOrder:     "desc",
		IncludeFacets: true,
		FacetFields:   []string{"type", "genre_slugs"},
		Highlight:     true,
	}
}

// SearchResult represents the search results.
type SearchResult struct {
	Query  string       `json:"query"`
	Total  uint64       `json:"total"`
	TookMs int64        `json:"took_ms"`
	Hits   []SearchHit  `json:"hits"`
	Facets SearchFacets `json:"facets,omitempty"`
}

// SearchHit represents a single search result.
type SearchHit struct {
	ID         string            `json:"id"`
	Type       DocType           `json:"type"`
	Score      float64           `json:"score"`
	Name       string            `json:"name"`
	Subtitle   string            `json:"subtitle,omitempty"`
	Author     string            `json:"author,omitempty"`
	Narrator   string            `json:"narrator,omitempty"`
	SeriesName string            `json:"series_name,omitempty"`
	Duration   int64             `json:"duration,omitempty"`
	BookCount  int               `json:"book_count,omitempty"`
	Highlights map[string]string `json:"highlights,omitempty"`
}

// SearchFacets contains facet counts.
type SearchFacets struct {
	Types     []FacetCount `json:"types,omitempty"`
	Genres    []FacetCount `json:"genres,omitempty"`
	Authors   []FacetCount `json:"authors,omitempty"`
	Narrators []FacetCount `json:"narrators,omitempty"`
}

// FacetCount represents a facet value and its count.
type FacetCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Search executes a search query.
func (s *SearchIndex) Search(ctx context.Context, params SearchParams) (*SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build the query
	searchQuery := buildSearchQuery(params)

	// Create search request
	searchRequest := bleve.NewSearchRequestOptions(searchQuery, params.Limit, params.Offset, false)

	// Add sorting
	addSorting(searchRequest, params)

	// Add facets
	if params.IncludeFacets {
		addFacets(searchRequest, params)
	}

	// Add highlighting
	if params.Highlight {
		searchRequest.Highlight = bleve.NewHighlight()
		searchRequest.Highlight.AddField("name")
		searchRequest.Highlight.AddField("author")
		searchRequest.Highlight.AddField("narrator")
		searchRequest.Highlight.AddField("series_name")
	}

	// Request stored fields
	searchRequest.Fields = []string{
		"id", "type", "name", "subtitle", "author", "narrator",
		"series_name", "duration", "book_count",
	}

	// Execute search
	searchResult, err := s.index.SearchInContext(ctx, searchRequest)
	if err != nil {
		return nil, fmt.Errorf("execute search: %w", err)
	}

	// Convert results
	result := &SearchResult{
		Query:  params.Query,
		Total:  searchResult.Total,
		TookMs: searchResult.Took.Milliseconds(),
		Hits:   make([]SearchHit, 0, len(searchResult.Hits)),
	}

	for _, hit := range searchResult.Hits {
		searchHit := SearchHit{
			ID:    hit.ID,
			Score: hit.Score,
		}

		// Extract stored fields
		if t, ok := hit.Fields["type"].(string); ok {
			searchHit.Type = DocType(t)
		}
		if n, ok := hit.Fields["name"].(string); ok {
			searchHit.Name = n
		}
		if st, ok := hit.Fields["subtitle"].(string); ok {
			searchHit.Subtitle = st
		}
		if a, ok := hit.Fields["author"].(string); ok {
			searchHit.Author = a
		}
		if n, ok := hit.Fields["narrator"].(string); ok {
			searchHit.Narrator = n
		}
		if sn, ok := hit.Fields["series_name"].(string); ok {
			searchHit.SeriesName = sn
		}
		if d, ok := hit.Fields["duration"].(float64); ok {
			searchHit.Duration = int64(d)
		}
		if bc, ok := hit.Fields["book_count"].(float64); ok {
			searchHit.BookCount = int(bc)
		}

		// Extract highlights
		if len(hit.Fragments) > 0 {
			searchHit.Highlights = make(map[string]string)
			for field, fragments := range hit.Fragments {
				if len(fragments) > 0 {
					searchHit.Highlights[field] = fragments[0]
				}
			}
		}

		result.Hits = append(result.Hits, searchHit)
	}

	// Extract facets
	if params.IncludeFacets {
		result.Facets = extractFacets(searchResult)
	}

	return result, nil
}

// buildSearchQuery constructs the Bleve query from params.
func buildSearchQuery(params SearchParams) query.Query {
	var queries []query.Query

	// Main text query
	// Search strategy:
	// - Books: match on title (name) and series_name only
	// - Contributors: match on name (their actual name)
	// - Series: match on name
	// We DON'T search author/narrator fields on books because that returns
	// every book by "Peter" when you search for "Peter", when you really
	// want Peter Pan (the book) or Peter Smith (the contributor).
	if params.Query != "" {
		textQueries := []query.Query{}

		// Name/title match with highest boost
		nameMatch := bleve.NewMatchQuery(params.Query)
		nameMatch.SetField("name")
		nameMatch.SetBoost(3.0)
		textQueries = append(textQueries, nameMatch)

		// Series name match (for books in a series)
		seriesMatch := bleve.NewMatchQuery(params.Query)
		seriesMatch.SetField("series_name")
		seriesMatch.SetBoost(1.5)
		textQueries = append(textQueries, seriesMatch)

		// Add fuzzy matching for typo tolerance on name
		fuzzyQuery := bleve.NewFuzzyQuery(params.Query)
		fuzzyQuery.SetFuzziness(1)
		fuzzyQuery.SetField("name")
		fuzzyQuery.SetBoost(0.8)
		textQueries = append(textQueries, fuzzyQuery)

		// Prefix query for autocomplete (minimum 2 chars)
		if len(params.Query) >= 2 {
			prefixQuery := bleve.NewPrefixQuery(strings.ToLower(params.Query))
			prefixQuery.SetField("name")
			prefixQuery.SetBoost(0.5)
			textQueries = append(textQueries, prefixQuery)
		}

		// Combine with OR (match any field)
		queries = append(queries, bleve.NewDisjunctionQuery(textQueries...))
	}

	// Type filter
	if len(params.Types) > 0 {
		typeQueries := make([]query.Query, len(params.Types))
		for i, t := range params.Types {
			tq := bleve.NewTermQuery(t)
			tq.SetField("type")
			typeQueries[i] = tq
		}
		queries = append(queries, bleve.NewDisjunctionQuery(typeQueries...))
	}

	// Genre slug filter (exact match, OR across slugs)
	if len(params.GenreSlugs) > 0 {
		genreQueries := make([]query.Query, len(params.GenreSlugs))
		for i, slug := range params.GenreSlugs {
			gq := bleve.NewTermQuery(slug)
			gq.SetField("genre_slugs")
			genreQueries[i] = gq
		}
		queries = append(queries, bleve.NewDisjunctionQuery(genreQueries...))
	}

	// Genre path filter (hierarchical - prefix match)
	if params.GenrePath != "" {
		prefixQuery := bleve.NewPrefixQuery(params.GenrePath)
		prefixQuery.SetField("genre_paths")
		queries = append(queries, prefixQuery)
	}

	// Duration range filter
	if params.MinDuration > 0 || params.MaxDuration > 0 {
		min := float64(params.MinDuration)
		max := float64(params.MaxDuration)
		if params.MaxDuration == 0 {
			max = math.MaxFloat64
		}
		rangeQuery := bleve.NewNumericRangeQuery(&min, &max)
		rangeQuery.SetField("duration")
		queries = append(queries, rangeQuery)
	}

	// Year range filter
	if params.MinYear > 0 || params.MaxYear > 0 {
		min := float64(params.MinYear)
		max := float64(params.MaxYear)
		if params.MaxYear == 0 {
			max = 3000 // Far future
		}
		rangeQuery := bleve.NewNumericRangeQuery(&min, &max)
		rangeQuery.SetField("publish_year")
		queries = append(queries, rangeQuery)
	}

	// Combine all queries with AND
	if len(queries) == 0 {
		return bleve.NewMatchAllQuery()
	}
	if len(queries) == 1 {
		return queries[0]
	}
	return bleve.NewConjunctionQuery(queries...)
}

// addSorting configures sort order.
func addSorting(req *bleve.SearchRequest, params SearchParams) {
	switch params.SortBy {
	case "title", "name":
		if params.SortOrder == "desc" {
			req.SortBy([]string{"-name"})
		} else {
			req.SortBy([]string{"name"})
		}
	case "author":
		if params.SortOrder == "desc" {
			req.SortBy([]string{"-author", "-name"})
		} else {
			req.SortBy([]string{"author", "name"})
		}
	case "recent":
		if params.SortOrder == "asc" {
			req.SortBy([]string{"created_at"})
		} else {
			req.SortBy([]string{"-created_at"})
		}
	case "duration":
		if params.SortOrder == "asc" {
			req.SortBy([]string{"duration"})
		} else {
			req.SortBy([]string{"-duration"})
		}
	default:
		// Relevance (score) is default - Bleve handles this
		req.SortBy([]string{"-_score"})
	}
}

// addFacets configures facet requests.
func addFacets(req *bleve.SearchRequest, params SearchParams) {
	for _, field := range params.FacetFields {
		facetReq := bleve.NewFacetRequest(field, 20) // Top 20 values
		req.AddFacet(field, facetReq)
	}
}

// extractFacets converts Bleve facets to our format.
func extractFacets(result *bleve.SearchResult) SearchFacets {
	facets := SearchFacets{}

	if typeFacet, ok := result.Facets["type"]; ok {
		for _, term := range typeFacet.Terms.Terms() {
			facets.Types = append(facets.Types, FacetCount{
				Value: term.Term,
				Count: term.Count,
			})
		}
	}

	if genreFacet, ok := result.Facets["genre_slugs"]; ok {
		for _, term := range genreFacet.Terms.Terms() {
			facets.Genres = append(facets.Genres, FacetCount{
				Value: term.Term,
				Count: term.Count,
			})
		}
	}

	if authorFacet, ok := result.Facets["author"]; ok {
		for _, term := range authorFacet.Terms.Terms() {
			facets.Authors = append(facets.Authors, FacetCount{
				Value: term.Term,
				Count: term.Count,
			})
		}
	}

	if narratorFacet, ok := result.Facets["narrator"]; ok {
		for _, term := range narratorFacet.Terms.Terms() {
			facets.Narrators = append(facets.Narrators, FacetCount{
				Value: term.Term,
				Count: term.Count,
			})
		}
	}

	return facets
}
