package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/search"
	"github.com/listenupapp/listenup-server/internal/store"
)

// SearchService provides search functionality across the library.
// It bridges the search index with the data store, handling document
// creation, updates, and query execution.
type SearchService struct {
	index  *search.SearchIndex
	store  store.Store
	logger *slog.Logger
}

// NewSearchService creates a new search service.
func NewSearchService(index *search.SearchIndex, store store.Store, logger *slog.Logger) *SearchService {
	return &SearchService{
		index:  index,
		store:  store,
		logger: logger,
	}
}

// Search performs a federated search across books, contributors, and series.
func (s *SearchService) Search(ctx context.Context, params search.SearchParams) (*search.SearchResult, error) {
	return s.index.Search(ctx, params)
}

// SearchContributors performs a fast contributor search for autocomplete.
// Uses Bleve index for O(log n) performance instead of O(n) BadgerDB scan.
func (s *SearchService) SearchContributors(ctx context.Context, query string, limit int) ([]search.ContributorSearchResult, error) {
	return s.index.SearchContributors(ctx, query, limit)
}

// IndexBook indexes a single book.
// Call this when a book is created or updated.
func (s *SearchService) IndexBook(ctx context.Context, book *domain.Book) error {
	doc, err := s.buildBookDocument(ctx, book)
	if err != nil {
		return fmt.Errorf("build document: %w", err)
	}

	if err := s.index.IndexDocument(doc); err != nil {
		return fmt.Errorf("index document: %w", err)
	}

	s.logger.Debug("indexed book", "id", book.ID, "title", book.Title)
	return nil
}

// IndexContributor indexes a single contributor.
func (s *SearchService) IndexContributor(ctx context.Context, c *domain.Contributor) error {
	// Count books by this contributor
	bookCount, err := s.store.CountBooksForContributor(ctx, c.ID)
	if err != nil {
		s.logger.Warn("failed to count books for contributor", "id", c.ID, "error", err)
		bookCount = 0
	}

	doc := search.ContributorToSearchDocument(c, bookCount)

	if err := s.index.IndexDocument(doc); err != nil {
		return fmt.Errorf("index contributor: %w", err)
	}

	s.logger.Debug("indexed contributor", "id", c.ID, "name", c.Name)
	return nil
}

// IndexSeries indexes a single series.
func (s *SearchService) IndexSeries(ctx context.Context, series *domain.Series) error {
	doc := search.SeriesToSearchDocument(series)

	if err := s.index.IndexDocument(doc); err != nil {
		return fmt.Errorf("index series: %w", err)
	}

	s.logger.Debug("indexed series", "id", series.ID, "name", series.Name)
	return nil
}

// DeleteBook removes a book from the index.
func (s *SearchService) DeleteBook(_ context.Context, bookID string) error {
	return s.index.DeleteDocument(bookID)
}

// DeleteContributor removes a contributor from the index.
func (s *SearchService) DeleteContributor(_ context.Context, contributorID string) error {
	return s.index.DeleteDocument(contributorID)
}

// DeleteSeries removes a series from the index.
func (s *SearchService) DeleteSeries(_ context.Context, seriesID string) error {
	return s.index.DeleteDocument(seriesID)
}

// UpdateBookTags updates just the tags field of a book in the index.
// This re-indexes the entire book document with updated tags.
func (s *SearchService) UpdateBookTags(ctx context.Context, bookID string, tagSlugs []string) error {
	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	doc, err := s.buildBookDocument(ctx, book)
	if err != nil {
		return fmt.Errorf("build document: %w", err)
	}

	// Add tags to the document
	doc.Tags = tagSlugs

	if err := s.index.IndexDocument(doc); err != nil {
		return fmt.Errorf("index document: %w", err)
	}

	s.logger.Debug("updated book tags in index", "id", bookID, "tags", tagSlugs)
	return nil
}

// DocumentCount returns the number of indexed documents.
func (s *SearchService) DocumentCount() (uint64, error) {
	return s.index.DocumentCount()
}

