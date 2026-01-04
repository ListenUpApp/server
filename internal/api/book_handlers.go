package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerBookRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/books",
		Summary:     "List books",
		Description: "Returns paginated list of books",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "getBook",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}",
		Summary:     "Get book",
		Description: "Returns a single book by ID",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBook)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateBook",
		Method:      http.MethodPatch,
		Path:        "/api/v1/books/{id}",
		Summary:     "Update book",
		Description: "Updates book metadata",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateBook)

	huma.Register(s.api, huma.Operation{
		OperationID: "setBookContributors",
		Method:      http.MethodPut,
		Path:        "/api/v1/books/{id}/contributors",
		Summary:     "Set book contributors",
		Description: "Sets the contributors for a book",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSetBookContributors)

	huma.Register(s.api, huma.Operation{
		OperationID: "setBookSeries",
		Method:      http.MethodPut,
		Path:        "/api/v1/books/{id}/series",
		Summary:     "Set book series",
		Description: "Sets the series for a book",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSetBookSeries)

	huma.Register(s.api, huma.Operation{
		OperationID: "getBookGenres",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}/genres",
		Summary:     "Get book genres",
		Description: "Returns genres for a book",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBookGenres)

	huma.Register(s.api, huma.Operation{
		OperationID: "setBookGenres",
		Method:      http.MethodPut,
		Path:        "/api/v1/books/{id}/genres",
		Summary:     "Set book genres",
		Description: "Sets genres for a book",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSetBookGenres)

	huma.Register(s.api, huma.Operation{
		OperationID: "applyBookMatch",
		Method:      http.MethodPost,
		Path:        "/api/v1/books/{id}/match",
		Summary:     "Apply metadata match",
		Description: "Applies Audible metadata to a book based on user selections",
		Tags:        []string{"Books", "Metadata"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleApplyBookMatch)
}

// === DTOs ===

// ListBooksInput contains parameters for listing books.
type ListBooksInput struct {
	Authorization string `header:"Authorization"`
	Limit         int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Items per page"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	UpdatedAfter  string `query:"updated_after" doc:"Filter by updated time (RFC3339)"`
}

// BookResponse contains book data in API responses.
type BookResponse struct {
	ID           string                    `json:"id" doc:"Book ID"`
	Title        string                    `json:"title" doc:"Book title"`
	Subtitle     string                    `json:"subtitle,omitempty" doc:"Book subtitle"`
	Description  string                    `json:"description,omitempty" doc:"Book description"`
	Publisher    string                    `json:"publisher,omitempty" doc:"Publisher name"`
	PublishYear  string                    `json:"publish_year,omitempty" doc:"Publication year"`
	Language     string                    `json:"language,omitempty" doc:"Language code"`
	Duration     int64                     `json:"duration" doc:"Total duration in milliseconds"`
	Size         int64                     `json:"size" doc:"Total size in bytes"`
	ASIN         string                    `json:"asin,omitempty" doc:"Amazon ASIN"`
	ISBN         string                    `json:"isbn,omitempty" doc:"ISBN"`
	Contributors []BookContributorResponse `json:"contributors" doc:"Book contributors"`
	Series       []BookSeriesResponse      `json:"series,omitempty" doc:"Series memberships"`
	GenreIDs     []string                  `json:"genre_ids,omitempty" doc:"Genre IDs"`
	AudioFiles   []AudioFileResponse       `json:"audio_files" doc:"Audio files"`
	CreatedAt    time.Time                 `json:"created_at" doc:"Creation time"`
	UpdatedAt    time.Time                 `json:"updated_at" doc:"Last update time"`
}

// BookContributorResponse represents a contributor in book responses.
type BookContributorResponse struct {
	ContributorID string   `json:"contributor_id" doc:"Contributor ID"`
	Name          string   `json:"name" doc:"Contributor name"`
	Roles         []string `json:"roles" doc:"Roles (author, narrator, etc.)"`
}

// BookSeriesResponse represents a series membership in book responses.
type BookSeriesResponse struct {
	SeriesID string `json:"series_id" doc:"Series ID"`
	Name     string `json:"name" doc:"Series name"`
	Sequence string `json:"sequence,omitempty" doc:"Sequence in series"`
}

// AudioFileResponse represents an audio file in book responses.
type AudioFileResponse struct {
	ID       string `json:"id" doc:"Audio file ID"`
	Path     string `json:"path" doc:"File path"`
	Duration int64  `json:"duration" doc:"Duration in milliseconds"`
	Size     int64  `json:"size" doc:"Size in bytes"`
	Format   string `json:"format" doc:"Audio format"`
	Codec    string `json:"codec" doc:"Audio codec"`
	Bitrate  int    `json:"bitrate" doc:"Bitrate in bps"`
}

// ListBooksResponse contains a paginated list of books.
type ListBooksResponse struct {
	Items      []BookResponse `json:"items" doc:"Books"`
	Total      int            `json:"total" doc:"Total count"`
	NextCursor string         `json:"next_cursor,omitempty" doc:"Next page cursor"`
}

// ListBooksOutput wraps the list books response for Huma.
type ListBooksOutput struct {
	Body ListBooksResponse
}

// GetBookInput contains parameters for getting a single book.
type GetBookInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

// BookOutput wraps the book response for Huma.
type BookOutput struct {
	Body BookResponse
}

// UpdateBookRequest is the request body for updating a book.
type UpdateBookRequest struct {
	Title       *string    `json:"title,omitempty" validate:"omitempty,min=1,max=500" doc:"Book title"`
	Subtitle    *string    `json:"subtitle,omitempty" validate:"omitempty,max=500" doc:"Book subtitle"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=10000" doc:"Book description"`
	Publisher   *string    `json:"publisher,omitempty" validate:"omitempty,max=200" doc:"Publisher name"`
	PublishYear *string    `json:"publish_year,omitempty" validate:"omitempty,max=10" doc:"Publication year"`
	Language    *string    `json:"language,omitempty" validate:"omitempty,max=10" doc:"Language code"`
	ASIN        *string    `json:"asin,omitempty" validate:"omitempty,len=10" doc:"Amazon ASIN"`
	ISBN        *string    `json:"isbn,omitempty" validate:"omitempty,max=17" doc:"ISBN"`
	CreatedAt   *time.Time `json:"created_at,omitempty" doc:"When the book was added to the library"`
}

// UpdateBookInput wraps the update book request for Huma.
type UpdateBookInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          UpdateBookRequest
}

// SetContributorsRequest is the request body for setting book contributors.
type SetContributorsRequest struct {
	Contributors []ContributorInput `json:"contributors" validate:"required,min=1,max=50,dive" doc:"Contributors"`
}

// ContributorInput represents a contributor in set contributors request.
type ContributorInput struct {
	Name  string   `json:"name" validate:"required,min=1,max=200" doc:"Contributor name"`
	Roles []string `json:"roles" validate:"required,min=1,max=10,dive,min=1,max=50" doc:"Roles"`
}

// SetContributorsInput wraps the set contributors request for Huma.
type SetContributorsInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          SetContributorsRequest
}

// SetSeriesRequest is the request body for setting book series.
type SetSeriesRequest struct {
	Series []SeriesInput `json:"series" validate:"omitempty,max=20,dive" doc:"Series memberships"`
}

// SeriesInput represents a series in set series request.
type SeriesInput struct {
	Name     string `json:"name" validate:"required,min=1,max=200" doc:"Series name"`
	Sequence string `json:"sequence,omitempty" validate:"omitempty,max=50" doc:"Sequence in series"`
}

// SetSeriesInput wraps the set series request for Huma.
type SetSeriesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          SetSeriesRequest
}

// GenreIDsResponse contains genre IDs.
type GenreIDsResponse struct {
	GenreIDs []string `json:"genre_ids" doc:"Genre IDs"`
}

// GenresOutput wraps the genres response for Huma.
type GenresOutput struct {
	Body GenreIDsResponse
}

// GetBookGenresInput contains parameters for getting book genres.
type GetBookGenresInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

// SetGenresRequest is the request body for setting book genres.
type SetGenresRequest struct {
	GenreIDs []string `json:"genre_ids" validate:"required,min=1,max=50" doc:"Genre IDs"`
}

// SetGenresInput wraps the set genres request for Huma.
type SetGenresInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          SetGenresRequest
}

