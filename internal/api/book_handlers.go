package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// BookUpdateRequest contains fields that can be updated on a book.
// Only non-nil fields are applied (true PATCH semantics).
// Note: omitempty is intentionally not used here - we need to distinguish between
// "field not provided" (nil pointer) and "field set to empty" (pointer to "").
// Note: Series is managed via PUT /api/v1/books/{id}/series endpoint.
type BookUpdateRequest struct {
	Title       *string `json:"title"`
	Subtitle    *string `json:"subtitle"`
	Description *string `json:"description"`
	Publisher   *string `json:"publisher"`
	PublishYear *string `json:"publish_year"`
	Language    *string `json:"language"`
	ISBN        *string `json:"isbn"`
	ASIN        *string `json:"asin"`
	Abridged    *bool   `json:"abridged"`
}

// handleUpdateBook updates a book's metadata (PATCH semantics).
// Only fields present in the request body are updated.
func (s *Server) handleUpdateBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	var req BookUpdateRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// Get the book with ACL check - ensures user has access.
	book, err := s.bookService.GetBook(ctx, userID, bookID)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to get book for update", "error", err, "book_id", bookID, "user_id", userID)
		response.InternalError(w, "Failed to retrieve book", s.logger)
		return
	}

	// Apply only the fields that were provided (PATCH semantics).
	if req.Title != nil {
		book.Title = *req.Title
	}
	if req.Subtitle != nil {
		book.Subtitle = *req.Subtitle
	}
	if req.Description != nil {
		book.Description = *req.Description
	}
	if req.Publisher != nil {
		book.Publisher = *req.Publisher
	}
	if req.PublishYear != nil {
		book.PublishYear = *req.PublishYear
	}
	if req.Language != nil {
		book.Language = *req.Language
	}
	if req.ISBN != nil {
		book.ISBN = *req.ISBN
	}
	if req.ASIN != nil {
		book.ASIN = *req.ASIN
	}
	if req.Abridged != nil {
		book.Abridged = *req.Abridged
	}
	// Note: Series is managed via PUT /api/v1/books/{id}/series endpoint

	// Update the book - store.UpdateBook handles SSE events and search reindexing.
	if err := s.store.UpdateBook(ctx, book); err != nil {
		s.logger.Error("Failed to update book", "error", err, "book_id", bookID, "user_id", userID)
		response.InternalError(w, "Failed to update book", s.logger)
		return
	}

	// Return the enriched book (with denormalized contributor names, series name, etc.).
	enrichedBook, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		// Log but don't fail - return the raw book if enrichment fails.
		s.logger.Warn("Failed to enrich book after update", "error", err, "book_id", bookID)
		response.Success(w, book, s.logger)
		return
	}

	response.Success(w, enrichedBook, s.logger)
}

// ContributorInput represents a contributor in a SetContributorsRequest.
type ContributorInput struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

// SetContributorsRequest is the request body for PUT /api/v1/books/{id}/contributors.
type SetContributorsRequest struct {
	Contributors []ContributorInput `json:"contributors"`
}

// handleSetBookContributors replaces all contributors for a book.
// PUT /api/v1/books/{id}/contributors
func (s *Server) handleSetBookContributors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	var req SetContributorsRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// Validate ACL - user must have access to the book
	_, err := s.bookService.GetBook(ctx, userID, bookID)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to get book for contributor update", "error", err, "book_id", bookID, "user_id", userID)
		response.InternalError(w, "Failed to retrieve book", s.logger)
		return
	}

	// Convert and validate roles
	storeContributors := make([]store.ContributorInput, 0, len(req.Contributors))
	for _, c := range req.Contributors {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			response.BadRequest(w, "Contributor name is required", s.logger)
			return
		}

		roles := make([]domain.ContributorRole, 0, len(c.Roles))
		for _, roleStr := range c.Roles {
			role := domain.ContributorRole(roleStr)
			if !role.IsValid() {
				response.BadRequest(w, "Invalid role: "+roleStr, s.logger)
				return
			}
			roles = append(roles, role)
		}

		if len(roles) == 0 {
			response.BadRequest(w, "At least one role is required for contributor: "+name, s.logger)
			return
		}

		storeContributors = append(storeContributors, store.ContributorInput{
			Name:  name,
			Roles: roles,
		})
	}

	// Set the contributors
	book, err := s.store.SetBookContributors(ctx, bookID, storeContributors)
	if err != nil {
		s.logger.Error("Failed to set book contributors", "error", err, "book_id", bookID, "user_id", userID)
		response.InternalError(w, "Failed to update contributors", s.logger)
		return
	}

	// Return enriched book
	enrichedBook, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		s.logger.Warn("Failed to enrich book after contributor update", "error", err, "book_id", bookID)
		response.Success(w, book, s.logger)
		return
	}

	response.Success(w, enrichedBook, s.logger)
}

