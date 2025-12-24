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

	// Direct chi route for cover streaming
	s.router.Get("/covers/{path}", s.handleServeCover)
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