// ApplyMatchRequest is the request body for applying Audible metadata.
type ApplyMatchRequest struct {
	ASIN      string             `json:"asin" doc:"Audible ASIN"`
	Region    string             `json:"region,omitempty" doc:"Audible region"`
	Fields    MatchFieldsRequest `json:"fields" doc:"Fields to apply"`
	Authors   []string           `json:"authors,omitempty" doc:"Author ASINs to apply"`
	Narrators []string           `json:"narrators,omitempty" doc:"Narrator ASINs to apply"`
	Series    []SeriesMatchInput `json:"series,omitempty" doc:"Series to apply"`
	Genres    []string           `json:"genres,omitempty" doc:"Genre names to apply"`
	CoverURL  string             `json:"cover_url,omitempty" doc:"Explicit cover URL to download (overrides Audible cover if provided)"`
}

// MatchFieldsRequest specifies which metadata fields to apply.
type MatchFieldsRequest struct {
	Title       bool `json:"title,omitempty" doc:"Apply title"`
	Subtitle    bool `json:"subtitle,omitempty" doc:"Apply subtitle"`
	Description bool `json:"description,omitempty" doc:"Apply description"`
	Publisher   bool `json:"publisher,omitempty" doc:"Apply publisher"`
	ReleaseDate bool `json:"releaseDate,omitempty" doc:"Apply release date"`
	Language    bool `json:"language,omitempty" doc:"Apply language"`
	Cover       bool `json:"cover,omitempty" doc:"Apply cover"`
}

