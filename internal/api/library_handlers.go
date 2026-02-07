package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/scanner"
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

	huma.Register(s.api, huma.Operation{
		OperationID: "getLibraryStatus",
		Method:      http.MethodGet,
		Path:        "/api/v1/library/status",
		Summary:     "Get library status",
		Description: "Returns whether a library exists and its basic info. Used by clients to determine if setup is needed.",
		Tags:        []string{"Libraries"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetLibraryStatus)

	huma.Register(s.api, huma.Operation{
		OperationID: "addScanPath",
		Method:      http.MethodPost,
		Path:        "/api/v1/libraries/{id}/scan-paths",
		Summary:     "Add scan path",
		Description: "Adds a scan path to a library. Admin only.",
		Tags:        []string{"Libraries"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleAddScanPath)

	huma.Register(s.api, huma.Operation{
		OperationID: "removeScanPath",
		Method:      http.MethodDelete,
		Path:        "/api/v1/libraries/{id}/scan-paths",
		Summary:     "Remove scan path",
		Description: "Removes a scan path from a library. Admin only.",
		Tags:        []string{"Libraries"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRemoveScanPath)

	huma.Register(s.api, huma.Operation{
		OperationID: "triggerScan",
		Method:      http.MethodPost,
		Path:        "/api/v1/libraries/{id}/scan",
		Summary:     "Trigger library scan",
		Description: "Triggers a manual library rescan. Admin only.",
		Tags:        []string{"Libraries"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleTriggerScan)

	huma.Register(s.api, huma.Operation{
		OperationID: "setupLibrary",
		Method:      http.MethodPost,
		Path:        "/api/v1/library/setup",
		Summary:     "Initial library setup",
		Description: "Creates the library with initial scan paths. Admin only. Fails if library already exists.",
		Tags:        []string{"Libraries"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSetupLibrary)
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

// LibraryStatusInput contains parameters for getting library status.
type LibraryStatusInput struct {
	Authorization string `header:"Authorization"`
}

// LibraryStatusResponse contains library existence and basic info.
type LibraryStatusResponse struct {
	Exists     bool             `json:"exists" doc:"Whether a library exists"`
	Library    *LibraryResponse `json:"library,omitempty" doc:"Library info if exists"`
	NeedsSetup bool             `json:"needs_setup" doc:"Whether setup is required (true only for admins when no library)"`
	BookCount  int              `json:"book_count" doc:"Number of books in library"`
	IsScanning bool             `json:"is_scanning" doc:"Whether a scan is in progress"`
}

// LibraryStatusOutput wraps the response for Huma.
type LibraryStatusOutput struct {
	Body LibraryStatusResponse
}

// SetupLibraryInput contains parameters for initial library setup.
type SetupLibraryInput struct {
	Authorization string `header:"Authorization"`
	Body          SetupLibraryRequest
}

// SetupLibraryRequest contains the initial library configuration.
type SetupLibraryRequest struct {
	Name      string   `json:"name" default:"My Library" doc:"Library name"`
	ScanPaths []string `json:"scan_paths" minItems:"1" doc:"Initial scan paths"`
	SkipInbox *bool    `json:"skip_inbox,omitempty" doc:"Skip inbox workflow for new books (default: false)"`
}

// SetupLibraryOutput wraps the response for Huma.
type SetupLibraryOutput struct {
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

func (s *Server) handleGetLibraryStatus(ctx context.Context, _ *LibraryStatusInput) (*LibraryStatusOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Check if user is admin
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	isAdmin := user.IsAdmin()

	library, err := s.store.GetDefaultLibrary(ctx)
	if err != nil {
		// No library exists
		return &LibraryStatusOutput{
			Body: LibraryStatusResponse{
				Exists:     false,
				NeedsSetup: isAdmin, // Only admins can set up
				BookCount:  0,
			},
		}, nil
	}

	// Get book count
	bookCount, err := s.store.CountBooks(ctx)
	if err != nil {
		bookCount = 0
	}

	isScanning := s.sseManager.IsScanning()

	return &LibraryStatusOutput{
		Body: LibraryStatusResponse{
			Exists: true,
			Library: &LibraryResponse{
				ID:         library.ID,
				Name:       library.Name,
				OwnerID:    library.OwnerID,
				ScanPaths:  library.ScanPaths,
				SkipInbox:  library.SkipInbox,
				AccessMode: string(library.GetAccessMode()),
				CreatedAt:  library.CreatedAt,
				UpdatedAt:  library.UpdatedAt,
			},
			NeedsSetup: false,
			BookCount:  bookCount,
			IsScanning: isScanning,
		},
	}, nil
}

func (s *Server) handleSetupLibrary(ctx context.Context, input *SetupLibraryInput) (*SetupLibraryOutput, error) {
	adminID, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Check if library already exists
	if existing, err := s.store.GetDefaultLibrary(ctx); err == nil && existing != nil {
		return nil, huma.Error409Conflict("library already exists")
	}

	// Validate all paths exist and are accessible
	for _, path := range input.Body.ScanPaths {
		cleanPath := filepath.Clean(path)
		if !filepath.IsAbs(cleanPath) {
			return nil, huma.Error400BadRequest("scan paths must be absolute: " + path)
		}

		info, err := os.Stat(cleanPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, huma.Error400BadRequest("path does not exist: " + path)
			}
			return nil, huma.Error400BadRequest("cannot access path: " + path)
		}

		if !info.IsDir() {
			return nil, huma.Error400BadRequest("path is not a directory: " + path)
		}
	}

	// Create library
	name := input.Body.Name
	if name == "" {
		name = "My Library"
	}

	result, err := s.store.EnsureLibrary(ctx, input.Body.ScanPaths[0], adminID)
	if err != nil {
		s.logger.Error("failed to create library", "error", err)
		return nil, huma.Error500InternalServerError("failed to create library")
	}

	library := result.Library
	library.Name = name
	if input.Body.SkipInbox != nil {
		library.SkipInbox = *input.Body.SkipInbox
	}

	// Add remaining scan paths
	for _, path := range input.Body.ScanPaths[1:] {
		library.AddScanPath(filepath.Clean(path))
	}

	library.UpdatedAt = time.Now()
	if err := s.store.UpdateLibrary(ctx, library); err != nil {
		s.logger.Error("failed to update library", "library_id", library.ID, "error", err)
		return nil, huma.Error500InternalServerError("failed to update library")
	}

	// Set scanning state IMMEDIATELY before launching async scan.
	// This ensures getLibraryStatus() returns isScanning=true right away,
	// even if the goroutine hasn't started executing yet.
	s.sseManager.SetScanning(true)

	// Trigger initial scan asynchronously (once per library, not per path)
	go func() {
		// Clear scanning state when goroutine exits (success or failure)
		defer s.sseManager.SetScanning(false)

		if s.services != nil && s.services.Book != nil {
			if _, err := s.services.Book.TriggerScan(context.Background(), library.ID, scanner.ScanOptions{}); err != nil {
				s.logger.Error("initial scan failed", "library_id", library.ID, "error", err)
			}
		}
	}()

	return &SetupLibraryOutput{
		Body: LibraryResponse{
			ID:         library.ID,
			Name:       library.Name,
			OwnerID:    library.OwnerID,
			ScanPaths:  library.ScanPaths,
			SkipInbox:  library.SkipInbox,
			AccessMode: string(library.GetAccessMode()),
			CreatedAt:  library.CreatedAt,
			UpdatedAt:  library.UpdatedAt,
		},
	}, nil
}

// === Scan Path Management DTOs ===

// ScanPathInput wraps the request for adding/removing scan paths.
type ScanPathInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Library ID"`
	Body          ScanPathRequest
}

// ScanPathRequest is the body for adding or removing a scan path.
type ScanPathRequest struct {
	Path string `json:"path" validate:"required" doc:"Absolute filesystem path"`
}

// TriggerScanInput wraps the request for triggering a manual scan.
type TriggerScanInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Library ID"`
}

// TriggerScanResponse indicates a scan was started.
type TriggerScanResponse struct {
	Message string `json:"message" doc:"Status message"`
}

// TriggerScanOutput wraps the response for Huma.
type TriggerScanOutput struct {
	Body TriggerScanResponse
}

// === Scan Path Handlers ===

func (s *Server) handleAddScanPath(ctx context.Context, input *ScanPathInput) (*LibraryOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	cleanPath := filepath.Clean(input.Body.Path)
	if !filepath.IsAbs(cleanPath) {
		return nil, huma.Error400BadRequest("path must be absolute")
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, huma.Error400BadRequest("path does not exist: " + cleanPath)
		}
		return nil, huma.Error400BadRequest("cannot access path: " + cleanPath)
	}
	if !info.IsDir() {
		return nil, huma.Error400BadRequest("path is not a directory: " + cleanPath)
	}

	lib, err := s.store.GetLibrary(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	lib.AddScanPath(cleanPath)
	lib.UpdatedAt = time.Now()

	if err := s.store.UpdateLibrary(ctx, lib); err != nil {
		return nil, huma.Error500InternalServerError("failed to update library")
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

func (s *Server) handleRemoveScanPath(ctx context.Context, input *ScanPathInput) (*LibraryOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	lib, err := s.store.GetLibrary(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	if len(lib.ScanPaths) <= 1 {
		return nil, huma.Error400BadRequest("cannot remove the last scan path")
	}

	cleanPath := filepath.Clean(input.Body.Path)
	lib.RemoveScanPath(cleanPath)
	lib.UpdatedAt = time.Now()

	if err := s.store.UpdateLibrary(ctx, lib); err != nil {
		return nil, huma.Error500InternalServerError("failed to update library")
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

func (s *Server) handleTriggerScan(ctx context.Context, input *TriggerScanInput) (*TriggerScanOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	// Verify library exists
	if _, err := s.store.GetLibrary(ctx, input.ID); err != nil {
		return nil, err
	}

	s.sseManager.SetScanning(true)

	go func() {
		defer s.sseManager.SetScanning(false)

		if s.services != nil && s.services.Book != nil {
			if _, err := s.services.Book.TriggerScan(context.Background(), input.ID, scanner.ScanOptions{}); err != nil {
				s.logger.Error("manual scan failed", "library_id", input.ID, "error", err)
			}
		}
	}()

	return &TriggerScanOutput{
		Body: TriggerScanResponse{Message: "Scan started"},
	}, nil
}
