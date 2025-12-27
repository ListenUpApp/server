package audible

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// Ensure json/v2 is available for future use (project convention)
var _ json.Options

// GetContributorProfile fetches a contributor's profile by ASIN by scraping the author page.
// We use web scraping instead of the API because the API no longer returns images,
// but the author page includes the og:image meta tag with the author's photo.
func (c *Client) GetContributorProfile(ctx context.Context, region Region, asin string) (*ContributorProfile, error) {
	// Build author page URL: https://www.audible.com/author/Name/ASIN
	// We use a placeholder name since Audible redirects to the correct URL
	authorURL := fmt.Sprintf("https://%s/author/x/%s", region.WebHost(), asin)

	body, err := c.doWebRequest(ctx, region, authorURL)
	if err != nil {
		return nil, wrapError("getContributorProfile", region, asin, err)
	}

	profile, err := parseContributorProfile(body, asin)
	if err != nil {
		return nil, wrapError("getContributorProfile", region, asin, err)
	}

	return profile, nil
}

// SearchContributors searches for contributors by name.
func (c *Client) SearchContributors(ctx context.Context, region Region, name string) ([]ContributorSearchResult, error) {
	// Build URL: https://www.audible.com/search?searchAuthor={name}
	// Note: We don't use the node parameter as category IDs differ by region
	query := url.Values{}
	query.Set("searchAuthor", name)

	u := url.URL{
		Scheme:   "https",
		Host:     region.WebHost(),
		Path:     "/search",
		RawQuery: query.Encode(),
	}

	body, err := c.doWebRequest(ctx, region, u.String())
	if err != nil {
		return nil, wrapError("searchContributors", region, "", err)
	}

	results, err := parseContributorSearch(body, name)
	if err != nil {
		return nil, wrapError("searchContributors", region, "", err)
	}

	c.logger.Debug("parsed contributor search",
		"results", len(results),
	)

	return results, nil
}

// doWebRequest executes an HTTP request to the Audible website with rate limiting.
// Handles geo-redirects by detecting when Audible redirects to a different regional
// domain and retrying the request with the correct URL.
func (c *Client) doWebRequest(ctx context.Context, region Region, fullURL string) ([]byte, error) {
	// Wait for rate limit (shared with API requests)
	if err := c.limiter.Wait(ctx, string(region)); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	// Parse the original URL to preserve path and query
	originalURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers to appear as a browser
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	// Set locale cookies to ensure we get the correct regional catalog
	// Without these, Audible geo-detects and may serve a different catalog
	localeCookie := region.localeCookie()
	if localeCookie != "" {
		req.Header.Set("Cookie", localeCookie)
	}

	c.logger.Debug("audible web request",
		"region", region,
		"url", fullURL,
	)

	resp, err := c.webHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	c.logger.Debug("audible response",
		"status", resp.StatusCode,
		"location", resp.Header.Get("Location"),
	)

	// Handle redirects: Audible may redirect for two reasons:
	// 1. Geo-redirect: Different regional domain (e.g., audible.com -> audible.ca)
	// 2. Normal redirect: Same host, different path (e.g., /author/x/ASIN -> /author/Name/ASIN)
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		location := resp.Header.Get("Location")
		if location != "" {
			redirectURL, err := url.Parse(location)
			if err == nil {
				// Determine the actual redirect URL
				var newURL url.URL

				if redirectURL.Host == "" {
					// Relative redirect (same host) - just follow it
					newURL = url.URL{
						Scheme:   "https",
						Host:     originalURL.Host,
						Path:     redirectURL.Path,
						RawQuery: redirectURL.RawQuery,
					}
					c.logger.Debug("following redirect",
						"from", fullURL,
						"to", newURL.String(),
					)
				} else if redirectURL.Host != originalURL.Host {
					// Geo-redirect - use new host but preserve original path/query
					newURL = url.URL{
						Scheme:   "https",
						Host:     redirectURL.Host,
						Path:     originalURL.Path,
						RawQuery: originalURL.RawQuery,
					}
					c.logger.Debug("detected geo-redirect, retrying with detected region",
						"from", originalURL.Host,
						"to", redirectURL.Host,
						"newURL", newURL.String(),
					)
				} else {
					// Same host absolute redirect - just follow it
					newURL = *redirectURL
					c.logger.Debug("following redirect",
						"from", fullURL,
						"to", newURL.String(),
					)
				}

				// Need to drain the response body before making new request
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				// Wait for rate limit again for the retry
				if err := c.limiter.Wait(ctx, string(region)); err != nil {
					return nil, fmt.Errorf("rate limit wait (retry): %w", err)
				}

				req2, err := http.NewRequestWithContext(ctx, http.MethodGet, newURL.String(), nil)
				if err != nil {
					return nil, fmt.Errorf("create retry request: %w", err)
				}

				req2.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
				req2.Header.Set("Accept-Language", "en-US,en;q=0.9")
				req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

				resp, err = c.webHTTP.Do(req2)
				if err != nil {
					return nil, fmt.Errorf("execute retry request: %w", err)
				}
				defer resp.Body.Close()
			}
		}
	}

	// Limit response size to 5MB
	const maxSize = 5 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	c.logger.Debug("audible response body",
		"status", resp.StatusCode,
		"bytes", len(body),
	)

	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	default:
		if resp.StatusCode >= 500 {
			return nil, ErrServer
		}
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
}

