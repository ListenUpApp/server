// Package service provides business logic layer for managing audiobooks, libraries, and synchronization.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/genre"
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ApplyMatchOptions contains the user's field selections for a match operation.
type ApplyMatchOptions struct {
	Fields    MatchFields
	Authors   []string // Audible ASINs
	Narrators []string // Audible ASINs
	Series    []SeriesMatchEntry
	Genres    []string
}

// MatchFields specifies which simple fields to apply.
type MatchFields struct {
	Title       bool
	Subtitle    bool
	Description bool
	Publisher   bool
	ReleaseDate bool
	Language    bool
	Cover       bool
}

// SeriesMatchEntry specifies a series match with granular control.
type SeriesMatchEntry struct {
	ASIN          string
	ApplyName     bool
	ApplySequence bool
}

// BookService orchestrates book operations.
type BookService struct {
	store           *store.Store
	scanner         *scanner.Scanner
	metadataService *MetadataService
	coverStorage    *images.Storage
	logger          *slog.Logger
}

// NewBookService creates a new book service.
func NewBookService(
	store *store.Store,
	scanner *scanner.Scanner,
	metadataService *MetadataService,
	coverStorage *images.Storage,
	logger *slog.Logger,
) *BookService {
	return &BookService{
		store:           store,
		scanner:         scanner,
		metadataService: metadataService,
		coverStorage:    coverStorage,
		logger:          logger,
	}
}

// ListBooks returns a paginated list of books accessible to the user.
// User can see books that are: (1) not in any collection, OR (2) in at least one collection they have access to.
func (s *BookService) ListBooks(ctx context.Context, userID string, params store.PaginationParams) (*store.PaginatedResult[*domain.Book], error) {
	params.Validate()

	// Get all books the user can access
	accessibleBooks, err := s.store.GetBooksForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get accessible books: %w", err)
	}

	// Apply cursor-based pagination manually
	total := len(accessibleBooks)

	// Decode cursor to get starting index
	startIdx := 0
	if params.Cursor != "" {
		decoded, err := store.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		// For in-memory pagination, cursor is just the index as string
		if _, err := fmt.Sscanf(decoded, "%d", &startIdx); err != nil {
			return nil, fmt.Errorf("invalid cursor format: %w", err)
		}
	}

	if startIdx >= total {
		return &store.PaginatedResult[*domain.Book]{
			Items:   []*domain.Book{},
			Total:   total,
			HasMore: false,
		}, nil
	}

	// Calculate end index
	endIdx := startIdx + params.Limit
	if endIdx > total {
		endIdx = total
	}

	// Slice the results
	items := accessibleBooks[startIdx:endIdx]
	hasMore := endIdx < total

	result := &store.PaginatedResult[*domain.Book]{
		Items:   items,
		Total:   total,
		HasMore: hasMore,
	}

	// Set next cursor if there are more results
	if hasMore {
		result.NextCursor = store.EncodeCursor(fmt.Sprintf("%d", endIdx))
	}

	return result, nil
}

// GetBook retrieves a single book by ID.
// Returns ErrBookNotFound if user doesn't have access to the book.
func (s *BookService) GetBook(ctx context.Context, userID, id string) (*domain.Book, error) {
	return s.store.GetBook(ctx, id, userID)
}

// TriggerScan initiates a full library scan.
// Returns the scan result including statistics about added/updated/unchanged books.
func (s *BookService) TriggerScan(ctx context.Context, libraryID string, opts scanner.ScanOptions) (*scanner.ScanResult, error) {
	// Get library to find scan paths.
	library, err := s.store.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	if len(library.ScanPaths) == 0 {
		return nil, fmt.Errorf("library has no scan paths configured")
	}

	s.logger.Info("triggering library scan",
		"library_id", libraryID,
		"library_name", library.Name,
		"scan_paths", len(library.ScanPaths),
	)

	// For now, scan the first path.
	// TODO: Support scanning multiple paths and aggregating results
	scanPath := library.ScanPaths[0]

	// Set library ID in options for event emission.
	opts.LibraryID = libraryID

	result, err := s.scanner.Scan(ctx, scanPath, opts)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	s.logger.Info("scan complete",
		"library_id", libraryID,
		"added", result.Added,
		"updated", result.Updated,
		"unchanged", result.Unchanged,
		"errors", result.Errors,
		"duration", result.CompletedAt.Sub(result.StartedAt),
	)

	return result, nil
}

// ScanFolder scans a specific folder and returns the book data without saving.
// Useful for previewing what would be scanned.
func (s *BookService) ScanFolder(ctx context.Context, folderPath string) (*domain.Book, error) {
	// Scan the folder.
	item, err := s.scanner.ScanFolder(ctx, folderPath, scanner.ScanOptions{})
	if err != nil {
		return nil, fmt.Errorf("scan folder: %w", err)
	}

	// Convert to book.
	book, err := scanner.ConvertToBook(ctx, item, s.store)
	if err != nil {
		return nil, fmt.Errorf("convert to book: %w", err)
	}

	return book, nil
}

