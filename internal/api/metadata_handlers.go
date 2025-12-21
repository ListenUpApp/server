package api

import (
	"encoding/json/v2"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
)

// MatchFields specifies which simple fields to apply from Audible metadata.
type MatchFields struct {
	Title       bool `json:"title"`
	Subtitle    bool `json:"subtitle"`
	Description bool `json:"description"`
	Publisher   bool `json:"publisher"`
	ReleaseDate bool `json:"releaseDate"`
	Language    bool `json:"language"`
	Cover       bool `json:"cover"`
}

// SeriesMatchEntry specifies a series to apply with granular control.
type SeriesMatchEntry struct {
	ASIN          string `json:"asin"`
	ApplyName     bool   `json:"applyName"`
	ApplySequence bool   `json:"applySequence"`
}

// MatchRequest contains the user's selections for applying Audible metadata.
type MatchRequest struct {
	ASIN      string             `json:"asin"`
	Region    string             `json:"region"`
	Fields    MatchFields        `json:"fields"`
	Authors   []string           `json:"authors"`   // ASINs of selected authors
	Narrators []string           `json:"narrators"` // ASINs of selected narrators
	Series    []SeriesMatchEntry `json:"series"`
	Genres    []string           `json:"genres"` // Selected genre strings
}

// handleMetadataSearch searches the Audible catalog.
func (s *Server) handleMetadataSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		response.BadRequest(w, "Query parameter 'q' is required", s.logger)
		return
	}

	regionStr := r.URL.Query().Get("region")
	var region *audible.Region
	if regionStr != "" {
		reg := audible.Region(regionStr)
		if !reg.Valid() {
			response.BadRequest(w, "Invalid region", s.logger)
			return
		}
		region = &reg
	}

	results, err := s.services.Metadata.Search(r.Context(), region, audible.SearchParams{
		Keywords: query,
	})
	if err != nil {
		s.logger.Error("Metadata search failed", "error", err, "query", query)
		response.InternalError(w, "Search failed", s.logger)
		return
	}

	response.Success(w, map[string]any{
		"matches": results,
	}, s.logger)
}

// handleGetMetadataBook retrieves full book metadata from Audible.
func (s *Server) handleGetMetadataBook(w http.ResponseWriter, r *http.Request) {
	asin := chi.URLParam(r, "asin")
	if asin == "" {
		response.BadRequest(w, "ASIN is required", s.logger)
		return
	}

	regionStr := r.URL.Query().Get("region")
	var region *audible.Region
	if regionStr != "" {
		reg := audible.Region(regionStr)
		if !reg.Valid() {
			response.BadRequest(w, "Invalid region", s.logger)
			return
		}
		region = &reg
	}

	book, err := s.services.Metadata.GetBook(r.Context(), region, asin)
	if err != nil {
		s.logger.Error("Failed to get metadata book", "error", err, "asin", asin)
		response.InternalError(w, "Failed to fetch book metadata", s.logger)
		return
	}

	response.Success(w, map[string]any{
		"book": book,
	}, s.logger)
}

// handleGetMetadataChapters retrieves chapter information from Audible.
func (s *Server) handleGetMetadataChapters(w http.ResponseWriter, r *http.Request) {
	asin := chi.URLParam(r, "asin")
	if asin == "" {
		response.BadRequest(w, "ASIN is required", s.logger)
		return
	}

	regionStr := r.URL.Query().Get("region")
	var region *audible.Region
	if regionStr != "" {
		reg := audible.Region(regionStr)
		if !reg.Valid() {
			response.BadRequest(w, "Invalid region", s.logger)
			return
		}
		region = &reg
	}

	chapters, err := s.services.Metadata.GetChapters(r.Context(), region, asin)
	if err != nil {
		s.logger.Error("Failed to get metadata chapters", "error", err, "asin", asin)
		response.InternalError(w, "Failed to fetch chapters", s.logger)
		return
	}

	response.Success(w, map[string]any{
		"chapters": chapters,
	}, s.logger)
}

// handleMatchBook applies selected Audible metadata to a local book.
func (s *Server) handleMatchBook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := mustGetUserID(ctx)
	bookID := chi.URLParam(r, "id")

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	var req MatchRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if req.ASIN == "" {
		response.BadRequest(w, "ASIN is required", s.logger)
		return
	}

	book, err := s.services.Book.ApplyMatch(ctx, userID, bookID, req.ASIN, req.Region, service.ApplyMatchOptions{
		Fields: service.MatchFields{
			Title:       req.Fields.Title,
			Subtitle:    req.Fields.Subtitle,
			Description: req.Fields.Description,
			Publisher:   req.Fields.Publisher,
			ReleaseDate: req.Fields.ReleaseDate,
			Language:    req.Fields.Language,
			Cover:       req.Fields.Cover,
		},
		Authors:   req.Authors,
		Narrators: req.Narrators,
		Series:    convertSeriesEntries(req.Series),
		Genres:    req.Genres,
	})
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to apply match", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to apply metadata", s.logger)
		return
	}

	// Enrich book for response
	enrichedBook, err := s.store.EnrichBook(ctx, book)
	if err != nil {
		s.logger.Warn("Failed to enrich matched book", "error", err, "book_id", bookID)
		// Return basic book without enrichment
		response.Success(w, map[string]any{
			"book": book,
		}, s.logger)
		return
	}

	response.Success(w, map[string]any{
		"book": enrichedBook,
	}, s.logger)
}

// convertSeriesEntries converts API types to service types.
func convertSeriesEntries(entries []SeriesMatchEntry) []service.SeriesMatchEntry {
	result := make([]service.SeriesMatchEntry, len(entries))
	for i, e := range entries {
		result[i] = service.SeriesMatchEntry{
			ASIN:          e.ASIN,
			ApplyName:     e.ApplyName,
			ApplySequence: e.ApplySequence,
		}
	}
	return result
}
