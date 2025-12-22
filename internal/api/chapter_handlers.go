package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/chapters"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleSuggestChapterNames returns chapter alignment suggestions.
func (s *Server) handleSuggestChapterNames(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := mustGetUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	asin := r.URL.Query().Get("asin")
	region := r.URL.Query().Get("region")

	result, err := s.services.Chapter.SuggestChapterNames(ctx, userID, bookID, asin, region)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		if errors.Is(err, service.ErrNoASIN) {
			response.BadRequest(w, "Book has no ASIN. Match to Audible first or provide ?asin= parameter", s.logger)
			return
		}
		s.logger.Error("Failed to suggest chapter names", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to generate suggestions", s.logger)
		return
	}

	response.Success(w, result, s.logger)
}

// ApplyChaptersRequest contains the chapters to apply.
type ApplyChaptersRequest struct {
	Chapters []chapters.AlignedChapter `json:"chapters"`
}

// handleApplyChapterNames applies suggested chapter names.
func (s *Server) handleApplyChapterNames(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := mustGetUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	var req ApplyChaptersRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if len(req.Chapters) == 0 {
		response.BadRequest(w, "No chapters provided", s.logger)
		return
	}

	book, err := s.services.Chapter.ApplyChapterNames(ctx, userID, bookID, req.Chapters)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to apply chapter names", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to apply chapter names", s.logger)
		return
	}

	// Enrich for response
	enrichedBook, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		s.logger.Warn("Failed to enrich book", "error", err)
		response.Success(w, map[string]any{
			"book": book,
		}, s.logger)
		return
	}

	response.Success(w, map[string]any{
		"book": enrichedBook,
	}, s.logger)
}