// ApplyMatch applies selected Audible metadata to a local book.
func (s *BookService) ApplyMatch(
	ctx context.Context,
	userID, bookID, asin, region string,
	opts ApplyMatchOptions,
) (*domain.Book, error) {
	// Get book with ACL check
	book, err := s.store.GetBook(ctx, bookID, userID)
	if err != nil {
		return nil, err
	}

	// Parse region
	var audibleRegion *audible.Region
	if region != "" {
		r := audible.Region(region)
		if r.Valid() {
			audibleRegion = &r
		}
	}

	// Fetch metadata from Audible
	audibleBook, err := s.metadataService.GetBook(ctx, audibleRegion, asin)
	if err != nil {
		return nil, fmt.Errorf("fetch audible metadata: %w", err)
	}

	// Apply simple fields
	if opts.Fields.Title {
		book.Title = audibleBook.Title
	}
	if opts.Fields.Subtitle {
		book.Subtitle = audibleBook.Subtitle
	}
	if opts.Fields.Description {
		book.Description = audibleBook.Description
	}
	if opts.Fields.Publisher {
		book.Publisher = audibleBook.Publisher
	}
	if opts.Fields.ReleaseDate && !audibleBook.ReleaseDate.IsZero() {
		book.PublishYear = audibleBook.ReleaseDate.Format("2006")
	}
	if opts.Fields.Language {
		book.Language = audibleBook.Language
	}

	// Store ASIN and region for future refresh
	book.ASIN = asin
	if audibleRegion != nil {
		book.AudibleRegion = string(*audibleRegion)
	}

	// Apply contributors
	if len(opts.Authors) > 0 || len(opts.Narrators) > 0 {
		contributors, err := s.resolveContributors(ctx, audibleBook, opts.Authors, opts.Narrators)
		if err != nil {
			return nil, fmt.Errorf("resolve contributors: %w", err)
		}
		book.Contributors = contributors
	}

	// Apply series
	if len(opts.Series) > 0 {
		seriesLinks, err := s.resolveSeries(ctx, audibleBook.Series, opts.Series)
		if err != nil {
			return nil, fmt.Errorf("resolve series: %w", err)
		}
		book.Series = seriesLinks
	}

	// Apply genres
	if len(opts.Genres) > 0 {
		genreIDs, err := s.resolveGenres(ctx, opts.Genres)
		if err != nil {
			return nil, fmt.Errorf("resolve genres: %w", err)
		}
		book.GenreIDs = genreIDs
	}

	// Apply cover
	if opts.Fields.Cover && audibleBook.CoverURL != "" {
		if err := s.applyCover(ctx, book, audibleBook.CoverURL); err != nil {
			s.logger.Warn("Failed to apply cover", "error", err, "book_id", bookID)
			// Continue without cover - don't fail the whole operation
		}
	}

	// Update book
	if err := s.store.UpdateBook(ctx, book); err != nil {
		return nil, fmt.Errorf("update book: %w", err)
	}

	s.logger.Info("Applied Audible match",
		"book_id", bookID,
		"asin", asin,
		"region", region,
	)

	return book, nil
}

// resolveContributors resolves Audible contributors to local entities.
// Uses ASIN-first matching: check ASIN → check name → create new.
// If found by name, enriches existing contributor with ASIN.
func (s *BookService) resolveContributors(
	ctx context.Context,
	audibleBook *audible.Book,
	authorASINs, narratorASINs []string,
) ([]domain.BookContributor, error) {
	var bookContributors []domain.BookContributor

	// Build lookup maps for selected ASINs
	selectedAuthors := make(map[string]bool)
	for _, asin := range authorASINs {
		selectedAuthors[asin] = true
	}
	selectedNarrators := make(map[string]bool)
	for _, asin := range narratorASINs {
		selectedNarrators[asin] = true
	}

	// Process authors
	for _, author := range audibleBook.Authors {
		if !selectedAuthors[author.ASIN] {
			continue
		}

		contributor, err := s.resolveContributor(ctx, author)
		if err != nil {
			return nil, err
		}

		bookContributors = append(bookContributors, domain.BookContributor{
			ContributorID: contributor.ID,
			Roles:         []domain.ContributorRole{domain.RoleAuthor},
		})
	}

	// Process narrators
	for _, narrator := range audibleBook.Narrators {
		if !selectedNarrators[narrator.ASIN] {
			continue
		}

		contributor, err := s.resolveContributor(ctx, narrator)
		if err != nil {
			return nil, err
		}

		// Check if this contributor is already in the list (could be author+narrator)
		found := false
		for i := range bookContributors {
			if bookContributors[i].ContributorID == contributor.ID {
				bookContributors[i].Roles = append(bookContributors[i].Roles, domain.RoleNarrator)
				found = true
				break
			}
		}

		if !found {
			bookContributors = append(bookContributors, domain.BookContributor{
				ContributorID: contributor.ID,
				Roles:         []domain.ContributorRole{domain.RoleNarrator},
			})
		}
	}

	return bookContributors, nil
}