// parseContributorProfile extracts contributor data from an author page.
func parseContributorProfile(htmlContent []byte, asin string) (*ContributorProfile, error) {
	doc, err := html.Parse(bytes.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	profile := &ContributorProfile{ASIN: asin}

	// Extract name from h1.bc-heading or similar
	profile.Name = findText(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "h1" && hasClass(n, "bc-heading")
	})

	// Extract biography from expander content
	profile.Biography = findText(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && hasClass(n, "bc-expander-content")
	})
	profile.Biography = strings.TrimSpace(profile.Biography)

	// Extract image URL - prefer og:image meta tag, fallback to img element
	// The og:image contains the full-size author photo when available
	profile.ImageURL = findAttr(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "meta" {
			return false
		}
		return getAttr(n, "property") == "og:image"
	}, "content")

	// Filter out generic Audible placeholder images
	if profile.ImageURL != "" && strings.Contains(profile.ImageURL, "Facebook_Placement") {
		profile.ImageURL = ""
	}

	// Fallback: try author-image-outline class (the actual class Audible uses)
	if profile.ImageURL == "" {
		profile.ImageURL = findAttr(doc, func(n *html.Node) bool {
			return n.Type == html.ElementNode && n.Data == "img" && hasClass(n, "author-image-outline")
		}, "src")
	}

	// If no name found, page probably doesn't exist
	if profile.Name == "" {
		return nil, ErrContributorNotFound
	}

	return profile, nil
}

