package audible

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/url"
)

// GetChapters retrieves chapter information for an audiobook.
func (c *Client) GetChapters(ctx context.Context, region Region, asin string) ([]Chapter, error) {
	if !region.Valid() {
		return nil, wrapError("getChapters", region, asin, ErrBadRequest)
	}
	if !ValidateASIN(asin) {
		return nil, wrapError("getChapters", region, asin, ErrInvalidASIN)
	}

	query := url.Values{}
	query.Set("response_groups", "chapter_info")

	path := fmt.Sprintf("/1.0/content/%s/metadata", asin)
	body, err := c.doRequest(ctx, region, path, query)
	if err != nil {
		return nil, wrapError("getChapters", region, asin, err)
	}

	// Parse response
	var resp struct {
		ContentMetadata struct {
			ChapterInfo rawChapterInfo `json:"chapter_info"`
		} `json:"content_metadata"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, wrapError("getChapters", region, asin, fmt.Errorf("parse response: %w", err))
	}

	// Convert to Chapter
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
