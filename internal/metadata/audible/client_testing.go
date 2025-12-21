package audible

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Test helper methods for unit testing with mock HTTP servers.
// These methods allow bypassing the region-based host lookup.

// searchWithHost performs a search using a custom base URL (for testing).
func (c *Client) searchWithHost(ctx context.Context, baseURL string, params SearchParams) ([]SearchResult, error) {
	query := url.Values{}
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

	limit := params.Limit
	if limit <= 0 {
		limit = defaultNumResults
	}
	if limit > maxNumResults {
		limit = maxNumResults
	}
	query.Set("num_results", strconv.Itoa(limit))
	query.Set("response_groups", responseGroups())
	query.Set("image_sizes", imageSizes())
	query.Set("products_sort_by", "Relevance")

	body, err := c.doRequestWithURL(ctx, baseURL+"/1.0/catalog/products?"+query.Encode())
	if err != nil {
		return nil, wrapError("search", RegionUS, "", err)
	}

	var resp struct {
		Products []rawProduct `json:"products"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, wrapError("search", RegionUS, "", fmt.Errorf("parse response: %w", err))
	}

	results := make([]SearchResult, 0, len(resp.Products))
	for i := range resp.Products {
		p := &resp.Products[i]
		authors, narrators := parseContributors(append(p.Authors, p.Narrators...))
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

// getBookWithHost retrieves a book using a custom base URL (for testing).
func (c *Client) getBookWithHost(ctx context.Context, baseURL, asin string) (*Book, error) {
	if !ValidateASIN(asin) {
		return nil, wrapError("getBook", RegionUS, asin, ErrInvalidASIN)
	}

	query := url.Values{}
	query.Set("response_groups", responseGroups())
	query.Set("image_sizes", imageSizes())

	body, err := c.doRequestWithURL(ctx, baseURL+"/1.0/catalog/products/"+asin+"?"+query.Encode())
	if err != nil {
		return nil, wrapError("getBook", RegionUS, asin, err)
	}

	var resp struct {
		Product rawProduct `json:"product"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, wrapError("getBook", RegionUS, asin, fmt.Errorf("parse response: %w", err))
	}

	return rawProductToBook(&resp.Product), nil
}

// getChaptersWithHost retrieves chapters using a custom base URL (for testing).
func (c *Client) getChaptersWithHost(ctx context.Context, baseURL, asin string) ([]Chapter, error) {
	if !ValidateASIN(asin) {
		return nil, wrapError("getChapters", RegionUS, asin, ErrInvalidASIN)
	}

	query := url.Values{}
	query.Set("response_groups", "chapter_info")

	body, err := c.doRequestWithURL(ctx, baseURL+"/1.0/content/"+asin+"/metadata?"+query.Encode())
	if err != nil {
		return nil, wrapError("getChapters", RegionUS, asin, err)
	}

	var resp struct {
		ContentMetadata struct {
			ChapterInfo rawChapterInfo `json:"chapter_info"`
		} `json:"content_metadata"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, wrapError("getChapters", RegionUS, asin, fmt.Errorf("parse response: %w", err))
	}

	chapters := make([]Chapter, 0, len(resp.ContentMetadata.ChapterInfo.Chapters))
	for _, ch := range resp.ContentMetadata.ChapterInfo.Chapters {
		chapters = append(chapters, Chapter{
			Title:      ch.Title,
			StartMs:    ch.StartOffsetMs,
			DurationMs: ch.LengthMs,
		})
	}

	return chapters, nil
}