// ReindexAll rebuilds the entire search index.
// This is a heavy operation - use sparingly.
func (s *SearchService) ReindexAll(ctx context.Context) error {
	s.logger.Info("starting full reindex")

	// Rebuild index (drops existing)
	if err := s.index.Rebuild(); err != nil {
		return fmt.Errorf("rebuild index: %w", err)
	}

	// Index all books
	books, err := s.store.ListAllBooks(ctx)
	if err != nil {
		return fmt.Errorf("list books: %w", err)
	}

	bookDocs := make([]*search.SearchDocument, 0, len(books))
	for _, book := range books {
		if book.IsDeleted() {
			continue
		}
		doc, err := s.buildBookDocument(ctx, book)
		if err != nil {
			s.logger.Warn("failed to build book document", "id", book.ID, "error", err)
			continue
		}
		bookDocs = append(bookDocs, doc)
	}

	if len(bookDocs) > 0 {
		if err := s.index.IndexDocuments(bookDocs); err != nil {
			return fmt.Errorf("index books: %w", err)
		}
	}
	s.logger.Info("indexed books", "count", len(bookDocs))

	// Index all contributors
	contributors, err := s.store.ListAllContributors(ctx)
	if err != nil {
		return fmt.Errorf("list contributors: %w", err)
	}

	// Get book counts for all contributors in one scan (not N+1)
	contributorBookCounts, _ := s.store.CountBooksForAllContributors(ctx)

	contribDocs := make([]*search.SearchDocument, 0, len(contributors))
	for _, c := range contributors {
		if c.IsDeleted() {
			continue
		}
		bookCount := contributorBookCounts[c.ID]
		doc := search.ContributorToSearchDocument(c, bookCount)
		contribDocs = append(contribDocs, doc)
	}

	if len(contribDocs) > 0 {
		if err := s.index.IndexDocuments(contribDocs); err != nil {
			return fmt.Errorf("index contributors: %w", err)
		}
	}
	s.logger.Info("indexed contributors", "count", len(contribDocs))

	// Index all series
	allSeries, err := s.store.ListAllSeries(ctx)
	if err != nil {
		return fmt.Errorf("list series: %w", err)
	}

	// Get book counts for all series in one batch
	seriesIDs := make([]string, 0, len(allSeries))
	for _, series := range allSeries {
		if !series.IsDeleted() {
			seriesIDs = append(seriesIDs, series.ID)
		}
	}
	seriesBookCounts, _ := s.store.CountBooksForMultipleSeries(ctx, seriesIDs)

	seriesDocs := make([]*search.SearchDocument, 0, len(allSeries))
	for _, series := range allSeries {
		if series.IsDeleted() {
			continue
		}
		doc := search.SeriesToSearchDocument(series)
		// Set actual count of books we have
		if count, ok := seriesBookCounts[series.ID]; ok {
			doc.BookCount = count
		}
		seriesDocs = append(seriesDocs, doc)
	}

	if len(seriesDocs) > 0 {
		if err := s.index.IndexDocuments(seriesDocs); err != nil {
			return fmt.Errorf("index series: %w", err)
		}
	}
	s.logger.Info("indexed series", "count", len(seriesDocs))

	total, _ := s.index.DocumentCount()
	s.logger.Info("full reindex complete", "total_documents", total)

	return nil
}

// buildBookDocument creates a search document from a book with denormalized fields.
func (s *SearchService) buildBookDocument(ctx context.Context, book *domain.Book) (*search.SearchDocument, error) {
	// Batch fetch all contributors for this book (single query instead of N+1)
	contributorIDs := make([]string, 0, len(book.Contributors))
	for _, bc := range book.Contributors {
		contributorIDs = append(contributorIDs, bc.ContributorID)
	}

	contributors, _ := s.store.GetContributorsByIDs(ctx, contributorIDs)
	contributorMap := make(map[string]*domain.Contributor, len(contributors))
	for _, c := range contributors {
		contributorMap[c.ID] = c
	}

	// Get author and narrator names (first of each role)
	var authorName string
	var narratorName string

	for _, bc := range book.Contributors {
		c := contributorMap[bc.ContributorID]
		if c == nil {
			continue
		}

		for _, role := range bc.Roles {
			if authorName == "" && role == domain.RoleAuthor {
				authorName = c.Name
			}
			if narratorName == "" && role == domain.RoleNarrator {
				narratorName = c.Name
			}
		}

		if authorName != "" && narratorName != "" {
			break
		}
	}

	// Get series name (use first/primary series for search)
	var seriesName string
	if len(book.Series) > 0 {
		series, err := s.store.GetSeries(ctx, book.Series[0].SeriesID)
		if err == nil {
			seriesName = series.Name
		}
	}

	// Batch fetch genres and expand paths (single query instead of N+1)
	genrePaths, genreSlugs := s.expandGenrePathsBatch(ctx, book.GenreIDs)

	// Get tag slugs for the book
	tagSlugs, _ := s.store.GetTagSlugsForBook(ctx, book.ID)

	doc := search.BookToSearchDocument(
		book,
		authorName,
		narratorName,
		seriesName,
		genrePaths,
		genreSlugs,
	)
	doc.Tags = tagSlugs

	return doc, nil
}

// expandGenrePathsBatch takes a list of genre IDs and returns:
// 1. All genre paths including ancestors (for hierarchical filtering)
// 2. All genre slugs (for exact matching)
//
// Uses batch fetch to avoid N+1 queries.
//
// For example, if a book has genre "Epic Fantasy" with path "/fiction/fantasy/epic-fantasy",
// this returns:
//
//	genrePaths: ["/fiction/fantasy/epic-fantasy", "/fiction/fantasy", "/fiction"]
//	genreSlugs: ["epic-fantasy"]
//
// This enables searches like "all Fantasy books" to include Epic Fantasy books.
func (s *SearchService) expandGenrePathsBatch(ctx context.Context, genreIDs []string) (genrePaths, genreSlugs []string) {
	if len(genreIDs) == 0 {
		return nil, nil
	}

	// Batch fetch all genres in one query
	genres, err := s.store.GetGenresByIDs(ctx, genreIDs)
	if err != nil {
		return nil, nil
	}

	pathSet := make(map[string]bool)
	slugSet := make(map[string]bool)

	for _, genre := range genres {
		// Add the slug
		if genre.Slug != "" {
			slugSet[genre.Slug] = true
		}

		// Add the full path and all ancestor paths
		// e.g., "/fiction/fantasy/epic-fantasy" expands to:
		// - "/fiction/fantasy/epic-fantasy"
		// - "/fiction/fantasy"
		// - "/fiction"
		path := genre.Path
		for path != "" {
			pathSet[path] = true

			// Find last slash to get parent path
			lastSlash := strings.LastIndex(path, "/")
			if lastSlash <= 0 {
				break
			}
			path = path[:lastSlash]
		}
	}

	// Convert sets to slices
	for p := range pathSet {
		genrePaths = append(genrePaths, p)
	}
	for slug := range slugSet {
		genreSlugs = append(genreSlugs, slug)
	}

	return genrePaths, genreSlugs
}
