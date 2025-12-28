package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerCoverRoutes() {
	// Cover routes use chi directly for streaming, not huma
	// But we still register them for OpenAPI documentation
	huma.Register(s.api, huma.Operation{
		OperationID: "getBookCover",
		Method:      http.MethodGet,
		Path:        "/api/v1/books/{id}/cover",
		Summary:     "Get book cover",
		Description: "Returns the cover image for a book",
		Tags:        []string{"Covers"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetBookCover)

	huma.Register(s.api, huma.Operation{
		OperationID: "uploadBookCover",
		Method:      http.MethodPost,
		Path:        "/api/v1/books/{id}/cover",
		Summary:     "Upload book cover",
		Description: "Uploads a cover image for a book",
		Tags:        []string{"Covers"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUploadBookCoverHuma)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteBookCover",
		Method:      http.MethodDelete,
		Path:        "/api/v1/books/{id}/cover",
		Summary:     "Delete book cover",
		Description: "Deletes the cover image for a book",
		Tags:        []string{"Covers"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteBookCover)

	// Direct chi routes for cover streaming (used by mobile clients)
	s.router.Get("/covers/{path}", s.handleServeCover)

	// Direct cover access by book ID (used by mobile clients for sync)
	s.router.Get("/api/v1/covers/{id}", s.handleServeCoverByBookID)

	// Batch cover download (used by mobile clients for sync)
	s.router.Get("/api/v1/covers/batch", s.handleServeCoverBatch)
}

// === DTOs ===

type GetBookCoverInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

type CoverRedirectOutput struct {
	Status   int
	Location string `header:"Location"`
}

func (o *CoverRedirectOutput) StatusCode() int {
	return o.Status
}

type UploadBookCoverInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
	RawBody       []byte
}

type CoverResponse struct {
	Path     string `json:"path" doc:"Cover path"`
	BlurHash string `json:"blur_hash,omitempty" doc:"BlurHash string"`
}

type CoverOutput struct {
	Body CoverResponse
}

type DeleteBookCoverInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Book ID"`
}

// === Handlers ===

func (s *Server) handleGetBookCover(ctx context.Context, input *GetBookCoverInput) (*CoverRedirectOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	book, err := s.store.GetBook(ctx, input.ID, userID)
	if err != nil {
		return nil, err
	}

	if book.CoverImage == nil || book.CoverImage.Path == "" {
		return nil, huma.Error404NotFound("Book has no cover")
	}

	return &CoverRedirectOutput{
		Status:   http.StatusTemporaryRedirect,
		Location: "/covers/" + book.CoverImage.Path,
	}, nil
}

func (s *Server) handleUploadBookCoverHuma(ctx context.Context, input *UploadBookCoverInput) (*CoverOutput, error) {
	if _, err := s.authenticateRequest(ctx, input.Authorization); err != nil {
		return nil, err
	}

	// This is a simplified version - the actual implementation would handle file upload
	return nil, huma.Error501NotImplemented("Use multipart form upload endpoint")
}

func (s *Server) handleDeleteBookCover(ctx context.Context, input *DeleteBookCoverInput) (*MessageOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	book, err := s.store.GetBook(ctx, input.ID, userID)
	if err != nil {
		return nil, err
	}

	if book.CoverImage != nil && book.CoverImage.Path != "" {
		// Delete from storage - use book ID as the image ID
		if err := s.storage.Covers.Delete(input.ID); err != nil {
			s.logger.Warn("failed to delete cover file", "book_id", input.ID, "error", err)
		}
	}

	book.CoverImage = nil
	book.Touch()

	if err := s.store.UpdateBook(ctx, book); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Cover deleted"}}, nil
}

func (s *Server) handleServeCover(w http.ResponseWriter, r *http.Request) {
	// The path is the cover ID (book ID without extension)
	id := chi.URLParam(r, "path")
	if id == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	// Remove .jpg extension if present
	if len(id) > 4 && id[len(id)-4:] == ".jpg" {
		id = id[:len(id)-4]
	}

	data, err := s.storage.Covers.Get(id)
	if err != nil {
		http.Error(w, "cover not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
}

// handleServeCoverByBookID serves a cover directly by book ID.
// Used by mobile clients for sync operations.
// GET /api/v1/covers/{id}
func (s *Server) handleServeCoverByBookID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}

	data, err := s.storage.Covers.Get(id)
	if err != nil {
		http.Error(w, "cover not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
}

// handleServeCoverBatch serves multiple covers as a TAR archive.
// Used by mobile clients for efficient batch sync.
// GET /api/v1/covers/batch?ids=book1,book2,book3
func (s *Server) handleServeCoverBatch(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		http.Error(w, "ids query parameter required", http.StatusBadRequest)
		return
	}

	ids := splitIDs(idsParam)
	if len(ids) == 0 {
		http.Error(w, "no valid ids provided", http.StatusBadRequest)
		return
	}

	// Limit to 100 covers per request
	if len(ids) > 100 {
		ids = ids[:100]
	}

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Cache-Control", "no-cache")

	// Write TAR entries for each available cover
	for _, id := range ids {
		data, err := s.storage.Covers.Get(id)
		if err != nil {
			// Skip missing covers silently
			continue
		}

		// Write TAR header (512 bytes)
		filename := id + ".jpg"
		if err := writeTarHeader(w, filename, len(data)); err != nil {
			s.logger.Warn("failed to write tar header", "id", id, "error", err)
			return
		}

		// Write file data
		if _, err := w.Write(data); err != nil {
			s.logger.Warn("failed to write cover data", "id", id, "error", err)
			return
		}

		// Pad to 512-byte boundary
		padding := (512 - (len(data) % 512)) % 512
		if padding > 0 {
			if _, err := w.Write(make([]byte, padding)); err != nil {
				s.logger.Warn("failed to write padding", "id", id, "error", err)
				return
			}
		}
	}

	// Write two zero blocks to end the TAR
	w.Write(make([]byte, 1024))
}

// splitIDs splits a comma-separated string of IDs into a slice.
func splitIDs(s string) []string {
	if s == "" {
		return nil
	}
	var ids []string
	for _, id := range splitString(s, ',') {
		id = trimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// splitString splits a string by a separator (simple implementation).
func splitString(s string, sep byte) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// trimSpace trims leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// writeTarHeader writes a POSIX TAR header for a file.
func writeTarHeader(w http.ResponseWriter, filename string, size int) error {
	header := make([]byte, 512)

	// Filename (0-99)
	copy(header[0:100], filename)

	// Mode (100-107) - 0644 in octal
	copy(header[100:108], "0000644\x00")

	// UID (108-115)
	copy(header[108:116], "0000000\x00")

	// GID (116-123)
	copy(header[116:124], "0000000\x00")

	// Size (124-135) - 11 octal digits + null
	sizeStr := fmt.Sprintf("%011o", size)
	copy(header[124:136], sizeStr+"\x00")

	// Mtime (136-147)
	copy(header[136:148], "00000000000\x00")

	// Checksum placeholder (148-155) - spaces for now
	copy(header[148:156], "        ")

	// Type flag (156) - '0' for regular file
	header[156] = '0'

	// Calculate checksum
	var checksum int
	for _, b := range header {
		checksum += int(b)
	}
	checksumStr := fmt.Sprintf("%06o\x00 ", checksum)
	copy(header[148:156], checksumStr)

	_, err := w.Write(header)
	return err
}
