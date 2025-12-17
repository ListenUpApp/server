package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleListSeries returns a paginated list of series.
// Note: Series visibility is not filtered by ACL yet - returns all series.
// TODO: Filter to only show series with at least one accessible book.
func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	params := parsePaginationParams(r)

	series, err := s.services.Sync.GetSeriesForSync(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to list series", "error", err, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series", s.logger)
		return
	}

	response.Success(w, series, s.logger)
}

// handleGetSeries returns a single series by ID.
// Note: Series visibility is not filtered by ACL yet.
// TODO: Return 404 if user has no access to any books in this series.
func (s *Server) handleGetSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	series, err := s.store.GetSeries(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrSeriesNotFound) {
			response.NotFound(w, "Series not found", s.logger)
			return
		}
		s.logger.Error("Failed to get series", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series", s.logger)
		return
	}

	response.Success(w, series, s.logger)
}

// handleGetSeriesBooks returns all books in a series that the user can access.
func (s *Server) handleGetSeriesBooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	// Get all books in the series
	allBooks, err := s.store.GetBooksBySeries(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get series books", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series books", s.logger)
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
		"series_id": id,
		"books":     accessibleBooks,
	}, s.logger)
}

// handleSyncSeries handles GET /api/v1/sync/series for syncing series.
func (s *Server) handleSyncSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	params := parsePaginationParams(r)

	series, err := s.services.Sync.GetSeriesForSync(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to sync series", "error", err)
		response.InternalError(w, "Failed to sync series", s.logger)
		return
	}

	response.Success(w, series, s.logger)
}

// SeriesUpdateRequest is the request body for PATCH /api/v1/series/{id}.
// Uses pointer fields for PATCH semantics - only provided fields are updated.
// NOTE: omitempty is NOT used here because we need to distinguish between
// "not provided" (nil) and "set to empty string" (pointer to "").
type SeriesUpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// handleUpdateSeries updates a series's metadata.
// PATCH /api/v1/series/{id}
//
// Allows updating: name, description.
// Uses PATCH semantics - only provided fields are updated.
// Cover image is handled separately via upload endpoint.
func (s *Server) handleUpdateSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	id := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if id == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	var req SeriesUpdateRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// Get existing series
	series, err := s.store.GetSeries(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrSeriesNotFound) {
			response.NotFound(w, "Series not found", s.logger)
			return
		}
		s.logger.Error("Failed to get series", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series", s.logger)
		return
	}

	// Apply non-nil fields (PATCH semantics)
	if req.Name != nil {
		if *req.Name == "" {
			response.BadRequest(w, "name cannot be empty", s.logger)
			return
		}
		series.Name = *req.Name
	}

	if req.Description != nil {
		series.Description = *req.Description
	}

	// Update timestamp
	series.UpdatedAt = time.Now()

	// Save updated series
	if err := s.store.UpdateSeries(ctx, series); err != nil {
		s.logger.Error("Failed to update series", "error", err, "id", id, "user_id", userID)
		response.InternalError(w, "Failed to update series", s.logger)
		return
	}

	s.logger.Info("Series updated",
		"series_id", id,
		"user_id", userID,
	)

	response.Success(w, series, s.logger)
}

// handleUploadSeriesCover handles cover image uploads for a series.
// PUT /api/v1/series/{id}/cover.
// Content-Type: multipart/form-data with "file" field.
func (s *Server) handleUploadSeriesCover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	seriesID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if seriesID == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	// Get the series to verify it exists
	series, err := s.store.GetSeries(ctx, seriesID)
	if err != nil {
		if errors.Is(err, store.ErrSeriesNotFound) {
			response.NotFound(w, "Series not found", s.logger)
			return
		}
		s.logger.Error("Failed to get series for cover upload", "error", err, "series_id", seriesID, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series", s.logger)
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
		s.logger.Error("Failed to read uploaded file", "error", err, "series_id", seriesID)
		response.InternalError(w, "Failed to read uploaded file", s.logger)
		return
	}

	// Validate it's an image by checking magic bytes
	contentType := detectImageType(imgData)
	if contentType == "" {
		response.BadRequest(w, "Invalid image format. Supported formats: JPEG, PNG, WebP, GIF", s.logger)
		return
	}

	// Save the cover image (overwrites any existing cover)
	if err := s.storage.SeriesCovers.Save(seriesID, imgData); err != nil {
		s.logger.Error("Failed to save series cover image", "error", err, "series_id", seriesID)
		response.InternalError(w, "Failed to save cover image", s.logger)
		return
	}

	// Update series's CoverImage field to mark it has a custom cover
	series.CoverImage = &domain.ImageFileInfo{
		Filename: header.Filename,
		Format:   contentType,
		Size:     header.Size,
	}

	// Update timestamp
	series.UpdatedAt = time.Now()

	// Save series update
	if err := s.store.UpdateSeries(ctx, series); err != nil {
		s.logger.Error("Failed to update series after cover upload", "error", err, "series_id", seriesID)
		response.InternalError(w, "Failed to update series", s.logger)
		return
	}

	s.logger.Info("Series cover uploaded successfully",
		"series_id", seriesID,
		"filename", header.Filename,
		"size", header.Size,
		"format", contentType,
	)

	response.Success(w, map[string]string{
		"cover_url": "/api/v1/series/" + seriesID + "/cover",
	}, s.logger)
}

