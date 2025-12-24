package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerGenreRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listGenres",
		Method:      http.MethodGet,
		Path:        "/api/v1/genres",
		Summary:     "List genres",
		Description: "Returns the full genre tree",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListGenres)

	huma.Register(s.api, huma.Operation{
		OperationID: "createGenre",
		Method:      http.MethodPost,
		Path:        "/api/v1/genres",
		Summary:     "Create genre",
		Description: "Creates a new genre",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateGenre)

	huma.Register(s.api, huma.Operation{
		OperationID: "getGenre",
		Method:      http.MethodGet,
		Path:        "/api/v1/genres/{id}",
		Summary:     "Get genre",
		Description: "Returns a genre by ID",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetGenre)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateGenre",
		Method:      http.MethodPatch,
		Path:        "/api/v1/genres/{id}",
		Summary:     "Update genre",
		Description: "Updates a genre",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateGenre)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteGenre",
		Method:      http.MethodDelete,
		Path:        "/api/v1/genres/{id}",
		Summary:     "Delete genre",
		Description: "Deletes a genre",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteGenre)

	huma.Register(s.api, huma.Operation{
		OperationID: "getGenreChildren",
		Method:      http.MethodGet,
		Path:        "/api/v1/genres/{id}/children",
		Summary:     "Get genre children",
		Description: "Returns direct children of a genre",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetGenreChildren)

	huma.Register(s.api, huma.Operation{
		OperationID: "getGenreBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/genres/{id}/books",
		Summary:     "Get genre books",
		Description: "Returns book IDs in a genre",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetGenreBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "moveGenre",
		Method:      http.MethodPost,
		Path:        "/api/v1/genres/{id}/move",
		Summary:     "Move genre",
		Description: "Moves a genre to a new parent",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMoveGenre)

	huma.Register(s.api, huma.Operation{
		OperationID: "mergeGenres",
		Method:      http.MethodPost,
		Path:        "/api/v1/genres/merge",
		Summary:     "Merge genres",
		Description: "Merges source genre into target genre",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMergeGenres)

	huma.Register(s.api, huma.Operation{
		OperationID: "listUnmappedGenres",
		Method:      http.MethodGet,
		Path:        "/api/v1/genres/unmapped",
		Summary:     "List unmapped genres",
		Description: "Returns raw genre strings that need mapping",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListUnmappedGenres)

	huma.Register(s.api, huma.Operation{
		OperationID: "mapUnmappedGenre",
		Method:      http.MethodPost,
		Path:        "/api/v1/genres/unmapped/map",
		Summary:     "Map unmapped genre",
		Description: "Creates an alias for an unmapped genre",
		Tags:        []string{"Genres"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMapUnmappedGenre)
}

// === DTOs ===

type ListGenresInput struct {
	Authorization string `header:"Authorization"`
}

type GenreResponse struct {
	ID          string    `json:"id" doc:"Genre ID"`
	Name        string    `json:"name" doc:"Genre name"`
	Slug        string    `json:"slug" doc:"URL-safe slug"`
	Description string    `json:"description,omitempty" doc:"Description"`
	ParentID    string    `json:"parent_id,omitempty" doc:"Parent genre ID"`
	Path        string    `json:"path" doc:"Full path (e.g., /fiction/fantasy)"`
	Depth       int       `json:"depth" doc:"Depth in tree"`
	SortOrder   int       `json:"sort_order" doc:"Sort order"`
	Color       string    `json:"color,omitempty" doc:"Display color"`
	CreatedAt   time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt   time.Time `json:"updated_at" doc:"Last update time"`
}

type ListGenresResponse struct {
	Genres []GenreResponse `json:"genres" doc:"List of genres"`
}

type ListGenresOutput struct {
	Body ListGenresResponse
}

type CreateGenreRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100" doc:"Genre name"`
	ParentID    string `json:"parent_id,omitempty" doc:"Parent genre ID"`
	Description string `json:"description,omitempty" doc:"Description"`
	Color       string `json:"color,omitempty" doc:"Display color"`
}

type CreateGenreInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateGenreRequest
}

type GenreOutput struct {
	Body GenreResponse
}

type GetGenreInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Genre ID"`
}

type UpdateGenreRequest struct {
	Name        *string `json:"name,omitempty" doc:"Genre name"`
	Description *string `json:"description,omitempty" doc:"Description"`
	Color       *string `json:"color,omitempty" doc:"Display color"`
	SortOrder   *int    `json:"sort_order,omitempty" doc:"Sort order"`
}

type UpdateGenreInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Genre ID"`
	Body          UpdateGenreRequest
}

type DeleteGenreInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Genre ID"`
}

type GetGenreChildrenInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Genre ID"`
}

type GenreChildrenOutput struct {
	Body ListGenresResponse
}

type GetGenreBooksInput struct {
	Authorization       string `header:"Authorization"`
	ID                  string `path:"id" doc:"Genre ID"`
	IncludeDescendants  bool   `query:"include_descendants" doc:"Include books from child genres"`
}

type GenreBooksResponse struct {
	BookIDs []string `json:"book_ids" doc:"Book IDs in genre"`
}

type GenreBooksOutput struct {
	Body GenreBooksResponse
}

type MoveGenreRequest struct {
	NewParentID string `json:"new_parent_id" doc:"New parent genre ID (empty for root)"`
}

type MoveGenreInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Genre ID"`
	Body          MoveGenreRequest
}

type MergeGenresRequest struct {
	SourceID string `json:"source_id" validate:"required" doc:"Genre to merge from"`
	TargetID string `json:"target_id" validate:"required" doc:"Genre to merge into"`
}

type MergeGenresInput struct {
	Authorization string `header:"Authorization"`
	Body          MergeGenresRequest
}

