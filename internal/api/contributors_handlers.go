package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleListContributors returns a paginated list of contributors.
// Note: Contributor visibility is not filtered by ACL yet - returns all contributors.
// TODO: Filter to only show contributors with at least one accessible book.
func (s *Server) handleListContributors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	params := parsePaginationParams(r)

	contributors, err := s.services.Sync.GetContributorsForSync(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to list contributors", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to retrieve contributors", s.logger)
		return
	}

	response.Success(w, contributors, s.logger)
}

// handleGetContributor returns a single contributor by ID.
// Note: Contributor visibility is not filtered by ACL yet.
// TODO: Return 404 if user has no access to any books by this contributor.
func (s *Server) handleGetContributor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	contributor, err := s.store.GetContributor(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrContributorNotFound) {
			response.NotFound(w, "Contributor not found", s.logger)
			return
		}
		s.logger.Error("Failed to get contributor", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve contributor", s.logger)
		return
	}

	response.Success(w, contributor, s.logger)
}

// handleGetContributorBooks returns all books by a contributor that the user can access.
func (s *Server) handleGetContributorBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	// Get all books by the contributor
	allBooks, err := s.store.GetBooksByContributor(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get contributor books", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve contributor books", s.logger)
		return
	}

	// Filter to only books the user can access
	accessibleBooks := make([]*domain.Book, 0, len(allBooks))
	for _, book := range allBooks {
		canAccess, err := s.store.CanUserAccessBook(ctx, userID, book.ID)
		if err != nil {
			s.logger.Warn("Failed to check book access", "book_id", book.ID, "user_id", userID, "error", err)
			continue
		}
		if canAccess {
			accessibleBooks = append(accessibleBooks, book)
		}
	}

	response.Success(w, map[string]interface{}{
		"contributor_id": id,
		"books":          accessibleBooks,
	}, s.logger)
}

// handleSearchContributors searches for contributors by name.
// Used for autocomplete when editing book contributors.
// GET /api/v1/contributors/search?q=steven&limit=10
//
// Uses Bleve full-text search for O(log n) performance with:
// - Prefix matching ("bran" → "Brandon Sanderson")
// - Word matching ("sanderson" in "Brandon Sanderson")
// - Fuzzy matching for typo tolerance
func (s *Server) handleSearchContributors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		response.BadRequest(w, "Query parameter 'q' is required", s.logger)
		return
	}

	// Parse limit parameter (default 10, max 50)
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
			if limit > 50 {
				limit = 50
			}
		}
	}

	// Use Bleve search for O(log n) performance instead of O(n) BadgerDB scan
	contributors, err := s.services.Search.SearchContributors(ctx, query, limit)
	if err != nil {
		s.logger.Error("Failed to search contributors", "error", err, "query", query, "user_id", userID)
		response.InternalError(w, "Failed to search contributors", s.logger)
		return
	}

	response.Success(w, map[string]interface{}{
		"contributors": contributors,
	}, s.logger)
}

// handleSyncContributors handles GET /api/v1/sync/contributors for syncing contributors.
func (s *Server) handleSyncContributors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	params := parsePaginationParams(r)

	contributors, err := s.services.Sync.GetContributorsForSync(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to sync contributors", "error", err)
		response.InternalError(w, "Failed to sync contributors", s.logger)
		return
	}

	response.Success(w, contributors, s.logger)
}

// MergeContributorsRequest is the request body for POST /api/v1/contributors/{id}/merge.
type MergeContributorsRequest struct {
	SourceContributorID string `json:"source_contributor_id"`
}

