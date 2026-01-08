// Package service provides business logic layer for managing audiobooks, libraries, and synchronization.
package service

import (
	"context"
	"log/slog"
	"sync"

	"github.com/listenupapp/listenup-server/internal/media/covers"
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
	"github.com/listenupapp/listenup-server/internal/metadata/itunes"
)

// CoverOption represents a cover from any source with its metadata.
type CoverOption struct {
	Source   string `json:"source"`    // "audible" or "itunes"
	URL      string `json:"url"`       // Full URL to cover image
	Width    int    `json:"width"`     // Image width in pixels
	Height   int    `json:"height"`    // Image height in pixels
	SourceID string `json:"source_id"` // ASIN for Audible, collectionId for iTunes
}

// CoverSearchResult contains covers from all sources.
type CoverSearchResult struct {
	Covers []CoverOption `json:"covers"`
}

// CoverDownloadResult contains the result of downloading a cover.
type CoverDownloadResult struct {
	Applied bool   `json:"applied"`          // Whether the cover was applied
	Source  string `json:"source,omitempty"` // Source of the cover
	Width   int    `json:"width,omitempty"`  // Actual width of stored cover
	Height  int    `json:"height,omitempty"` // Actual height of stored cover
	Error   string `json:"error,omitempty"`  // Error message if failed
}

// CoverService handles cover search and download from multiple sources.
type CoverService struct {
	itunesClient    *itunes.Client
	metadataService *MetadataService
	downloader      *covers.Downloader
	logger          *slog.Logger
}

// NewCoverService creates a new cover service.
func NewCoverService(
	itunesClient *itunes.Client,
	metadataService *MetadataService,
	coverStorage *images.Storage,
	logger *slog.Logger,
) *CoverService {
	return &CoverService{
		itunesClient:    itunesClient,
		metadataService: metadataService,
		downloader:      covers.NewDownloader(coverStorage, logger),
		logger:          logger,
	}
}

// SearchCovers searches for covers from all available sources.
// Results are ordered: Audible first, then iTunes, preserving each API's relevance order.
func (s *CoverService) SearchCovers(ctx context.Context, title, author string) (*CoverSearchResult, error) {
	var audibleCovers []CoverOption
	var itunesCovers []CoverOption
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Search iTunes
	wg.Go(func() {
		results, err := s.itunesClient.SearchByTitleAndAuthor(ctx, title, author)
		if err != nil {
			s.logger.Warn("iTunes cover search failed",
				"title", title,
				"author", author,
				"error", err,
			)
			return
		}

		mu.Lock()
		for _, r := range results {
			if r.CoverURL == "" {
				continue
			}
			itunesCovers = append(itunesCovers, CoverOption{
				Source:   "itunes",
				URL:      r.CoverURL,
				Width:    r.CoverWidth,
				Height:   r.CoverHeight,
				SourceID: formatITunesID(r.ID),
			})
		}
		mu.Unlock()
	})

	// Search Audible using cached results if available
	wg.Go(func() {
		params := audible.SearchParams{Keywords: title + " " + author}
		results, _, err := s.metadataService.SearchWithFallback(ctx, params)
		if err != nil {
			s.logger.Warn("Audible cover search failed",
				"title", title,
				"author", author,
				"error", err,
			)
			return
		}

		mu.Lock()
		for _, r := range results {
			if r.CoverURL == "" {
				continue
			}
			// Audible covers are typically 500x500
			audibleCovers = append(audibleCovers, CoverOption{
				Source:   "audible",
				URL:      r.CoverURL,
				Width:    500,
				Height:   500,
				SourceID: r.ASIN,
			})
		}
		mu.Unlock()
	})

	wg.Wait()

	// Combine: Audible first (preserving relevance order), then iTunes
	allCovers := make([]CoverOption, 0, len(audibleCovers)+len(itunesCovers))
	allCovers = append(allCovers, audibleCovers...)
	allCovers = append(allCovers, itunesCovers...)

	return &CoverSearchResult{Covers: allCovers}, nil
}

// DownloadCover downloads a cover from the given URL and stores it for the book.
func (s *CoverService) DownloadCover(ctx context.Context, bookID, url string) *CoverDownloadResult {
	source := covers.DetectSource(url)
	result := s.downloader.Download(ctx, bookID, url, source)

	downloadResult := &CoverDownloadResult{
		Applied: result.Success,
		Source:  result.Source,
		Width:   result.Width,
		Height:  result.Height,
	}

	if result.Error != nil {
		downloadResult.Error = result.Error.Error()
	}

	return downloadResult
}

// formatITunesID formats an iTunes collection ID as a string.
func formatITunesID(id int64) string {
	return itoa(id)
}

// itoa converts int64 to string without importing strconv.
func itoa(i int64) string {
	if i == 0 {
		return "0"
	}

	negative := i < 0
	if negative {
		i = -i
	}

	var buf [20]byte
	pos := len(buf)

	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}

	if negative {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}
