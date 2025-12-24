package api

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
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
		OperationID: "getBookTags",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}/tags",
		Summary:     "Get book tags",
		Description: "Returns tags for a book",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBookTags)

	huma.Register(s.api, huma.Operation{
		OperationID: "addBookTag",
		Method:      http.MethodPost,
		Path:        "/api/v1/books/{id}/tags",
		Summary:     "Add book tag",
		Description: "Adds a tag to a book",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAddBookTag)

	huma.Register(s.api, huma.Operation{
		OperationID: "removeBookTag",
		Method:      http.MethodDelete,
		Path:        "/api/v1/books/{id}/tags/{tagId}",
		Summary:     "Remove book tag",
		Description: "Removes a tag from a book",
		Tags:        []string{"Books"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRemoveBookTag)
}

// === DTOs ===

type ListBooksInput struct {
	Authorization string `header:"Authorization"`
	Limit         int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Items per page"`
	Cursor        string `query:"cursor" doc:"Pagination cursor"`
	UpdatedAfter  string `query:"updated_after" doc:"Filter by updated time (RFC3339)"`
}

type BookResponse struct {
	ID            string                    `json:"id" doc:"Book ID"`
	Title         string                    `json:"title" doc:"Book title"`
	Subtitle      string                    `json:"subtitle,omitempty" doc:"Book subtitle"`
	Description   string                    `json:"description,omitempty" doc:"Book description"`
	Publisher     string                    `json:"publisher,omitempty" doc:"Publisher name"`
	PublishYear   string                    `json:"publish_year,omitempty" doc:"Publication year"`
	Language      string                    `json:"language,omitempty" doc:"Language code"`
	Duration      int64                     `json:"duration" doc:"Total duration in milliseconds"`
	Size          int64                     `json:"size" doc:"Total size in bytes"`
	ASIN          string                    `json:"asin,omitempty" doc:"Amazon ASIN"`
	ISBN          string                    `json:"isbn,omitempty" doc:"ISBN"`
	Contributors  []BookContributorResponse `json:"contributors" doc:"Book contributors"`
	Series        []BookSeriesResponse      `json:"series,omitempty" doc:"Series memberships"`
	GenreIDs      []string                  `json:"genre_ids,omitempty" doc:"Genre IDs"`
	AudioFiles    []AudioFileResponse       `json:"audio_files" doc:"Audio files"`
	CreatedAt     time.Time                 `json:"created_at" doc:"Creation time"`
	UpdatedAt     time.Time                 `json:"updated_at" doc:"Last update time"`
}

type BookContributorResponse struct {
	ContributorID string   `json:"contributor_id" doc:"Contributor ID"`
	Name          string   `json:"name" doc:"Contributor name"`
	Roles         []string `json:"roles" doc:"Roles (author, narrator, etc.)"`
}

type BookSeriesResponse struct {
	SeriesID string `json:"series_id" doc:"Series ID"`
	Name     string `json:"name" doc:"Series name"`
	Sequence string `json:"sequence,omitempty" doc:"Sequence in series"`
}

type AudioFileResponse struct {
	ID       string `json:"id" doc:"Audio file ID"`
	Path     string `json:"path" doc:"File path"`
	Duration int64  `json:"duration" doc:"Duration in milliseconds"`
	Size     int64  `json:"size" doc:"Size in bytes"`
	Format   string `json:"format" doc:"Audio format"`
	Codec    string `json:"codec" doc:"Audio codec"`
	Bitrate  int    `json:"bitrate" doc:"Bitrate in bps"`
}

type ListBooksResponse struct {
	Items      []BookResponse `json:"items" doc:"Books"`
	Total      int            `json:"total" doc:"Total count"`
	NextCursor string         `json:"next_cursor,omitempty" doc:"Next page cursor"`
}

type ListBooksOutput struct {
	Body ListBooksResponse
}

type GetBookInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

type BookOutput struct {
	Body BookResponse
}

type UpdateBookRequest struct {
	Title       *string `json:"title,omitempty" doc:"Book title"`
	Subtitle    *string `json:"subtitle,omitempty" doc:"Book subtitle"`
	Description *string `json:"description,omitempty" doc:"Book description"`
	Publisher   *string `json:"publisher,omitempty" doc:"Publisher name"`
	PublishYear *string `json:"publish_year,omitempty" doc:"Publication year"`
	Language    *string `json:"language,omitempty" doc:"Language code"`
	ASIN        *string `json:"asin,omitempty" doc:"Amazon ASIN"`
	ISBN        *string `json:"isbn,omitempty" doc:"ISBN"`
}

type UpdateBookInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          UpdateBookRequest
}

type SetContributorsRequest struct {
	Contributors []ContributorInput `json:"contributors" validate:"required" doc:"Contributors"`
}

type ContributorInput struct {
	Name  string   `json:"name" validate:"required" doc:"Contributor name"`
	Roles []string `json:"roles" validate:"required" doc:"Roles"`
}

type SetContributorsInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          SetContributorsRequest
}

type SetSeriesRequest struct {
	Series []SeriesInput `json:"series" doc:"Series memberships"`
}

type SeriesInput struct {
	Name     string `json:"name" validate:"required" doc:"Series name"`
	Sequence string `json:"sequence,omitempty" doc:"Sequence in series"`
}

type SetSeriesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          SetSeriesRequest
}

type GenreIDsResponse struct {
	GenreIDs []string `json:"genre_ids" doc:"Genre IDs"`
}

type GenresOutput struct {
	Body GenreIDsResponse
}

type GetBookGenresInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

type SetGenresRequest struct {
	GenreIDs []string `json:"genre_ids" validate:"required" doc:"Genre IDs"`
}

type SetGenresInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          SetGenresRequest
}

type TagIDsResponse struct {
	TagIDs []string `json:"tag_ids" doc:"Tag IDs"`
}

type TagsOutput struct {
	Body TagIDsResponse
}

type GetBookTagsInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

type AddTagRequest struct {
	TagID string `json:"tag_id" validate:"required" doc:"Tag ID"`
}

type AddTagInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	Body          AddTagRequest
}

type RemoveTagInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	TagID         string `path:"tagId" doc:"Tag ID"`
}

// === Handlers ===

func (s *Server) handleListBooks(ctx context.Context, input *ListBooksInput) (*ListBooksOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
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

func (s *Server) handleSetBookSeries(ctx context.Context, input *SetSeriesInput) (*BookOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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
	userID, err := s.authenticateRequest(ctx, input.Authorization)
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

func (s *Server) handleGetBookTags(ctx context.Context, input *GetBookTagsInput) (*TagsOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	tags, err := s.services.Tag.GetTagsForBook(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	tagIDs := make([]string, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}

	return &TagsOutput{Body: TagIDsResponse{TagIDs: tagIDs}}, nil
}

func (s *Server) handleAddBookTag(ctx context.Context, input *AddTagInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Tag.AddTagToBook(ctx, userID, input.ID, input.Body.TagID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Tag added"}}, nil
}

func (s *Server) handleRemoveBookTag(ctx context.Context, input *RemoveTagInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Tag.RemoveTagFromBook(ctx, userID, input.ID, input.TagID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Tag removed"}}, nil
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

func convertStringRoles(roles []string) []string {
	result := make([]string, len(roles))
	copy(result, roles)
	return result
}

// Unused but required for potential streaming endpoint
var _ = io.EOF
