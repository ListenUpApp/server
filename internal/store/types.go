package store

import (
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
)

// ContributorInput represents a contributor to be set on a book.
type ContributorInput struct {
	Name  string                   `json:"name"`
	Roles []domain.ContributorRole `json:"roles"`
}

// SeriesInput represents a series to be set on a book.
type SeriesInput struct {
	Name     string `json:"name"`
	Sequence string `json:"sequence"`
}

// BootstrapResult contains the initialized library and collections.
type BootstrapResult struct {
	Library         *domain.Library
	InboxCollection *domain.Collection
	IsNewLibrary    bool
}

// CachedBook wraps fetched book metadata with cache info.
type CachedBook struct {
	Book      *audible.Book  `json:"book"`
	FetchedAt time.Time      `json:"fetched_at"`
	Region    audible.Region `json:"region"`
}

// CachedChapters wraps fetched chapter data with cache info.
type CachedChapters struct {
	Chapters  []audible.Chapter `json:"chapters"`
	FetchedAt time.Time         `json:"fetched_at"`
	Region    audible.Region    `json:"region"`
}

// CachedSearch wraps search results with cache info.
type CachedSearch struct {
	Results   []audible.SearchResult `json:"results"`
	FetchedAt time.Time              `json:"fetched_at"`
	Region    audible.Region         `json:"region"`
	Query     string                 `json:"query"`
}
