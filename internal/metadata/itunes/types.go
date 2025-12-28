// Package itunes provides a client for searching Apple iTunes audiobook covers.
package itunes

// AudiobookResult represents an audiobook from iTunes search.
type AudiobookResult struct {
	ID          int64  `json:"id"`           // iTunes collectionId
	Title       string `json:"title"`        // Collection name
	Artist      string `json:"artist"`       // Author name
	CoverURL    string `json:"cover_url"`    // High-res cover URL
	CoverWidth  int    `json:"cover_width"`  // Actual image width
	CoverHeight int    `json:"cover_height"` // Actual image height
}

// searchResponse is the raw iTunes API response.
type searchResponse struct {
	ResultCount int            `json:"resultCount"`
	Results     []searchResult `json:"results"`
}

// searchResult is a single result from iTunes search.
type searchResult struct {
	WrapperType      string  `json:"wrapperType"`
	CollectionType   string  `json:"collectionType"`
	CollectionID     int64   `json:"collectionId"`
	CollectionName   string  `json:"collectionName"`
	ArtistName       string  `json:"artistName"`
	ArtworkURL60     string  `json:"artworkUrl60"`
	ArtworkURL100    string  `json:"artworkUrl100"`
	CollectionPrice  float64 `json:"collectionPrice,omitempty"`
	TrackCount       int     `json:"trackCount,omitempty"`
	ReleaseDate      string  `json:"releaseDate,omitempty"`
	PrimaryGenreName string  `json:"primaryGenreName,omitempty"`
}
