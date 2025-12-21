// Package audible provides a client for the Audible catalog API.
package audible

import "time"

// Region represents an Audible marketplace.
type Region string

const (
	RegionUS Region = "us"
	RegionUK Region = "uk"
	RegionDE Region = "de"
	RegionFR Region = "fr"
	RegionAU Region = "au"
	RegionCA Region = "ca"
	RegionJP Region = "jp"
	RegionIT Region = "it"
	RegionIN Region = "in"
	RegionES Region = "es"
)

// AllRegions returns all supported Audible regions.
func AllRegions() []Region {
	return []Region{
		RegionUS, RegionUK, RegionDE, RegionFR, RegionAU,
		RegionCA, RegionJP, RegionIT, RegionIN, RegionES,
	}
}

// Host returns the API host for this region.
func (r Region) Host() string {
	hosts := map[Region]string{
		RegionUS: "api.audible.com",
		RegionUK: "api.audible.co.uk",
		RegionDE: "api.audible.de",
		RegionFR: "api.audible.fr",
		RegionAU: "api.audible.com.au",
		RegionCA: "api.audible.ca",
		RegionJP: "api.audible.co.jp",
		RegionIT: "api.audible.it",
		RegionIN: "api.audible.in",
		RegionES: "api.audible.es",
	}
	if host, ok := hosts[r]; ok {
		return host
	}
	return hosts[RegionUS] // Default to US
}

// Valid returns true if this is a recognized region.
func (r Region) Valid() bool {
	switch r {
	case RegionUS, RegionUK, RegionDE, RegionFR, RegionAU,
		RegionCA, RegionJP, RegionIT, RegionIN, RegionES:
		return true
	}
	return false
}

// Contributor represents an author or narrator.
type Contributor struct {
	ASIN string `json:"asin,omitempty"`
	Name string `json:"name"`
	Role string `json:"role"` // "author", "narrator"
}

// SeriesEntry represents a book's position in a series.
type SeriesEntry struct {
	ASIN     string `json:"asin,omitempty"`
	Name     string `json:"name"`
	Position string `json:"position,omitempty"` // "1", "2", "1-3", etc.
}

// Book represents full audiobook metadata from Audible.
type Book struct {
	ASIN           string        `json:"asin"`
	Title          string        `json:"title"`
	Subtitle       string        `json:"subtitle,omitempty"`
	Authors        []Contributor `json:"authors"`
	Narrators      []Contributor `json:"narrators"`
	Publisher      string        `json:"publisher,omitempty"`
	ReleaseDate    time.Time     `json:"release_date,omitempty"`
	RuntimeMinutes int           `json:"runtime_minutes"`
	Description    string        `json:"description,omitempty"`
	CoverURL       string        `json:"cover_url,omitempty"`
	Series         []SeriesEntry `json:"series,omitempty"`
	Genres         []string      `json:"genres,omitempty"`
	Language       string        `json:"language,omitempty"`
	Rating         float32       `json:"rating,omitempty"`
	RatingCount    int           `json:"rating_count,omitempty"`
}

// Chapter represents a chapter marker.
type Chapter struct {
	Title      string `json:"title"`
	StartMs    int64  `json:"start_ms"`
	DurationMs int64  `json:"duration_ms"`
}

// SearchParams defines parameters for catalog search.
type SearchParams struct {
	Keywords string // General search terms
	Title    string // Search by title
	Author   string // Search by author name
	Narrator string // Search by narrator name
	Limit    int    // Max results (default 25, max 50)
}

// SearchResult is a lighter-weight result for search listings.
type SearchResult struct {
	ASIN           string        `json:"asin"`
	Title          string        `json:"title"`
	Subtitle       string        `json:"subtitle,omitempty"`
	Authors        []Contributor `json:"authors"`
	Narrators      []Contributor `json:"narrators"`
	CoverURL       string        `json:"cover_url,omitempty"`
	RuntimeMinutes int           `json:"runtime_minutes"`
	ReleaseDate    time.Time     `json:"release_date,omitempty"`
}
