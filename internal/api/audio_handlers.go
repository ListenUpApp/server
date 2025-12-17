package api

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleStreamAudio streams an audio file with HTTP Range support for seeking.
// GET /api/v1/books/{id}/audio/{audioFileId}.
func (s *Server) handleStreamAudio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	bookID := chi.URLParam(r, "id")
	audioFileID := chi.URLParam(r, "audioFileId")

	if userID == "" {
		response.Unauthorized(w, "Authentication required", s.logger)
		return
	}

	if bookID == "" {
		response.BadRequest(w, "Book ID is required", s.logger)
		return
	}

	if audioFileID == "" {
		response.BadRequest(w, "Audio file ID is required", s.logger)
		return
	}

	// Get book (handles access control).
	book, err := s.services.Book.GetBook(ctx, userID, bookID)
	if err != nil {
		if errors.Is(err, store.ErrBookNotFound) {
			response.NotFound(w, "Book not found", s.logger)
			return
		}
		s.logger.Error("Failed to get book", "error", err, "book_id", bookID)
		response.InternalError(w, "Failed to retrieve book", s.logger)
		return
	}

	// Find the audio file.
	var audioFilePath string
	var audioFormat string
	for _, af := range book.AudioFiles {
		if af.ID == audioFileID {
			audioFilePath = af.Path
			audioFormat = af.Format
			break
		}
	}

	if audioFilePath == "" {
		response.NotFound(w, "Audio file not found", s.logger)
		return
	}

	// Verify file exists on disk.
	if _, err := os.Stat(audioFilePath); os.IsNotExist(err) {
		s.logger.Error("Audio file missing from disk",
			"book_id", bookID,
			"audio_file_id", audioFileID,
			"path", audioFilePath,
		)
		response.NotFound(w, "Audio file not found on disk", s.logger)
		return
	}

	// Set content type based on format.
	contentType := getAudioContentType(audioFormat)
	w.Header().Set("Content-Type", contentType)

	// Allow caching (audio files don't change).
	w.Header().Set("Cache-Control", "private, max-age=86400")

	// http.ServeFile handles:
	// - Range requests (partial content, 206 responses)
	// - Content-Length and Content-Range headers
	// - Accept-Ranges: bytes header
	// - If-Range conditional requests
	// - Last-Modified based caching
	http.ServeFile(w, r, audioFilePath)
}

// getAudioContentType returns the MIME type for an audio format.
func getAudioContentType(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "m4a", "m4b", "mp4":
		return "audio/mp4"
	case "ogg", "oga", "opus":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "aac":
		return "audio/aac"
	case "wma":
		return "audio/x-ms-wma"
	default:
		return "application/octet-stream"
	}
}
