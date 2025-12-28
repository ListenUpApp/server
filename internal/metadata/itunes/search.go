package itunes

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	searchBaseURL = "https://itunes.apple.com/search"
	defaultLimit  = 10
)

// SearchAudiobooks searches iTunes for audiobooks matching the query.
// Returns results with high-resolution cover URLs and probed dimensions.
func (c *Client) SearchAudiobooks(ctx context.Context, query string) ([]AudiobookResult, error) {
	if err := c.wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	// Build search URL
	params := url.Values{}
	params.Set("term", query)
	params.Set("media", "audiobook")
	params.Set("entity", "audiobook")
	params.Set("limit", fmt.Sprintf("%d", defaultLimit))

	searchURL := searchBaseURL + "?" + params.Encode()

	c.logger.Debug("searching iTunes",
		"query", query,
		"url", searchURL,
	)

	// Make request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", resp.StatusCode)
	}

	// Parse response
	var searchResp searchResponse
	if err := json.UnmarshalRead(resp.Body, &searchResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	c.logger.Debug("iTunes search results",
		"query", query,
		"count", searchResp.ResultCount,
	)

	// Convert to AudiobookResult with high-res cover URLs
	results := make([]AudiobookResult, 0, len(searchResp.Results))
	for i := range searchResp.Results {
		r := &searchResp.Results[i]
		// Only include audiobooks
		if r.WrapperType != "audiobook" && r.CollectionType != "Audiobook" {
			continue
		}

		// Get the best artwork URL we have and transform to max size
		artworkURL := r.ArtworkURL100
		if artworkURL == "" {
			artworkURL = r.ArtworkURL60
		}
		coverURL := MaxCoverURL(artworkURL)

		result := AudiobookResult{
			ID:       r.CollectionID,
			Title:    r.CollectionName,
			Artist:   r.ArtistName,
			CoverURL: coverURL,
		}

		results = append(results, result)
	}

	return results, nil
}

// SearchAudiobooksWithDimensions searches and probes dimensions for each result.
// This makes additional HTTP requests to get actual image sizes.
func (c *Client) SearchAudiobooksWithDimensions(ctx context.Context, query string) ([]AudiobookResult, error) {
	results, err := c.SearchAudiobooks(ctx, query)
	if err != nil {
		return nil, err
	}

	// Probe dimensions for each result
	for i := range results {
		if results[i].CoverURL == "" {
			continue
		}

		width, height, err := GetImageDimensions(ctx, c.httpClient, results[i].CoverURL)
		if err != nil {
			c.logger.Warn("failed to probe cover dimensions, using fallback",
				"url", results[i].CoverURL,
				"error", err,
			)
			// Fallback: iTunes typically returns 2400x2400 for high-res covers
			// Better to show an estimate than 0x0
			results[i].CoverWidth = 2400
			results[i].CoverHeight = 2400
			continue
		}

		results[i].CoverWidth = width
		results[i].CoverHeight = height
	}

	return results, nil
}

// SearchByTitleAndAuthor searches using both title and author for better matching.
func (c *Client) SearchByTitleAndAuthor(ctx context.Context, title, author string) ([]AudiobookResult, error) {
	// Combine title and author for search
	query := strings.TrimSpace(title)
	if author != "" {
		query = query + " " + strings.TrimSpace(author)
	}

	return c.SearchAudiobooksWithDimensions(ctx, query)
}