// handleMergeContributors merges a source contributor into a target contributor.
// POST /api/v1/contributors/{id}/merge
//
// This is used when a user identifies that two contributors are actually the same person
// (e.g., "Richard Bachman" is a pen name for "Stephen King").
//
// The merge operation:
//   - Re-links all books from source to target, preserving original attribution via CreditedAs
//   - Adds source's name to target's Aliases field
//   - Soft-deletes the source contributor
//
// After merge, future book scans for the source name will automatically link to the target.
func (s *Server) handleMergeContributors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	targetID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if targetID == "" {
		response.BadRequest(w, "Target contributor ID is required", s.logger)
		return
	}

	var req MergeContributorsRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.SourceContributorID == "" {
		response.BadRequest(w, "source_contributor_id is required", s.logger)
		return
	}

	if req.SourceContributorID == targetID {
		response.BadRequest(w, "Cannot merge contributor into itself", s.logger)
		return
	}

	// Perform the merge
	merged, err := s.store.MergeContributors(ctx, req.SourceContributorID, targetID)
	if err != nil {
		if errors.Is(err, store.ErrContributorNotFound) {
			response.NotFound(w, "Contributor not found", s.logger)
			return
		}
		s.logger.Error("Failed to merge contributors",
			"error", err,
			"source_id", req.SourceContributorID,
			"target_id", targetID,
			"user_id", userID,
		)
		response.InternalError(w, "Failed to merge contributors", s.logger)
		return
	}

	s.logger.Info("Contributors merged",
		"source_id", req.SourceContributorID,
		"target_id", targetID,
		"user_id", userID,
	)

	response.Success(w, merged, s.logger)
}

// UnmergeContributorRequest is the request body for POST /api/v1/contributors/{id}/unmerge.
type UnmergeContributorRequest struct {
	AliasName string `json:"alias_name"`
}

// handleUnmergeContributor splits an alias back into a separate contributor.
// POST /api/v1/contributors/{id}/unmerge
//
// This is the reverse of merge - when a user decides that an alias should be
// a separate contributor after all.
//
// The unmerge operation:
//   - Creates a new contributor with the alias name
//   - Re-links books that were credited to that alias to the new contributor
//   - Removes the alias from the source contributor
//
// Returns the newly created contributor.
func (s *Server) handleUnmergeContributor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	sourceID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if sourceID == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	var req UnmergeContributorRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.AliasName == "" {
		response.BadRequest(w, "alias_name is required", s.logger)
		return
	}

	// Perform the unmerge
	newContributor, err := s.store.UnmergeContributor(ctx, sourceID, req.AliasName)
	if err != nil {
		if errors.Is(err, store.ErrContributorNotFound) {
			response.NotFound(w, "Contributor not found", s.logger)
			return
		}
		s.logger.Error("Failed to unmerge contributor",
			"error", err,
			"source_id", sourceID,
			"alias_name", req.AliasName,
			"user_id", userID,
		)
		response.InternalError(w, "Failed to unmerge contributor", s.logger)
		return
	}

	s.logger.Info("Contributor unmerged",
		"source_id", sourceID,
		"alias_name", req.AliasName,
		"new_contributor_id", newContributor.ID,
		"user_id", userID,
	)

	response.Success(w, newContributor, s.logger)
}