type ListUnmappedGenresInput struct {
	Authorization string `header:"Authorization"`
}

type UnmappedGenreResponse struct {
	RawValue   string `json:"raw_value" doc:"Raw genre string"`
	BookCount  int    `json:"book_count" doc:"Number of books with this genre"`
	FirstSeenAt time.Time `json:"first_seen_at" doc:"When first encountered"`
}

type ListUnmappedGenresResponse struct {
	UnmappedGenres []UnmappedGenreResponse `json:"unmapped_genres" doc:"List of unmapped genres"`
}

type ListUnmappedGenresOutput struct {
	Body ListUnmappedGenresResponse
}

type MapUnmappedGenreRequest struct {
	RawValue string   `json:"raw_value" validate:"required" doc:"Raw genre string to map"`
	GenreIDs []string `json:"genre_ids" validate:"required,min=1" doc:"Genre IDs to map to"`
}

type MapUnmappedGenreInput struct {
	Authorization string `header:"Authorization"`
	Body          MapUnmappedGenreRequest
}

// === Handlers ===

func (s *Server) handleListGenres(ctx context.Context, input *ListGenresInput) (*ListGenresOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	genres, err := s.services.Genre.ListGenres(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]GenreResponse, len(genres))
	for i, g := range genres {
		resp[i] = mapGenreResponse(g)
	}

	return &ListGenresOutput{Body: ListGenresResponse{Genres: resp}}, nil
}

func (s *Server) handleCreateGenre(ctx context.Context, input *CreateGenreInput) (*GenreOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	g, err := s.services.Genre.CreateGenre(ctx, service.CreateGenreRequest{
		Name:        input.Body.Name,
		ParentID:    input.Body.ParentID,
		Description: input.Body.Description,
		Color:       input.Body.Color,
	})
	if err != nil {
		return nil, err
	}

	return &GenreOutput{Body: mapGenreResponse(g)}, nil
}

func (s *Server) handleGetGenre(ctx context.Context, input *GetGenreInput) (*GenreOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	g, err := s.services.Genre.GetGenre(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &GenreOutput{Body: mapGenreResponse(g)}, nil
}

func (s *Server) handleUpdateGenre(ctx context.Context, input *UpdateGenreInput) (*GenreOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	g, err := s.services.Genre.UpdateGenre(ctx, input.ID, service.UpdateGenreRequest{
		Name:        input.Body.Name,
		Description: input.Body.Description,
		Color:       input.Body.Color,
		SortOrder:   input.Body.SortOrder,
	})
	if err != nil {
		return nil, err
	}

	return &GenreOutput{Body: mapGenreResponse(g)}, nil
}

func (s *Server) handleDeleteGenre(ctx context.Context, input *DeleteGenreInput) (*MessageOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	if err := s.services.Genre.DeleteGenre(ctx, input.ID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Genre deleted"}}, nil
}

func (s *Server) handleGetGenreChildren(ctx context.Context, input *GetGenreChildrenInput) (*GenreChildrenOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	children, err := s.services.Genre.GetGenreChildren(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	resp := make([]GenreResponse, len(children))
	for i, g := range children {
		resp[i] = mapGenreResponse(g)
	}

	return &GenreChildrenOutput{Body: ListGenresResponse{Genres: resp}}, nil
}

func (s *Server) handleGetGenreBooks(ctx context.Context, input *GetGenreBooksInput) (*GenreBooksOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	bookIDs, err := s.services.Genre.GetBooksForGenre(ctx, input.ID, input.IncludeDescendants)
	if err != nil {
		return nil, err
	}

	return &GenreBooksOutput{Body: GenreBooksResponse{BookIDs: bookIDs}}, nil
}

func (s *Server) handleMoveGenre(ctx context.Context, input *MoveGenreInput) (*GenreOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	g, err := s.services.Genre.MoveGenre(ctx, input.ID, input.Body.NewParentID)
	if err != nil {
		return nil, err
	}

	return &GenreOutput{Body: mapGenreResponse(g)}, nil
}

func (s *Server) handleMergeGenres(ctx context.Context, input *MergeGenresInput) (*MessageOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	if err := s.services.Genre.MergeGenres(ctx, service.MergeGenresRequest{
		SourceID: input.Body.SourceID,
		TargetID: input.Body.TargetID,
	}); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Genres merged"}}, nil
}

func (s *Server) handleListUnmappedGenres(ctx context.Context, input *ListUnmappedGenresInput) (*ListUnmappedGenresOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	unmapped, err := s.services.Genre.ListUnmappedGenres(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]UnmappedGenreResponse, len(unmapped))
	for i, u := range unmapped {
		resp[i] = UnmappedGenreResponse{
			RawValue:    u.RawValue,
			BookCount:   u.BookCount,
			FirstSeenAt: u.FirstSeen,
		}
	}

	return &ListUnmappedGenresOutput{Body: ListUnmappedGenresResponse{UnmappedGenres: resp}}, nil
}

func (s *Server) handleMapUnmappedGenre(ctx context.Context, input *MapUnmappedGenreInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Genre.MapUnmappedGenre(ctx, userID, service.MapUnmappedGenreRequest{
		RawValue: input.Body.RawValue,
		GenreIDs: input.Body.GenreIDs,
	}); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Genre mapped"}}, nil
}

// === Mappers ===

func mapGenreResponse(g *domain.Genre) GenreResponse {
	return GenreResponse{
		ID:          g.ID,
		Name:        g.Name,
		Slug:        g.Slug,
		Description: g.Description,
		ParentID:    g.ParentID,
		Path:        g.Path,
		Depth:       g.Depth,
		SortOrder:   g.SortOrder,
		Color:       g.Color,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}