// SeriesInput represents a series in a SetSeriesRequest.
type SeriesInput struct {
	Name     string `json:"name"`
	Sequence string `json:"sequence"`
}

// SetSeriesRequest is the request body for PUT /api/v1/books/{id}/series.
type SetSeriesRequest struct {
	Series []SeriesInput `json:"series"`
}

// handleSetBookSeries replaces all series for a book.
// PUT /api/v1/books/{id}/series
func (s *Server) handleSetBookSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	var req SetSeriesRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// Validate ACL - user must have access to the book
	_, err := s.bookService.GetBook(ctx, userID, bookID)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to get book for series update", "error", err, "book_id", bookID, "user_id", userID)
		response.InternalError(w, "Failed to retrieve book", s.logger)
		return
	}

	// Convert to store input format
	storeSeries := make([]store.SeriesInput, 0, len(req.Series))
	for _, sr := range req.Series {
		name := strings.TrimSpace(sr.Name)
		if name == "" {
			response.BadRequest(w, "Series name is required", s.logger)
			return
		}

		storeSeries = append(storeSeries, store.SeriesInput{
			Name:     name,
			Sequence: strings.TrimSpace(sr.Sequence),
		})
	}

	// Set the series
	book, err := s.store.SetBookSeries(ctx, bookID, storeSeries)
	if err != nil {
		s.logger.Error("Failed to set book series", "error", err, "book_id", bookID, "user_id", userID)
		response.InternalError(w, "Failed to update series", s.logger)
		return
	}

	// Return enriched book
	enrichedBook, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		s.logger.Warn("Failed to enrich book after series update", "error", err, "book_id", bookID)
		response.Success(w, book, s.logger)
		return
	}

	response.Success(w, enrichedBook, s.logger)
}

// handleUploadCover handles cover image uploads for a book.
// PUT /api/v1/books/{id}/cover
// Content-Type: multipart/form-data with "file" field
func (s *Server) handleUploadCover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	// Validate ACL - user must have access to the book
	book, err := s.bookService.GetBook(ctx, userID, bookID)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to get book for cover upload", "error", err, "book_id", bookID, "user_id", userID)
		response.InternalError(w, "Failed to retrieve book", s.logger)
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
		s.logger.Error("Failed to read uploaded file", "error", err, "book_id", bookID)
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
	if err := s.coverStorage.Save(bookID, imgData); err != nil {
		s.logger.Error("Failed to save cover image", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to save cover image", s.logger)
		return
	}

	// Update book's CoverImage field to mark it has a custom cover
	book.CoverImage = &domain.ImageFileInfo{
		Filename: header.Filename,
		Format:   contentType,
		Size:     header.Size,
	}

	// Save book update
	if err := s.store.UpdateBook(ctx, book); err != nil {
		s.logger.Error("Failed to update book after cover upload", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to update book", s.logger)
		return
	}

	s.logger.Info("Cover uploaded successfully",
		"book_id", bookID,
		"filename", header.Filename,
		"size", header.Size,
		"format", contentType,
	)

	response.Success(w, map[string]string{
		"cover_url": "/api/v1/covers/" + bookID,
	}, s.logger)
}

// detectImageType checks magic bytes to determine image format.
// Returns empty string if not a recognized image format.
func detectImageType(data []byte) string {
	if len(data) < 8 {
		return ""
	}

	// JPEG: starts with FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}

	// PNG: starts with 89 50 4E 47 0D 0A 1A 0A
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}

	// GIF: starts with GIF87a or GIF89a
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "image/gif"
	}

	// WebP: starts with RIFF....WEBP
	if data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		len(data) >= 12 && data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp"
	}

	return ""
}
