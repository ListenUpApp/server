package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerLibraryRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listLibraries",
		Method:      http.MethodGet,
		Path:        "/api/v1/libraries",
		Summary:     "List libraries",
		Description: "Returns all libraries",
		Tags:        []string{"Libraries"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListLibraries)

	huma.Register(s.api, huma.Operation{
		OperationID: "getLibrary",
		Method:      http.MethodGet,
		Path:        "/api/v1/libraries/{id}",
		Summary:     "Get library",
		Description: "Returns a library by ID",
		Tags:        []string{"Libraries"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetLibrary)
}

// === DTOs ===

// ListLibrariesInput contains parameters for listing libraries.
type ListLibrariesInput struct {
	Authorization string `header:"Authorization"`
}

// LibraryResponse contains library data in API responses.
type LibraryResponse struct {
	ID        string    `json:"id" doc:"Library ID"`
	Name      string    `json:"name" doc:"Library name"`
	OwnerID   string    `json:"owner_id" doc:"Owner user ID"`
	ScanPaths []string  `json:"scan_paths" doc:"Paths to scan for audiobooks"`
	SkipInbox bool      `json:"skip_inbox" doc:"Whether to skip inbox for new books"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt time.Time `json:"updated_at" doc:"Last update time"`
}

// ListLibrariesResponse contains a list of libraries.
type ListLibrariesResponse struct {
	Libraries []LibraryResponse `json:"libraries" doc:"List of libraries"`
}

// ListLibrariesOutput wraps the list libraries response for Huma.
type ListLibrariesOutput struct {
	Body ListLibrariesResponse
}

// GetLibraryInput contains parameters for getting a library.
type GetLibraryInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Library ID"`
}

// LibraryOutput wraps the library response for Huma.
type LibraryOutput struct {
	Body LibraryResponse
}

// === Handlers ===

func (s *Server) handleListLibraries(ctx context.Context, _ *ListLibrariesInput) (*ListLibrariesOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]LibraryResponse, len(libraries))
	for i, lib := range libraries {
		resp[i] = LibraryResponse{
			ID:        lib.ID,
			Name:      lib.Name,
			OwnerID:   lib.OwnerID,
			ScanPaths: lib.ScanPaths,
			SkipInbox: lib.SkipInbox,
			CreatedAt: lib.CreatedAt,
			UpdatedAt: lib.UpdatedAt,
		}
	}

	return &ListLibrariesOutput{Body: ListLibrariesResponse{Libraries: resp}}, nil
}

func (s *Server) handleGetLibrary(ctx context.Context, input *GetLibraryInput) (*LibraryOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	lib, err := s.store.GetLibrary(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	return &LibraryOutput{
		Body: LibraryResponse{
			ID:        lib.ID,
			Name:      lib.Name,
			OwnerID:   lib.OwnerID,
			ScanPaths: lib.ScanPaths,
			SkipInbox: lib.SkipInbox,
			CreatedAt: lib.CreatedAt,
			UpdatedAt: lib.UpdatedAt,
		},
	}, nil
}
