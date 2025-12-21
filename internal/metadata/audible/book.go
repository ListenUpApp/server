package audible

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/url"
	"regexp"
	"time"
)

// ASIN format: 10 alphanumeric characters, typically starting with B.
var asinRegex = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// ValidateASIN checks if an ASIN has valid format.
func ValidateASIN(asin string) bool {
	return asinRegex.MatchString(asin)
}

// GetBook retrieves full metadata for a single audiobook by ASIN.
func (c *Client) GetBook(ctx context.Context, region Region, asin string) (*Book, error) {
	if !region.Valid() {
		return nil, wrapError("getBook", region, asin, ErrBadRequest)
	}
	if !ValidateASIN(asin) {
		return nil, wrapError("getBook", region, asin, ErrInvalidASIN)
	}

	query := url.Values{}
	query.Set("response_groups", responseGroups())
	query.Set("image_sizes", imageSizes())

	path := fmt.Sprintf("/1.0/catalog/products/%s", asin)
	body, err := c.doRequest(ctx, region, path, query)
	if err != nil {
		return nil, wrapError("getBook", region, asin, err)
	}

	// Parse response
	var resp struct {
		Product rawProduct `json:"product"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, wrapError("getBook", region, asin, fmt.Errorf("parse response: %w", err))
	}

	return rawProductToBook(&resp.Product), nil
}

// rawProductToBook converts a raw API response to a Book.
func rawProductToBook(p *rawProduct) *Book {
	authors, narrators := parseContributors(append(p.Authors, p.Narrators...))

	var releaseDate time.Time
	if p.ReleaseDate != "" {
		releaseDate, _ = time.Parse("2006-01-02", p.ReleaseDate)
	}

	var rating float32
	var ratingCount int
	if p.Rating != nil {
		rating = p.Rating.OverallDistribution.DisplayAverageRating
		ratingCount = p.Rating.OverallDistribution.NumReviews
	}

	// Extract genres from category ladders
	genres := extractGenres(p.CategoryLadders)

	return &Book{
		ASIN:           p.ASIN,
		Title:          p.Title,
		Subtitle:       p.Subtitle,
		Authors:        authors,
		Narrators:      narrators,
		Publisher:      p.PublisherName,
		ReleaseDate:    releaseDate,
		RuntimeMinutes: p.RuntimeLengthMin,
		Description:    stripHTML(p.MerchandisingSummary),
		CoverURL:       selectCoverURL(p.ProductImages),
		Series:         parseSeries(p.SeriesPrimary),
		Genres:         genres,
		Language:       p.Language,
		Rating:         rating,
		RatingCount:    ratingCount,
	}
}

// extractGenres pulls genre names from category ladders.
func extractGenres(ladders []rawCategoryLadder) []string {
	seen := make(map[string]bool)
	var genres []string

	for _, ladder := range ladders {
		for _, cat := range ladder.Ladder {
			if cat.Name != "" && !seen[cat.Name] {
				seen[cat.Name] = true
				genres = append(genres, cat.Name)
			}
		}
	}

	return genres
}
