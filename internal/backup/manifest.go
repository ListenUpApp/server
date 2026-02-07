package backup

import "time"

// FormatVersion is the backup format version. Increment major on breaking changes.
const FormatVersion = "1.0"

// Manifest describes backup contents and metadata.
type Manifest struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`

	// Server identity
	ServerID        string `json:"server_id"`
	ServerName      string `json:"server_name"`
	ListenUpVersion string `json:"listenup_version"`

	// Content summary
	Counts EntityCounts `json:"counts"`

	// What's included
	IncludesImages   bool `json:"includes_images"`
	IncludesEvents   bool `json:"includes_events"`
	IncludesSettings bool `json:"includes_settings"`
}

// EntityCounts tracks entity counts for validation and progress reporting.
type EntityCounts struct {
	Users            int `json:"users"`
	Libraries        int `json:"libraries"`
	Books            int `json:"books"`
	Contributors     int `json:"contributors"`
	Series           int `json:"series"`
	Genres           int `json:"genres"`
	Tags             int `json:"tags"`
	Collections      int `json:"collections"`
	CollectionShares int `json:"collection_shares"`
	Shelves           int `json:"shelves"`
	Activities       int `json:"activities"`
	ListeningEvents  int `json:"listening_events"`
	ReadingSessions  int `json:"reading_sessions"`
	Images           int `json:"images,omitempty"`
}