// handleGetSeriesCover serves cover images for series.
// GET /api/v1/series/{id}/cover.
func (s *Server) handleGetSeriesCover(w http.ResponseWriter, r *http.Request) {
	seriesID := chi.URLParam(r, "id")
	if seriesID == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	// Check if cover exists
	if !s.storage.SeriesCovers.Exists(seriesID) {
		response.NotFound(w, "Cover not found for this series", s.logger)
		return
	}

	// Get cover file info for Last-Modified header
	coverPath := s.storage.SeriesCovers.Path(seriesID)
	fileInfo, err := os.Stat(coverPath)
	if err != nil {
		s.logger.Error("Failed to stat series cover file", "series_id", seriesID, "error", err)
		response.InternalError(w, "Failed to retrieve cover", s.logger)
		return
	}

	// Compute ETag from hash
	hash, err := s.storage.SeriesCovers.Hash(seriesID)
	if err != nil {
		s.logger.Error("Failed to compute series cover hash", "series_id", seriesID, "error", err)
		response.InternalError(w, "Failed to retrieve cover", s.logger)
		return
	}
	etag := `"` + hash + `"`

	// Check If-None-Match for cache validation
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Get cover data
	data, err := s.storage.SeriesCovers.Get(seriesID)
	if err != nil {
		s.logger.Error("Failed to read series cover file", "series_id", seriesID, "error", err)
		response.InternalError(w, "Failed to retrieve cover", s.logger)
		return
	}

	// Set caching headers
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=604800") // 1 week
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))

	// Write image data
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		s.logger.Error("Failed to write series cover response", "series_id", seriesID, "error", err)
	}
}

// handleDeleteSeriesCover deletes a series cover image.
// DELETE /api/v1/series/{id}/cover.
func (s *Server) handleDeleteSeriesCover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	seriesID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if seriesID == "" {
		response.BadRequest(w, "Series ID is required", s.logger)
		return
	}

	// Get the series to verify it exists
	series, err := s.store.GetSeries(ctx, seriesID)
	if err != nil {
		if errors.Is(err, store.ErrSeriesNotFound) {
			response.NotFound(w, "Series not found", s.logger)
			return
		}
		s.logger.Error("Failed to get series for cover deletion", "error", err, "series_id", seriesID, "user_id", userID)
		response.InternalError(w, "Failed to retrieve series", s.logger)
		return
	}

	// Delete the cover image from storage
	if err := s.storage.SeriesCovers.Delete(seriesID); err != nil {
		s.logger.Error("Failed to delete series cover", "error", err, "series_id", seriesID)
		response.InternalError(w, "Failed to delete cover", s.logger)
		return
	}

	// Clear series's CoverImage field
	series.CoverImage = nil

	// Update timestamp
	series.UpdatedAt = time.Now()

	// Save series update
	if err := s.store.UpdateSeries(ctx, series); err != nil {
		s.logger.Error("Failed to update series after cover deletion", "error", err, "series_id", seriesID)
		response.InternalError(w, "Failed to update series", s.logger)
		return
	}

	s.logger.Info("Series cover deleted successfully",
		"series_id", seriesID,
		"user_id", userID,
	)

	w.WriteHeader(http.StatusNoContent)
}
