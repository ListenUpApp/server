package audible

import (
	"context"
	"encoding/json/v2"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseContributorProfileAPI(t *testing.T) {
	// Test parsing the API response format
	jsonResponse := []byte(`{
		"contributor": {
			"bio": "I'm Brandon Sanderson, a fantasy author.",
			"contributor_id": "B001IGFHW6",
			"name": "Brandon Sanderson"
		},
		"response_groups": ["always-returned"]
	}`)

	var resp struct {
		Contributor struct {
			ContributorID string `json:"contributor_id"`
			Name          string `json:"name"`
			Bio           string `json:"bio"`
		} `json:"contributor"`
	}
	err := json.Unmarshal(jsonResponse, &resp)
	require.NoError(t, err)

	assert.Equal(t, "B001IGFHW6", resp.Contributor.ContributorID)
	assert.Equal(t, "Brandon Sanderson", resp.Contributor.Name)
	assert.Contains(t, resp.Contributor.Bio, "I'm Brandon Sanderson")
}

func TestParseContributorSearch(t *testing.T) {
	html, err := os.ReadFile("testdata/contributor_search.html")
	require.NoError(t, err)

	results, err := parseContributorSearch(html, "Brandon Sanderson")
	require.NoError(t, err)

	require.Len(t, results, 2)

	// Results are sorted by relevance - exact match "Brandon Sanderson" should be first
	assert.Equal(t, "B001IGFHW6", results[0].ASIN)
	assert.Equal(t, "Brandon Sanderson", results[0].Name)
	assert.Equal(t, "142 titles", results[0].Description)

	assert.Equal(t, "B001JP7WUO", results[1].ASIN)
	assert.Equal(t, "Brandon Mull", results[1].Name)
}

func TestParseContributorSearch_PlainTextLinks(t *testing.T) {
	// Test parsing author links found in audiobook listings (plain text, no h3)
	html := []byte(`<!DOCTYPE html>
<html>
<head><title>Search Results</title></head>
<body>
<div class="productListItem">
  <span>Written by:
    <a href="/author/Timothy-Ferriss/B001ILKBW2?ref=abc">Timothy Ferriss</a>
  </span>
</div>
<div class="productListItem">
  <span>Written by:
    <a href="/author/Tim-Ferriss/B001ILKBW2?ref=xyz">Tim Ferriss</a>
  </span>
</div>
<div class="productListItem">
  <span>Written by:
    <a href="/author/James-Clear/B07D23CFGR?ref=def">James Clear</a>
  </span>
</div>
</body>
</html>`)

	results, err := parseContributorSearch(html, "Timothy Ferriss")
	require.NoError(t, err)

	// Should have 2 results (Timothy Ferriss deduplicated, James Clear)
	require.Len(t, results, 2)

	// Results sorted by relevance - Timothy Ferriss matches search query exactly
	// and has 2 occurrences (both links refer to same ASIN)
	assert.Equal(t, "B001ILKBW2", results[0].ASIN)
	assert.Equal(t, "Timothy Ferriss", results[0].Name)

	assert.Equal(t, "B07D23CFGR", results[1].ASIN)
	assert.Equal(t, "James Clear", results[1].Name)
}

func TestParseContributorProfileAPI_EmptyName(t *testing.T) {
	// API returns empty name when contributor not found
	jsonResponse := []byte(`{
		"contributor": {
			"bio": "",
			"contributor_id": "",
			"name": ""
		}
	}`)

	var resp struct {
		Contributor struct {
			Name string `json:"name"`
		} `json:"contributor"`
	}
	err := json.Unmarshal(jsonResponse, &resp)
	require.NoError(t, err)

	// Empty name indicates not found
	assert.Empty(t, resp.Contributor.Name)
}

func TestSearchContributorsLive(t *testing.T) {
	if os.Getenv("LIVE_TEST") == "" {
		t.Skip("skipping live test (set LIVE_TEST=1 to run)")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := New(logger)
	defer client.Close()

	results, err := client.SearchContributors(context.Background(), RegionUS, "Stephen King")
	if err != nil {
		t.Fatalf("SearchContributors failed: %v", err)
	}

	t.Logf("Found %d results", len(results))
	for i, r := range results {
		t.Logf("%d. %s (ASIN: %s, Image: %s)", i+1, r.Name, r.ASIN, r.ImageURL)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result for Stephen King")
	}
}

func TestGetContributorProfileLive(t *testing.T) {
	if os.Getenv("LIVE_TEST") == "" {
		t.Skip("skipping live test (set LIVE_TEST=1 to run)")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := New(logger)
	defer client.Close()

	// Stephen King's ASIN
	profile, err := client.GetContributorProfile(context.Background(), RegionUS, "B000AQ0842")
	if err != nil {
		t.Fatalf("GetContributorProfile failed: %v", err)
	}

	t.Logf("Name: %s", profile.Name)
	t.Logf("Biography: %.100s...", profile.Biography)
	t.Logf("ImageURL: %s", profile.ImageURL)
}

func TestWebHost(t *testing.T) {
	tests := []struct {
		region Region
		want   string
	}{
		{RegionUS, "www.audible.com"},
		{RegionUK, "www.audible.co.uk"},
		{RegionDE, "www.audible.de"},
		{RegionAU, "www.audible.com.au"},
		{RegionJP, "www.audible.co.jp"},
		{Region("invalid"), "www.audible.com"}, // Default to US
	}

	for _, tt := range tests {
		t.Run(string(tt.region), func(t *testing.T) {
			got := tt.region.WebHost()
			assert.Equal(t, tt.want, got)
		})
	}
}