// SeriesMatchInput specifies series metadata to apply.
type SeriesMatchInput struct {
	ASIN          string `json:"asin,omitempty" doc:"Series ASIN"`
	ApplyName     bool   `json:"applyName,omitempty" doc:"Apply series name"`
	ApplySequence bool   `json:"applySequence,omitempty" doc:"Apply sequence"`
}

// ApplyMatchInput wraps the apply match request for Huma.
type ApplyMatchInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          ApplyMatchRequest
}

// CoverResultResponse contains cover download status.
type CoverResultResponse struct {
	Applied bool   `json:"applied" doc:"Whether cover was successfully applied"`
	Source  string `json:"source,omitempty" doc:"Cover source (audible, itunes)"`
	Width   int    `json:"width,omitempty" doc:"Cover width in pixels"`
	Height  int    `json:"height,omitempty" doc:"Cover height in pixels"`
	Error   string `json:"error,omitempty" doc:"Error message if cover failed"`
}

// ApplyMatchResponse includes the book and cover status.
type ApplyMatchResponse struct {
	Book  BookResponse         `json:"book" doc:"Updated book"`
	Cover *CoverResultResponse `json:"cover,omitempty" doc:"Cover download result (if cover was requested)"`
}

// ApplyMatchOutput wraps the apply match response for Huma.
type ApplyMatchOutput struct {
	Body ApplyMatchResponse
}

// === Handlers ===

func (s *Server) handleListBooks(ctx context.Context, input *ListBooksInput) (*ListBooksOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	params := store.PaginationParams{
		Limit:  input.Limit,
		Cursor: input.Cursor,
	}
	if input.UpdatedAfter != "" {
		if t, err := time.Parse(time.RFC3339, input.UpdatedAfter); err == nil {
			params.UpdatedAfter = t
		}
	}

	result, err := s.services.Book.ListBooks(ctx, userID, params)
	if err != nil {
		return nil, err
	}

	books := make([]BookResponse, len(result.Items))
	for i, b := range result.Items {
		enriched, err := s.store.EnrichBook(ctx, b)
		if err != nil {
			return nil, err
		}
		books[i] = mapEnrichedBookResponse(enriched)
	}

	return &ListBooksOutput{
		Body: ListBooksResponse{
			Items:      books,
			Total:      result.Total,
			NextCursor: result.NextCursor,
		},
	}, nil
}

