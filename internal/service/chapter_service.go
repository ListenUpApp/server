package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/chapters"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ErrNoASIN is returned when a book has no ASIN for chapter lookup.
var ErrNoASIN = errors.New("book has no ASIN for chapter lookup")

// ChapterService handles chapter name alignment operations.
type ChapterService struct {
	store           *store.Store
	metadataService *MetadataService
	logger          *slog.Logger
}

// NewChapterService creates a new chapter service.
func NewChapterService(
	store *store.Store,
	metadataService *MetadataService,
	logger *slog.Logger,
) *ChapterService {
	return &ChapterService{
		store:           store,
		metadataService: metadataService,
		logger:          logger,
	}
}

// SuggestChapterNames analyzes a book and returns alignment suggestions.
func (s *ChapterService) SuggestChapterNames(
	ctx context.Context,
	userID, bookID string,
	asinOverride, regionOverride string,
) (*chapters.AlignmentResult, error) {
	// Get book with ACL check
	book, err := s.store.GetBook(ctx, bookID, userID)
	if err != nil {
		return nil, err
	}

	// Determine ASIN
	asin := asinOverride
	if asin == "" {
		asin = book.ASIN
	}
	if asin == "" {
		return nil, ErrNoASIN
	}

	// Determine region
	var region *audible.Region
	if regionOverride != "" {
		r := audible.Region(regionOverride)
		if r.Valid() {
			region = &r
		}
	} else if book.AudibleRegion != "" {
		r := audible.Region(book.AudibleRegion)
		if r.Valid() {
			region = &r
		}
	}

	// Fetch remote chapters
	remoteChapters, err := s.metadataService.GetChapters(ctx, region, asin)
	if err != nil {
		return nil, fmt.Errorf("fetch remote chapters: %w", err)
	}

	// Convert domain chapters to alignment format
	localChapters := convertDomainChapters(book.Chapters)

	// Convert audible chapters to alignment format
	remoteAlignChapters := convertAudibleChapters(remoteChapters)

	// Run alignment
	result := chapters.Align(localChapters, remoteAlignChapters)

	// Add detection info
	analysis := chapters.AnalyzeChapters(localChapters)
	result.NeedsUpdate = analysis.NeedsUpdate

	s.logger.Info("Generated chapter suggestions",
		"book_id", bookID,
		"local_count", len(book.Chapters),
		"remote_count", len(remoteChapters),
		"needs_update", result.NeedsUpdate,
		"confidence", result.OverallConfidence,
	)

	return &result, nil
}

// ApplyChapterNames updates chapter titles in the database.
func (s *ChapterService) ApplyChapterNames(
	ctx context.Context,
	userID, bookID string,
	updates []chapters.AlignedChapter,
) (*domain.Book, error) {
	// Get book with ACL check
	book, err := s.store.GetBook(ctx, bookID, userID)
	if err != nil {
		return nil, err
	}

	// Build update map
	updateMap := make(map[int]string)
	for _, u := range updates {
		if u.SuggestedName != "" {
			updateMap[u.Index] = u.SuggestedName
		}
	}

	// Apply updates
	for i := range book.Chapters {
		if newTitle, ok := updateMap[i]; ok {
			book.Chapters[i].Title = newTitle
		}
	}

	// Save book
	if err := s.store.UpdateBook(ctx, book); err != nil {
		return nil, fmt.Errorf("save book: %w", err)
	}

	s.logger.Info("Applied chapter names",
		"book_id", bookID,
		"updated_count", len(updateMap),
	)

	return book, nil
}

func convertDomainChapters(dc []domain.Chapter) []chapters.Chapter {
	result := make([]chapters.Chapter, len(dc))
	for i, c := range dc {
		result[i] = chapters.Chapter{
			Title:     c.Title,
			StartTime: c.StartTime,
			EndTime:   c.EndTime,
		}
	}
	return result
}

func convertAudibleChapters(ac []audible.Chapter) []chapters.RemoteChapter {
	result := make([]chapters.RemoteChapter, len(ac))
	for i, c := range ac {
		result[i] = chapters.RemoteChapter{
			Title:      c.Title,
			StartMs:    c.StartMs,
			DurationMs: c.DurationMs,
		}
	}
	return result
}
