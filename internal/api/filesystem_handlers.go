package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerFilesystemRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "browseFilesystem",
		Method:      http.MethodGet,
		Path:        "/api/v1/filesystem",
		Summary:     "Browse filesystem directories",
		Description: "Returns directories at the specified path. Admin only.",
		Tags:        []string{"Filesystem"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleBrowseFilesystem)
}

// === DTOs ===

// BrowseFilesystemInput contains parameters for browsing the filesystem.
type BrowseFilesystemInput struct {
	Authorization string `header:"Authorization"`
	Path          string `query:"path" default:"/" doc:"Directory path to browse"`
}

// DirectoryEntry represents a single directory in the filesystem.
type DirectoryEntry struct {
	Name string `json:"name" doc:"Directory name"`
	Path string `json:"path" doc:"Full path to directory"`
}

// BrowseFilesystemResponse contains the directory listing.
type BrowseFilesystemResponse struct {
	Path    string           `json:"path" doc:"Current path"`
	Parent  string           `json:"parent,omitempty" doc:"Parent directory path"`
	Entries []DirectoryEntry `json:"entries" doc:"Directories in this path"`
	IsRoot  bool             `json:"is_root" doc:"Whether this is the filesystem root"`
}

// BrowseFilesystemOutput wraps the response for Huma.
type BrowseFilesystemOutput struct {
	Body BrowseFilesystemResponse
}

// === Handler ===

func (s *Server) handleBrowseFilesystem(ctx context.Context, input *BrowseFilesystemInput) (*BrowseFilesystemOutput, error) {
	// Admin only
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	// Clean and validate path
	path := filepath.Clean(input.Path)
	if path == "" {
		path = "/"
	}

	// Security: Ensure path is absolute
	if !filepath.IsAbs(path) {
		path = "/" + path
	}

	// Check if path exists and is a directory
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, huma.Error404NotFound("directory not found")
		}
		if os.IsPermission(err) {
			return nil, huma.Error403Forbidden("permission denied")
		}
		return nil, huma.Error500InternalServerError("failed to access path")
	}

	if !info.IsDir() {
		return nil, huma.Error400BadRequest("path is not a directory")
	}

	// Read directory entries
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil, huma.Error403Forbidden("permission denied reading directory")
		}
		return nil, huma.Error500InternalServerError("failed to read directory")
	}

	// Filter to directories only, exclude hidden and system dirs
	entries := make([]DirectoryEntry, 0)
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip hidden directories
		if strings.HasPrefix(name, ".") {
			continue
		}

		// Skip system directories
		if isSystemDirectory(name) {
			continue
		}

		fullPath := filepath.Join(path, name)

		entries = append(entries, DirectoryEntry{
			Name: name,
			Path: fullPath,
		})
	}

	// Sort alphabetically (case-insensitive)
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	// Determine parent
	parent := ""
	isRoot := path == "/"
	if !isRoot {
		parent = filepath.Dir(path)
	}

	return &BrowseFilesystemOutput{
		Body: BrowseFilesystemResponse{
			Path:    path,
			Parent:  parent,
			Entries: entries,
			IsRoot:  isRoot,
		},
	}, nil
}

// isSystemDirectory returns true for directories that should be hidden.
func isSystemDirectory(name string) bool {
	systemDirs := map[string]bool{
		"proc":       true,
		"sys":        true,
		"dev":        true,
		"run":        true,
		"snap":       true,
		"lost+found": true,
		"boot":       true,
		"lib":        true,
		"lib32":      true,
		"lib64":      true,
		"libx32":     true,
		"sbin":       true,
		"bin":        true,
		"usr":        true,
		"etc":        true,
		"var":        true,
		"tmp":        true,
		"root":       true,
	}
	return systemDirs[name]
}