func (s *Server) handleGetBook(ctx context.Context, input *GetBookInput) (*BookOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	book, err := s.services.Book.GetBook(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	enriched, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		return nil, err
	}

	return &BookOutput{Body: mapEnrichedBookResponse(enriched)}, nil
}

func (s *Server) handleUpdateBook(ctx context.Context, input *UpdateBookInput) (*BookOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Get existing book
	book, err := s.store.GetBook(ctx, input.ID, userID)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if input.Body.Title != nil {
		book.Title = *input.Body.Title
	}
	if input.Body.Subtitle != nil {
		book.Subtitle = *input.Body.Subtitle
	}
	if input.Body.Description != nil {
		book.Description = *input.Body.Description
	}
	if input.Body.Publisher != nil {
		book.Publisher = *input.Body.Publisher
	}
	if input.Body.PublishYear != nil {
		book.PublishYear = *input.Body.PublishYear
	}
	if input.Body.Language != nil {
		book.Language = *input.Body.Language
	}
	if input.Body.ASIN != nil {
		book.ASIN = *input.Body.ASIN
	}
	if input.Body.ISBN != nil {
		book.ISBN = *input.Body.ISBN
	}
	if input.Body.CreatedAt != nil {
		book.CreatedAt = *input.Body.CreatedAt
	}

	if err := s.store.UpdateBook(ctx, book); err != nil {
		return nil, err
	}

	enriched, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		return nil, err
	}

	return &BookOutput{Body: mapEnrichedBookResponse(enriched)}, nil
}

func (s *Server) handleSetBookContributors(ctx context.Context, input *SetContributorsInput) (*BookOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Verify user access BEFORE modifying the book
	if _, err := s.store.GetBook(ctx, input.ID, userID); err != nil {
		return nil, err
	}

	// Convert to store input format
	contributors := make([]store.ContributorInput, len(input.Body.Contributors))
	for i, c := range input.Body.Contributors {
		roles := make([]domain.ContributorRole, len(c.Roles))
		for j, r := range c.Roles {
			roles[j] = domain.ContributorRole(r)
		}
		contributors[i] = store.ContributorInput{
			Name:  c.Name,
			Roles: roles,
		}
	}

	book, err := s.store.SetBookContributors(ctx, input.ID, contributors)
	if err != nil {
		return nil, err
	}

	enriched, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		return nil, err
	}

	return &BookOutput{Body: mapEnrichedBookResponse(enriched)}, nil
}

func (s *Server) handleSetBookSeries(ctx context.Context, input *SetSeriesInput) (*BookOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Convert to store input format
	seriesInputs := make([]store.SeriesInput, len(input.Body.Series))
	for i, s := range input.Body.Series {
		seriesInputs[i] = store.SeriesInput{
			Name:     s.Name,
			Sequence: s.Sequence,
		}
	}

	book, err := s.store.SetBookSeries(ctx, input.ID, seriesInputs)
	if err != nil {
		return nil, err
	}

	// Verify user access
	if _, err := s.store.GetBook(ctx, input.ID, userID); err != nil {
		return nil, err
	}

	enriched, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		return nil, err
	}

	return &BookOutput{Body: mapEnrichedBookResponse(enriched)}, nil
}