// parseContributorSearch extracts search results from a search page.
// Audible search results show author links within audiobook listings.
// The same author may appear multiple times, so we deduplicate by ASIN.
// Results are ranked by relevance to the search query.
func parseContributorSearch(htmlContent []byte, searchQuery string) ([]ContributorSearchResult, error) {
	doc, err := html.Parse(bytes.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	// Track authors with occurrence count (more occurrences = more relevant)
	type authorData struct {
		result     ContributorSearchResult
		occurrences int
	}
	resultMap := make(map[string]*authorData)
	asinRegex := regexp.MustCompile(`/author/[^/]+/([A-Z0-9]+)`)

	// Find all author links in search results
	findAll(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return false
		}

		href := getAttr(n, "href")
		if !strings.Contains(href, "/author/") {
			return false
		}

		matches := asinRegex.FindStringSubmatch(href)
		if matches == nil {
			return false
		}

		asin := matches[1]

		// If we've seen this author, increment occurrence count
		if existing, exists := resultMap[asin]; exists {
			existing.occurrences++
			return false
		}

		// Extract name - try h3 first (structured results), fallback to plain text
		name := findText(n, func(child *html.Node) bool {
			return child.Type == html.ElementNode && child.Data == "h3"
		})
		if name == "" {
			name = extractTextContent(n)
		}

		if name == "" {
			return false
		}

		result := ContributorSearchResult{
			ASIN: asin,
			Name: name,
		}

		// Extract description if available (e.g., "142 titles")
		result.Description = findText(n, func(child *html.Node) bool {
			return child.Type == html.ElementNode && hasClass(child, "subtitle")
		})

		// Extract image if available
		result.ImageURL = findAttr(n, func(child *html.Node) bool {
			return child.Type == html.ElementNode && child.Data == "img"
		}, "src")

		resultMap[asin] = &authorData{result: result, occurrences: 1}
		return false
	})

	// Convert to slice with relevance scores
	type scoredResult struct {
		result ContributorSearchResult
		score  int
	}
	scored := make([]scoredResult, 0, len(resultMap))

	queryLower := strings.ToLower(strings.TrimSpace(searchQuery))
	queryWords := strings.Fields(queryLower)

	for _, data := range resultMap {
		score := calculateRelevanceScore(data.result.Name, queryLower, queryWords, data.occurrences)
		scored = append(scored, scoredResult{result: data.result, score: score})
	}

	// Sort by relevance score (highest first), then by name for consistency
	slices.SortFunc(scored, func(a, b scoredResult) int {
		if a.score != b.score {
			return b.score - a.score // Higher score first
		}
		return strings.Compare(a.result.Name, b.result.Name)
	})

	// Extract just the results
	results := make([]ContributorSearchResult, len(scored))
	for i, s := range scored {
		results[i] = s.result
	}

	return results, nil
}

// calculateRelevanceScore computes how relevant an author name is to the search query.
// Higher score = more relevant. Considers exact match, partial match, word overlap,
// and occurrence frequency on the page.
func calculateRelevanceScore(name, queryLower string, queryWords []string, occurrences int) int {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	nameWords := strings.Fields(nameLower)

	score := 0

	// Exact match (case-insensitive): highest priority
	if nameLower == queryLower {
		score += 1000
	}

	// Name contains the full query
	if strings.Contains(nameLower, queryLower) {
		score += 500
	}

	// Query contains the full name (e.g., searching "Stephen King Jr" matches "Stephen King")
	if strings.Contains(queryLower, nameLower) {
		score += 400
	}

	// Word-level matching: reward overlapping words
	matchingWords := 0
	for _, qw := range queryWords {
		for _, nw := range nameWords {
			if qw == nw {
				matchingWords++
				break
			}
		}
	}
	if len(queryWords) > 0 {
		// Score based on percentage of query words matched
		score += (matchingWords * 200) / len(queryWords)
	}

	// Bonus for high occurrence count (author appears many times = likely the searched author)
	// Cap at 10 occurrences to prevent overwhelming other signals
	occurrenceBonus := occurrences
	if occurrenceBonus > 10 {
		occurrenceBonus = 10
	}
	score += occurrenceBonus * 5

	return score
}

// HTML parsing helpers

func hasClass(n *html.Node, class string) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			classes := strings.Fields(attr.Val)
			for _, c := range classes {
				if c == class {
					return true
				}
			}
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func findText(n *html.Node, match func(*html.Node) bool) string {
	if match(n) {
		return extractTextContent(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if text := findText(c, match); text != "" {
			return text
		}
	}
	return ""
}

func findAttr(n *html.Node, match func(*html.Node) bool, attr string) string {
	if match(n) {
		return getAttr(n, attr)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if val := findAttr(c, match, attr); val != "" {
			return val
		}
	}
	return ""
}

func findAll(n *html.Node, match func(*html.Node) bool) {
	match(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		findAll(c, match)
	}
}

func extractTextContent(n *html.Node) string {
	var buf strings.Builder
	var extract func(*html.Node)
	extract = func(node *html.Node) {
		if node.Type == html.TextNode {
			buf.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(n)
	return strings.TrimSpace(buf.String())
}