// resolveContributor resolves a single Audible contributor.
func (s *BookService) resolveContributor(ctx context.Context, audibleContrib audible.Contributor) (*domain.Contributor, error) {
	// 1. Try ASIN lookup first
	if audibleContrib.ASIN != "" {
		existing, err := s.store.GetContributorByASIN(ctx, audibleContrib.ASIN)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, store.ErrContributorNotFound) {
			return nil, err
		}
	}

	// 2. Fall back to name matching
	existing, err := s.store.GetOrCreateContributorByName(ctx, audibleContrib.Name)
	if err != nil {
		return nil, err
	}

	// 3. Enrich with ASIN if found by name and missing ASIN
	if existing.ASIN == "" && audibleContrib.ASIN != "" {
		existing.ASIN = audibleContrib.ASIN
		if err := s.store.UpdateContributor(ctx, existing); err != nil {
			s.logger.Warn("Failed to enrich contributor with ASIN",
				"error", err,
				"contributor_id", existing.ID,
				"asin", audibleContrib.ASIN,
			)
			// Continue without enrichment
		}
	}

	return existing, nil
}

// resolveSeries resolves Audible series to local entities.
func (s *BookService) resolveSeries(
	ctx context.Context,
	audibleSeries []audible.SeriesEntry,
	selections []SeriesMatchEntry,
) ([]domain.BookSeries, error) {
	var bookSeries []domain.BookSeries

	// Build lookup map for selections
	selectionMap := make(map[string]SeriesMatchEntry)
	for _, sel := range selections {
		selectionMap[sel.ASIN] = sel
	}

	for _, as := range audibleSeries {
		sel, selected := selectionMap[as.ASIN]
		if !selected {
			continue
		}

		// Resolve series entity
		series, err := s.resolveSingleSeries(ctx, as)
		if err != nil {
			return nil, err
		}

		bs := domain.BookSeries{
			SeriesID: series.ID,
		}

		// Apply sequence only if selected
		if sel.ApplySequence {
			bs.Sequence = as.Position
		}

		bookSeries = append(bookSeries, bs)
	}

	return bookSeries, nil
}

// resolveSingleSeries resolves a single Audible series.
func (s *BookService) resolveSingleSeries(ctx context.Context, audibleSeries audible.SeriesEntry) (*domain.Series, error) {
	// 1. Try ASIN lookup first
	if audibleSeries.ASIN != "" {
		existing, err := s.store.GetSeriesByASIN(ctx, audibleSeries.ASIN)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, store.ErrSeriesNotFound) {
			return nil, err
		}
	}

	// 2. Fall back to name matching
	existing, err := s.store.GetOrCreateSeriesByName(ctx, audibleSeries.Name)
	if err != nil {
		return nil, err
	}

	// 3. Enrich with ASIN if found by name and missing ASIN
	if existing.ASIN == "" && audibleSeries.ASIN != "" {
		existing.ASIN = audibleSeries.ASIN
		if err := s.store.UpdateSeries(ctx, existing); err != nil {
			s.logger.Warn("Failed to enrich series with ASIN",
				"error", err,
				"series_id", existing.ID,
				"asin", audibleSeries.ASIN,
			)
		}
	}

	return existing, nil
}

// resolveGenres resolves genre strings to genre IDs.
func (s *BookService) resolveGenres(ctx context.Context, genreNames []string) ([]string, error) {
	var genreIDs []string

	for _, name := range genreNames {
		// Create genre slug from name
		slug := genre.Slugify(name)

		// Get or create the genre
		g, err := s.store.GetOrCreateGenreBySlug(ctx, slug, name, "")
		if err != nil {
			return nil, fmt.Errorf("resolve genre %q: %w", name, err)
		}
		genreIDs = append(genreIDs, g.ID)
	}

	return genreIDs, nil
}

// applyCover downloads and stores a cover image from a URL.
func (s *BookService) applyCover(ctx context.Context, book *domain.Book, coverURL string) error {
	// Download image
	resp, err := http.Get(coverURL)
	if err != nil {
		return fmt.Errorf("download cover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download cover: status %d", resp.StatusCode)
	}

	// Read image data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read cover data: %w", err)
	}

	// Store using image service
	if err := s.coverStorage.Save(book.ID, data); err != nil {
		return fmt.Errorf("store cover: %w", err)
	}

	// Note: CoverImage metadata is typically set during scanning, not here
	// This function only downloads and stores the raw image file
	return nil
}