func (s *Server) handleGetBookGenres(ctx context.Context, input *GetBookGenresInput) (*GenresOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Verify user access
	if _, err := s.store.GetBook(ctx, input.ID, userID); err != nil {
		return nil, err
	}

	genres, err := s.store.GetGenreIDsForBook(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &GenresOutput{Body: GenreIDsResponse{GenreIDs: genres}}, nil
}

func (s *Server) handleSetBookGenres(ctx context.Context, input *SetGenresInput) (*GenresOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Verify user access
	if _, err := s.store.GetBook(ctx, input.ID, userID); err != nil {
		return nil, err
	}

	if err := s.store.SetBookGenres(ctx, input.ID, input.Body.GenreIDs); err != nil {
		return nil, err
	}

	return &GenresOutput{Body: GenreIDsResponse{GenreIDs: input.Body.GenreIDs}}, nil
}

// === Helpers ===

func mapEnrichedBookResponse(b *dto.Book) BookResponse {
	contributors := make([]BookContributorResponse, len(b.Contributors))
	for i, c := range b.Contributors {
		contributors[i] = BookContributorResponse{
			ContributorID: c.ContributorID,
			Name:          c.Name,
			Roles:         c.Roles,
		}
	}

	series := make([]BookSeriesResponse, len(b.SeriesInfo))
	for i, s := range b.SeriesInfo {
		series[i] = BookSeriesResponse{
			SeriesID: s.SeriesID,
			Name:     s.Name,
			Sequence: s.Sequence,
		}
	}

	audioFiles := make([]AudioFileResponse, len(b.AudioFiles))
	for i, af := range b.AudioFiles {
		audioFiles[i] = AudioFileResponse{
			ID:       af.ID,
			Path:     af.Path,
			Duration: af.Duration,
			Size:     af.Size,
			Format:   af.Format,
			Codec:    af.Codec,
			Bitrate:  af.Bitrate,
		}
	}

	return BookResponse{
		ID:           b.ID,
		Title:        b.Title,
		Subtitle:     b.Subtitle,
		Description:  b.Description,
		Publisher:    b.Publisher,
		PublishYear:  b.PublishYear,
		Language:     b.Language,
		Duration:     b.TotalDuration,
		Size:         b.TotalSize,
		ASIN:         b.ASIN,
		ISBN:         b.ISBN,
		Contributors: contributors,
		Series:       series,
		GenreIDs:     b.GenreIDs,
		AudioFiles:   audioFiles,
		CreatedAt:    b.CreatedAt,
		UpdatedAt:    b.UpdatedAt,
	}
}

func (s *Server) handleApplyBookMatch(ctx context.Context, input *ApplyMatchInput) (*ApplyMatchOutput, error) {
	// Validate ASIN is present
	if input.Body.ASIN == "" {
		return nil, huma.Error400BadRequest("ASIN is required")
	}

	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Convert request to service options
	opts := service.ApplyMatchOptions{
		Fields: service.MatchFields{
			Title:       input.Body.Fields.Title,
			Subtitle:    input.Body.Fields.Subtitle,
			Description: input.Body.Fields.Description,
			Publisher:   input.Body.Fields.Publisher,
			ReleaseDate: input.Body.Fields.ReleaseDate,
			Language:    input.Body.Fields.Language,
			Cover:       input.Body.Fields.Cover,
		},
		Authors:   input.Body.Authors,
		Narrators: input.Body.Narrators,
		Genres:    input.Body.Genres,
		CoverURL:  input.Body.CoverURL,
	}

	// Convert series entries
	for _, se := range input.Body.Series {
		opts.Series = append(opts.Series, service.SeriesMatchEntry{
			ASIN:          se.ASIN,
			ApplyName:     se.ApplyName,
			ApplySequence: se.ApplySequence,
		})
	}

	// Apply the match
	result, err := s.services.Book.ApplyMatchWithCoverResult(ctx, userID, input.ID, input.Body.ASIN, input.Body.Region, opts)
	if err != nil {
		return nil, err
	}

	// Enrich book for response
	enriched, err := s.store.EnrichBook(ctx, result.Book)
	if err != nil {
		return nil, err
	}

	response := ApplyMatchResponse{
		Book: mapEnrichedBookResponse(enriched),
	}

	// Add cover result if a cover was requested
	if result.CoverResult != nil {
		response.Cover = &CoverResultResponse{
			Applied: result.CoverResult.Applied,
			Source:  result.CoverResult.Source,
			Width:   result.CoverResult.Width,
			Height:  result.CoverResult.Height,
			Error:   result.CoverResult.Error,
		}
	}

	return &ApplyMatchOutput{Body: response}, nil
}
