package audible

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/listenupapp/listenup-server/internal/ratelimit"
)

const (
	// Rate limit: 1 request per second per region, burst of 3
	defaultRPS   = 1.0
	defaultBurst = 3

	// HTTP client settings
	defaultTimeout = 30 * time.Second

	// API settings
	defaultNumResults = 25
	maxNumResults     = 50
)

// Client is a rate-limited Audible API client.
type Client struct {
	http    *http.Client
	limiter *ratelimit.KeyedRateLimiter
	logger  *slog.Logger
}

// New creates a new Audible client.
func New(logger *slog.Logger) *Client {
	return &Client{
		http: &http.Client{
			Timeout: defaultTimeout,
		},
		limiter: ratelimit.New(defaultRPS, defaultBurst),
		logger:  logger,
	}
}

// Close releases resources held by the client.
func (c *Client) Close() {
	c.limiter.Stop()
}

// doRequest executes an HTTP request with rate limiting.
func (c *Client) doRequest(ctx context.Context, region Region, path string, query url.Values) ([]byte, error) {
	// Wait for rate limit
	if err := c.limiter.Wait(ctx, string(region)); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	// Build URL
	u := url.URL{
		Scheme:   "https",
		Host:     region.Host(),
		Path:     path,
		RawQuery: query.Encode(),
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ListenUp/1.0")

	// Execute
	c.logger.Debug("audible request",
		"region", region,
		"path", path,
	)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check status
	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case http.StatusBadRequest:
		return nil, ErrBadRequest
	default:
		if resp.StatusCode >= 500 {
			return nil, ErrServer
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
}

// responseGroups returns the standard response_groups parameter value.
func responseGroups() string {
	return "contributors,product_desc,product_attrs,product_extended_attrs,media,rating,series,category_ladders"
}

// imageSizes returns the standard image_sizes parameter value.
func imageSizes() string {
	return "500,1024"
}

// parseContributors extracts authors and narrators from the API response.
func parseContributors(raw []rawContributor) (authors, narrators []Contributor) {
	for _, c := range raw {
		contrib := Contributor{
			ASIN: c.ASIN,
			Name: c.Name,
			Role: c.Role,
		}
		switch c.Role {
		case "author":
			authors = append(authors, contrib)
		case "narrator":
			narrators = append(narrators, contrib)
		default:
			// Include other roles as authors for now
			if c.Role != "" {
				contrib.Role = c.Role
			}
			authors = append(authors, contrib)
		}
	}
	return
}

// parseSeries extracts series information from the API response.
func parseSeries(raw []rawSeries) []SeriesEntry {
	var series []SeriesEntry
	for _, s := range raw {
		series = append(series, SeriesEntry{
			ASIN:     s.ASIN,
			Name:     s.Title,
			Position: s.Sequence,
		})
	}
	return series
}

// selectCoverURL picks the best available cover URL (prefer 1024px).
func selectCoverURL(images map[string]string) string {
	// Prefer larger images
	for _, size := range []string{"1024", "500", "image_url"} {
		if url, ok := images[size]; ok && url != "" {
			return url
		}
	}
	return ""
}

// Raw API response types (internal)

type rawProduct struct {
	ASIN                 string              `json:"asin"`
	Title                string              `json:"title"`
	Subtitle             string              `json:"subtitle"`
	PublisherName        string              `json:"publisher_name"`
	ReleaseDate          string              `json:"release_date"`
	RuntimeLengthMin     int                 `json:"runtime_length_min"`
	MerchandisingSummary string              `json:"merchandising_summary"`
	ProductImages        map[string]string   `json:"product_images"`
	Authors              []rawContributor    `json:"authors"`
	Narrators            []rawContributor    `json:"narrators"`
	SeriesPrimary        []rawSeries         `json:"series"`
	CategoryLadders      []rawCategoryLadder `json:"category_ladders"`
	Language             string              `json:"language"`
	Rating               *rawRating          `json:"rating"`
}

type rawContributor struct {
	ASIN string `json:"asin"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type rawSeries struct {
	ASIN     string `json:"asin"`
	Title    string `json:"title"`
	Sequence string `json:"sequence"`
}

type rawCategoryLadder struct {
	Ladder []rawCategory `json:"ladder"`
}

type rawCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type rawRating struct {
	OverallDistribution struct {
		DisplayAverageRating float32 `json:"display_average_rating"`
		NumReviews           int     `json:"num_reviews"`
	} `json:"overall_distribution"`
}

type rawChapterInfo struct {
	Chapters []rawChapter `json:"chapters"`
}

type rawChapter struct {
	Title          string `json:"title"`
	StartOffsetMs  int64  `json:"start_offset_ms"`
	StartOffsetSec int64  `json:"start_offset_sec"`
	LengthMs       int64  `json:"length_ms"`
}

// doRequestWithURL executes an HTTP request without rate limiting (for testing).
func (c *Client) doRequestWithURL(ctx context.Context, fullURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ListenUp/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case http.StatusBadRequest:
		return nil, ErrBadRequest
	default:
		if resp.StatusCode >= 500 {
			return nil, ErrServer
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
}
