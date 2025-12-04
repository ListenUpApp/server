package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/search"
)

// handleSearch performs a federated search across books, contributors, and series.
// Query parameters:
//   - q: search query (required)
//   - types: comma-separated list of types to search (book, contributor, series)
//   - genre: genre slug for filtering
//   - genre_path: genre path for hierarchical filtering (e.g., /fiction/fantasy)
//   - min_duration: minimum duration in seconds
//   - max_duration: maximum duration in seconds
//   - limit: max results (default 20, max 100)
//   - offset: pagination offset
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	// Check if search service is available
	if s.searchService == nil {
		response.Error(w, http.StatusServiceUnavailable, "Search service not available", s.logger)
		return
	}

	// Parse query parameter (required)
	query := r.URL.Query().Get("q")
	if query == "" {
		response.BadRequest(w, "Query parameter 'q' is required", s.logger)
		return
	}

	// Build search params from query string
	params := s.parseSearchParams(r)
	params.Query = query

	// Add timeout to prevent long-running searches
	searchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Execute search
	result, err := s.searchService.Search(searchCtx, params)
	if err != nil {
		if searchCtx.Err() == context.DeadlineExceeded {
			s.logger.Warn("search timed out", "query", query, "user_id", userID)
			response.Error(w, http.StatusGatewayTimeout, "Search timed out", s.logger)
			return
		}
		s.logger.Error("search failed", "query", query, "error", err, "user_id", userID)
		response.InternalError(w, "Search failed", s.logger)
		return
	}

	response.Success(w, result, s.logger)
}

// handleReindex triggers a full reindex of the search index.
// This is an admin operation that rebuilds the search index from scratch.
func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if s.searchService == nil {
		response.Error(w, http.StatusServiceUnavailable, "Search service not available", s.logger)
		return
	}

	// TODO: Add admin check once we have roles
	// For now, any authenticated user can trigger reindex

	s.logger.Info("reindex triggered", "user_id", userID)

	// Run reindex in background
	go func() {
		reindexCtx := context.Background()
		if err := s.searchService.ReindexAll(reindexCtx); err != nil {
			s.logger.Error("reindex failed", "error", err)
		} else {
			count, _ := s.searchService.DocumentCount()
			s.logger.Info("reindex completed", "documents", count)
		}
	}()

	response.Success(w, map[string]string{
		"status":  "started",
		"message": "Reindex started in background",
	}, s.logger)
}

// handleSearchStats returns search index statistics.
func (s *Server) handleSearchStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if s.searchService == nil {
		response.Error(w, http.StatusServiceUnavailable, "Search service not available", s.logger)
		return
	}

	count, err := s.searchService.DocumentCount()
	if err != nil {
		s.logger.Error("failed to get document count", "error", err)
		response.InternalError(w, "Failed to get search statistics", s.logger)
		return
	}

	response.Success(w, map[string]interface{}{
		"document_count": count,
	}, s.logger)
}

// parseSearchParams parses search parameters from the query string.
func (s *Server) parseSearchParams(r *http.Request) search.SearchParams {
	params := search.SearchParams{
		Limit:  20,
		Offset: 0,
	}

	// Parse types
	if types := r.URL.Query().Get("types"); types != "" {
		// Split by comma
		for _, t := range splitAndTrim(types, ",") {
			params.Types = append(params.Types, t)
		}
	}

	// Parse genre slugs
	if genre := r.URL.Query().Get("genre"); genre != "" {
		params.GenreSlugs = splitAndTrim(genre, ",")
	}

	// Parse genre path (hierarchical filtering)
	if genrePath := r.URL.Query().Get("genre_path"); genrePath != "" {
		params.GenrePath = genrePath
	}

	// Parse duration filters
	if minDur := r.URL.Query().Get("min_duration"); minDur != "" {
		if v, err := strconv.ParseInt(minDur, 10, 64); err == nil {
			params.MinDuration = v
		}
	}

	if maxDur := r.URL.Query().Get("max_duration"); maxDur != "" {
		if v, err := strconv.ParseInt(maxDur, 10, 64); err == nil {
			params.MaxDuration = v
		}
	}

	// Parse pagination
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if v, err := strconv.Atoi(limit); err == nil && v > 0 {
			if v > 100 {
				v = 100 // Cap at 100
			}
			params.Limit = v
		}
	}

	if offset := r.URL.Query().Get("offset"); offset != "" {
		if v, err := strconv.Atoi(offset); err == nil && v >= 0 {
			params.Offset = v
		}
	}

	return params
}

// splitAndTrim splits a string and trims whitespace from each part.
func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range split(s, sep) {
		trimmed := trim(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// split is a simple string split helper.
func split(s, sep string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

// trim removes leading and trailing whitespace.
func trim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
