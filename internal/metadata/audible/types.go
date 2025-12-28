// Package audible provides a client for the Audible catalog API.
package audible

import "time"

// Region represents an Audible marketplace.
type Region string

// Audible marketplace regions.
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

// WebHost returns the website host for this region (for web scraping).
func (r Region) WebHost() string {
	hosts := map[Region]string{
		RegionUS: "www.audible.com",
		RegionUK: "www.audible.co.uk",
		RegionDE: "www.audible.de",
		RegionFR: "www.audible.fr",
		RegionAU: "www.audible.com.au",
		RegionCA: "www.audible.ca",
		RegionJP: "www.audible.co.jp",
		RegionIT: "www.audible.it",
		RegionIN: "www.audible.in",
		RegionES: "www.audible.es",
	}
	if host, ok := hosts[r]; ok {
		return host
	}
	return hosts[RegionUS]
}

// localeCookie returns cookies to force the correct regional catalog.
// Without these, Audible geo-detects based on IP and may serve different content.
func (r Region) localeCookie() string {
	cookies := map[Region]string{
		RegionUS: "lc-main=en_US; i18n-prefs=USD",
		RegionUK: "lc-acbuk=en_GB; i18n-prefs=GBP",
		RegionDE: "lc-acbde=de_DE; i18n-prefs=EUR",
		RegionFR: "lc-acbfr=fr_FR; i18n-prefs=EUR",
		RegionAU: "lc-acbau=en_AU; i18n-prefs=AUD",
		RegionCA: "lc-acbca=en_CA; i18n-prefs=CAD",
		RegionJP: "lc-acbjp=ja_JP; i18n-prefs=JPY",
		RegionIT: "lc-acbit=it_IT; i18n-prefs=EUR",
		RegionIN: "lc-acbin=en_IN; i18n-prefs=INR",
		RegionES: "lc-acbes=es_ES; i18n-prefs=EUR",
	}
	if cookie, ok := cookies[r]; ok {
		return cookie
	}
	return cookies[RegionUS]
}

// Locale returns the API locale string for this region (e.g., "en-US", "de-DE").
func (r Region) Locale() string {
	locales := map[Region]string{
		RegionUS: "en-US",
		RegionUK: "en-GB",
		RegionDE: "de-DE",
		RegionFR: "fr-FR",
		RegionAU: "en-AU",
		RegionCA: "en-CA",
		RegionJP: "ja-JP",
		RegionIT: "it-IT",
		RegionIN: "en-IN",
		RegionES: "es-ES",
	}
	if locale, ok := locales[r]; ok {
		return locale
	}
	return locales[RegionUS]
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

// ContributorProfile represents full contributor metadata from Audible.
type ContributorProfile struct {
	ASIN      string `json:"asin"`
	Name      string `json:"name"`
	Biography string `json:"biography,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
}

// ContributorSearchResult represents a contributor in search results.
type ContributorSearchResult struct {
	ASIN        string `json:"asin"`
	Name        string `json:"name"`
	ImageURL    string `json:"image_url,omitempty"`
	Description string `json:"description,omitempty"` // e.g., "142 titles"
}
