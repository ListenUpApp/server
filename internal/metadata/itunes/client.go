package itunes

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// Client provides access to the iTunes Search API for audiobook covers.
type Client struct {
	httpClient  *http.Client
	rateLimiter *rate.Limiter
	logger      *slog.Logger
}

// NewClient creates a new iTunes client.
// Rate limited to 20 requests per minute as recommended by Apple.
func NewClient(logger *slog.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		// 20 requests per minute = 1 request per 3 seconds, burst of 5
		rateLimiter: rate.NewLimiter(rate.Every(3*time.Second), 5),
		logger:      logger,
	}
}

// Close releases resources. Currently a no-op but included for interface consistency.
func (c *Client) Close() {
	// No persistent resources to close
}

// wait blocks until rate limiter allows a request.
func (c *Client) wait(ctx context.Context) error {
	return c.rateLimiter.Wait(ctx)
}
