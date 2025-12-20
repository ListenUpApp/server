package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/store"
)

// handleStreamAudio streams an audio file with HTTP Range support for seeking.
// GET /api/v1/books/{id}/audio/{audioFileId}
// Optional query param: ?variant=transcoded to serve transcoded version.
func (s *Server) handleStreamAudio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := mustGetUserID(ctx)
	bookID := chi.URLParam(r, "id")
	audioFileID := chi.URLParam(r, "audioFileId")
	variant := r.URL.Query().Get("variant")

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

	// Check if requesting transcoded variant.
	if variant == "transcoded" {
		s.streamTranscodedAudio(w, r, audioFileID, audioFilePath)
		return
	}

	// Serve original file.
	s.streamOriginalAudio(w, r, bookID, audioFileID, audioFilePath, audioFormat)
}

// streamOriginalAudio serves the original audio file.
func (s *Server) streamOriginalAudio(w http.ResponseWriter, r *http.Request, bookID, audioFileID, path, format string) {
	// Verify file exists on disk.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		s.logger.Error("Audio file missing from disk",
			"book_id", bookID,
			"audio_file_id", audioFileID,
			"path", path,
		)
		response.NotFound(w, "Audio file not found on disk", s.logger)
		return
	}

	// Set content type based on format.
	contentType := getAudioContentType(format)
	w.Header().Set("Content-Type", contentType)

	// Allow caching (audio files don't change).
	w.Header().Set("Cache-Control", CacheOneDayPrivate)

	// http.ServeFile handles:
	// - Range requests (partial content, 206 responses)
	// - Content-Length and Content-Range headers
	// - Accept-Ranges: bytes header
	// - If-Range conditional requests
	// - Last-Modified based caching
	http.ServeFile(w, r, path)
}

// streamTranscodedAudio redirects to the HLS playlist for transcoded content.
// HLS allows progressive playback - client can start as soon as first segment is ready.
func (s *Server) streamTranscodedAudio(w http.ResponseWriter, r *http.Request, audioFileID, originalPath string) {
	ctx := r.Context()

	// Check if transcoding is available.
	if s.services.Transcode == nil || !s.services.Transcode.IsEnabled() {
		// Fallback to original if transcoding disabled.
		s.logger.Warn("Transcode requested but transcoding is disabled",
			"audio_file_id", audioFileID,
		)
		response.NotFound(w, "Transcoded variant not available", s.logger)
		return
	}

	// Check if HLS content is ready (first segment available).
	_, ok := s.services.Transcode.GetHLSPathIfReady(ctx, audioFileID)
	if !ok {
		// HLS not ready yet - check if transcode is in progress.
		response.JSON(w, http.StatusAccepted, map[string]any{
			"status":  "transcoding",
			"message": "Audio is being transcoded, please try again shortly",
		}, s.logger)
		return
	}

	// Redirect to HLS playlist endpoint.
	// Client should use: /api/v1/transcode/{audioFileID}/playlist.m3u8
	redirectURL := fmt.Sprintf("/api/v1/transcode/%s/playlist.m3u8", audioFileID)
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// handleHLSPlaylist serves the HLS playlist (.m3u8) for transcoded audio.
// Generates playlist dynamically from available segments for progressive playback.
func (s *Server) handleHLSPlaylist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = mustGetUserID(ctx) // Validates auth; userID not needed for this endpoint
	audioFileID := chi.URLParam(r, "audioFileId")

	if audioFileID == "" {
		response.BadRequest(w, "Audio file ID is required", s.logger)
		return
	}

	// Generate dynamic playlist from available segments
	playlist, err := s.services.Transcode.GenerateDynamicPlaylist(ctx, audioFileID)
	if err != nil {
		s.logger.Debug("Playlist not ready", "error", err, "audio_file_id", audioFileID)
		response.NotFound(w, "HLS content not ready", s.logger)
		return
	}

	// Build base URL for segment URLs
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	// Rewrite segment paths to full API URLs
	rewritten := rewriteHLSPlaylist(playlist, audioFileID, baseURL)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", CacheNoStore) // Playlist changes during transcode
	w.Write([]byte(rewritten))
}

// handleHLSSegment serves an HLS segment (.ts) for transcoded audio.
// GET /api/v1/transcode/{audioFileId}/{segment}
func (s *Server) handleHLSSegment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = mustGetUserID(ctx) // Validates auth; userID not needed for this endpoint
	audioFileID := chi.URLParam(r, "audioFileId")
	segment := chi.URLParam(r, "segment")

	if audioFileID == "" || segment == "" {
		response.BadRequest(w, "Audio file ID and segment are required", s.logger)
		return
	}

	// Validate segment filename (prevent path traversal).
	if !isValidSegmentName(segment) {
		response.BadRequest(w, "Invalid segment name", s.logger)
		return
	}

	// Get HLS path.
	hlsPath, ok := s.services.Transcode.GetHLSPathIfReady(ctx, audioFileID)
	if !ok {
		response.NotFound(w, "HLS content not ready", s.logger)
		return
	}

	segmentPath := hlsPath + "/" + segment

	// Verify segment exists.
	if _, err := os.Stat(segmentPath); os.IsNotExist(err) {
		response.NotFound(w, "Segment not found", s.logger)
		return
	}

	w.Header().Set("Content-Type", "video/MP2T")
	w.Header().Set("Cache-Control", CacheOneDayPrivate) // Segments are immutable
	http.ServeFile(w, r, segmentPath)
}

// rewriteHLSPlaylist rewrites segment paths in the playlist to use full API URLs.
// Also ensures #EXT-X-ENDLIST is present for ExoPlayer compatibility during progressive transcoding.
func rewriteHLSPlaylist(content, audioFileID, baseURL string) string {
	lines := strings.Split(content, "\n")
	var result []string
	hasEndList := false

	for _, line := range lines {
		if strings.HasSuffix(line, ".ts") && !strings.HasPrefix(line, "#") {
			// Rewrite segment path to full API URL.
			segmentName := strings.TrimSpace(line)
			line = fmt.Sprintf("%s/api/v1/transcode/%s/%s", baseURL, audioFileID, segmentName)
		}
		if strings.TrimSpace(line) == "#EXT-X-ENDLIST" {
			hasEndList = true
		}
		result = append(result, line)
	}

	// Append #EXT-X-ENDLIST if not present.
	// This allows ExoPlayer to play the stream as VOD even during transcoding.
	if !hasEndList {
		result = append(result, "#EXT-X-ENDLIST")
	}

	return strings.Join(result, "\n")
}

// isValidSegmentName validates HLS segment filenames.
func isValidSegmentName(name string) bool {
	// Only allow seg_XXXX.ts format.
	if !strings.HasSuffix(name, ".ts") {
		return false
	}
	// Prevent path traversal.
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return false
	}
	return true
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
