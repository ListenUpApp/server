package audible

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Search searches the Audible catalog.
func (c *Client) Search(ctx context.Context, region Region, params SearchParams) ([]SearchResult, error) {
	if !region.Valid() {
		return nil, wrapError("search", region, "", ErrBadRequest)
	}

	query := url.Values{}

	// Build search query
	if params.Keywords != "" {
		query.Set("keywords", params.Keywords)
	}
	if params.Title != "" {
		query.Set("title", params.Title)
	}
	if params.Author != "" {
		query.Set("author", params.Author)
	}
	if params.Narrator != "" {
		query.Set("narrator", params.Narrator)
	}

	// Set result limit
	limit := params.Limit
	if limit <= 0 {
		limit = defaultNumResults
	}
	if limit > maxNumResults {
		limit = maxNumResults
	}
	query.Set("num_results", strconv.Itoa(limit))

	// Standard params
	query.Set("response_groups", responseGroups())
	query.Set("image_sizes", imageSizes())
	query.Set("products_sort_by", "Relevance")

	// Execute request
	body, err := c.doRequest(ctx, region, "/1.0/catalog/products", query)
	if err != nil {
		return nil, wrapError("search", region, "", err)
	}

	// Parse response
	var resp struct {
		Products []rawProduct `json:"products"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, wrapError("search", region, "", fmt.Errorf("parse response: %w", err))
	}

	// Convert to SearchResult.
	results := make([]SearchResult, 0, len(resp.Products))
	for i := range resp.Products {
		p := &resp.Products[i]
		authors, narrators := separateContributorsByRole(p.Authors, p.Narrators)

		var releaseDate time.Time
		if p.ReleaseDate != "" {
			releaseDate, _ = time.Parse("2006-01-02", p.ReleaseDate)
		}

		results = append(results, SearchResult{
			ASIN:           p.ASIN,
			Title:          p.Title,
			Subtitle:       p.Subtitle,
			Authors:        authors,
			Narrators:      narrators,
			CoverURL:       selectCoverURL(p.ProductImages),
			RuntimeMinutes: p.RuntimeLengthMin,
			ReleaseDate:    releaseDate,
		})
	}

	return results, nil
}