// UpdateContributorRequest is the request body for PUT /api/v1/contributors/{id}.
type UpdateContributorRequest struct {
	Name      string   `json:"name"`
	Biography string   `json:"biography,omitempty"`
	Website   string   `json:"website,omitempty"`
	BirthDate string   `json:"birth_date,omitempty"`
	DeathDate string   `json:"death_date,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
}

// handleUpdateContributor updates a contributor's metadata.
// PUT /api/v1/contributors/{id}
//
// Allows updating: name, biography, website, birth_date, death_date, aliases.
// Image is handled separately via upload endpoint.
func (s *Server) handleUpdateContributor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	var req UpdateContributorRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.Name == "" {
		response.BadRequest(w, "name is required", s.logger)
		return
	}

	// Get existing contributor
	contributor, err := s.store.GetContributor(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrContributorNotFound) {
			response.NotFound(w, "Contributor not found", s.logger)
			return
		}
		s.logger.Error("Failed to get contributor", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve contributor", s.logger)
		return
	}

	// Update fields
	contributor.Name = req.Name
	contributor.Biography = req.Biography
	contributor.Website = req.Website
	contributor.BirthDate = req.BirthDate
	contributor.DeathDate = req.DeathDate
	contributor.Aliases = req.Aliases

	// Save updated contributor
	if err := s.store.UpdateContributor(ctx, contributor); err != nil {
		s.logger.Error("Failed to update contributor", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to update contributor", s.logger)
		return
	}

	s.logger.Info("Contributor updated",
		"contributor_id", id,
		"name", req.Name,
		"user_id", userID,
	)

	response.Success(w, contributor, s.logger)
}

// handleUploadContributorImage handles profile photo uploads for a contributor.
// PUT /api/v1/contributors/{id}/image
// Content-Type: multipart/form-data with "file" field
func (s *Server) handleUploadContributorImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	contributorID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if contributorID == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	// Validate contributor exists
	contributor, err := s.store.GetContributor(ctx, contributorID)
	if err != nil {
		if errors.Is(err, store.ErrContributorNotFound) {
			response.NotFound(w, "Contributor not found", s.logger)
			return
		}
		s.logger.Error("Failed to get contributor for image upload", "error", err, "contributor_id", contributorID, "user_id", userID)
		response.InternalError(w, "Failed to retrieve contributor", s.logger)
		return
	}

	// Parse multipart form (limit to 10MB)
	const maxUploadSize = 10 << 20 // 10MB
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		response.BadRequest(w, "Failed to parse form data", s.logger)
		return
	}

	// Get the uploaded file
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, "No file uploaded. Use 'file' field in multipart form", s.logger)
		return
	}
	defer file.Close()

	// Validate file size
	if header.Size > maxUploadSize {
		response.BadRequest(w, "File too large. Maximum size is 10MB", s.logger)
		return
	}

	// Read file content
	imgData := make([]byte, header.Size)
	if _, err := file.Read(imgData); err != nil {
		s.logger.Error("Failed to read uploaded file", "error", err, "contributor_id", contributorID)
		response.InternalError(w, "Failed to read uploaded file", s.logger)
		return
	}

	// Validate it's an image by checking magic bytes
	contentType := detectImageType(imgData)
	if contentType == "" {
		response.BadRequest(w, "Invalid image format. Supported formats: JPEG, PNG, WebP, GIF", s.logger)
		return
	}

	// Save the image (overwrites any existing image)
	if err := s.storage.ContributorImages.Save(contributorID, imgData); err != nil {
		s.logger.Error("Failed to save contributor image", "error", err, "contributor_id", contributorID)
		response.InternalError(w, "Failed to save image", s.logger)
		return
	}

	// Update contributor's ImageURL field
	contributor.ImageURL = "/api/v1/contributors/" + contributorID + "/image"

	// Save contributor update (this emits SSE event)
	if err := s.store.UpdateContributor(ctx, contributor); err != nil {
		s.logger.Error("Failed to update contributor after image upload", "error", err, "contributor_id", contributorID)
		response.InternalError(w, "Failed to update contributor", s.logger)
		return
	}

	s.logger.Info("Contributor image uploaded successfully",
		"contributor_id", contributorID,
		"filename", header.Filename,
		"size", header.Size,
		"format", contentType,
	)

	response.Success(w, map[string]string{
		"image_url": contributor.ImageURL,
	}, s.logger)
}

// handleGetContributorImage serves a contributor's profile photo.
// GET /api/v1/contributors/{id}/image
func (s *Server) handleGetContributorImage(w http.ResponseWriter, r *http.Request) {
	contributorID := chi.URLParam(r, "id")

	if contributorID == "" {
		response.BadRequest(w, "Contributor ID is required", s.logger)
		return
	}

	// Check if image exists
	if !s.storage.ContributorImages.Exists(contributorID) {
		response.NotFound(w, "Image not found for this contributor", s.logger)
		return
	}

	// Get image file info for Last-Modified header
	imagePath := s.storage.ContributorImages.Path(contributorID)
	fileInfo, err := os.Stat(imagePath)
	if err != nil {
		s.logger.Error("Failed to stat contributor image file", "contributor_id", contributorID, "error", err)
		response.InternalError(w, "Failed to retrieve image", s.logger)
		return
	}

	// Compute ETag from hash
	hash, err := s.storage.ContributorImages.Hash(contributorID)
	if err != nil {
		s.logger.Error("Failed to compute contributor image hash", "contributor_id", contributorID, "error", err)
		response.InternalError(w, "Failed to retrieve image", s.logger)
		return
	}
	etag := `"` + hash[:16] + `"` // Truncate to reasonable length

	// Handle conditional GET (If-None-Match)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Get image data
	data, err := s.storage.ContributorImages.Get(contributorID)
	if err != nil {
		s.logger.Error("Failed to read contributor image file", "contributor_id", contributorID, "error", err)
		response.InternalError(w, "Failed to retrieve image", s.logger)
		return
	}

	// Detect content type from the actual file
	contentType := detectImageType(data)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Set cache headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "public, max-age=86400") // 24 hours

	// Write image data
	if _, err := w.Write(data); err != nil {
		s.logger.Error("Failed to write contributor image response", "contributor_id", contributorID, "error", err)
	}
}
