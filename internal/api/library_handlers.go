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

	huma.Register(s.api, huma.Operation{
		OperationID: "updateLibrary",
		Method:      http.MethodPatch,
		Path:        "/api/v1/libraries/{id}",
		Summary:     "Update library",
		Description: "Updates a library (admin only)",
		Tags:        []string{"Libraries"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateLibrary)
}

// === DTOs ===

// ListLibrariesInput contains parameters for listing libraries.
type ListLibrariesInput struct {
	Authorization string `header:"Authorization"`
}

// LibraryResponse contains library data in API responses.
type LibraryResponse struct {
	ID         string    `json:"id" doc:"Library ID"`
	Name       string    `json:"name" doc:"Library name"`
	OwnerID    string    `json:"owner_id" doc:"Owner user ID"`
	ScanPaths  []string  `json:"scan_paths" doc:"Paths to scan for audiobooks"`
	SkipInbox  bool      `json:"skip_inbox" doc:"Whether to skip inbox for new books"`
	AccessMode string    `json:"access_mode" doc:"Access mode: open or restricted"`
	CreatedAt  time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt  time.Time `json:"updated_at" doc:"Last update time"`
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

// UpdateLibraryRequest is the request body for updating a library.
type UpdateLibraryRequest struct {
	Name       *string `json:"name,omitempty" validate:"omitempty,min=1,max=100" doc:"Library name"`
	AccessMode *string `json:"access_mode,omitempty" validate:"omitempty,oneof=open restricted" doc:"Access mode: open or restricted"`
}

// UpdateLibraryInput wraps the update library request for Huma.
type UpdateLibraryInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Library ID"`
	Body          UpdateLibraryRequest
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
			ID:         lib.ID,
			Name:       lib.Name,
			OwnerID:    lib.OwnerID,
			ScanPaths:  lib.ScanPaths,
			SkipInbox:  lib.SkipInbox,
			AccessMode: string(lib.GetAccessMode()),
			CreatedAt:  lib.CreatedAt,
			UpdatedAt:  lib.UpdatedAt,
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
			ID:         lib.ID,
			Name:       lib.Name,
			OwnerID:    lib.OwnerID,
			ScanPaths:  lib.ScanPaths,
			SkipInbox:  lib.SkipInbox,
			AccessMode: string(lib.GetAccessMode()),
			CreatedAt:  lib.CreatedAt,
			UpdatedAt:  lib.UpdatedAt,
		},
	}, nil
}

func (s *Server) handleUpdateLibrary(ctx context.Context, input *UpdateLibraryInput) (*LibraryOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	lib, err := s.services.Library.UpdateLibrary(ctx, input.ID, input.Body.Name, input.Body.AccessMode)
	if err != nil {
		return nil, err
	}

	return &LibraryOutput{
		Body: LibraryResponse{
			ID:         lib.ID,
			Name:       lib.Name,
			OwnerID:    lib.OwnerID,
			ScanPaths:  lib.ScanPaths,
			SkipInbox:  lib.SkipInbox,
			AccessMode: string(lib.GetAccessMode()),
			CreatedAt:  lib.CreatedAt,
			UpdatedAt:  lib.UpdatedAt,
		},
	}, nil
}
