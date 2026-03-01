package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// NOTE: Audio streaming routes are registered directly on chi (not Huma) because they
// serve raw binary audio data with range request support. They do NOT appear in /openapi.json.
// Routes:
//
//	GET /api/v1/audio/{bookId}/{fileId} - Stream audio file
//	GET /api/v1/books/{bookId}/audio/{fileId} - Stream audio file (alias)
//	GET /api/v1/audio/{bookId}/{fileId}/transcode/{*} - Stream transcoded audio
//	GET /api/v1/books/{bookId}/audio/{fileId}/transcode/{*} - Stream transcoded audio (alias)
//
// registerAudioRoutes sets up audio streaming routes.
// These are handled directly by chi for performance (not huma).
func (s *Server) registerAudioRoutes() {
	// Audio streaming with range support (new URL format)
	s.router.Get("/api/v1/audio/{bookId}/{fileId}", s.handleStreamAudio)
	s.router.Head("/api/v1/audio/{bookId}/{fileId}", s.handleStreamAudio)

	// Legacy URL format (for client compatibility)
	s.router.Get("/api/v1/books/{bookId}/audio/{fileId}", s.handleStreamAudio)
	s.router.Head("/api/v1/books/{bookId}/audio/{fileId}", s.handleStreamAudio)

	// Transcoded audio (HLS segments, etc.)
	s.router.Get("/api/v1/audio/{bookId}/{fileId}/transcode/{*}", s.handleTranscodedAudio)
	s.router.Head("/api/v1/audio/{bookId}/{fileId}/transcode/{*}", s.handleTranscodedAudio)
	s.router.Get("/api/v1/books/{bookId}/audio/{fileId}/transcode/{*}", s.handleTranscodedAudio)
	s.router.Head("/api/v1/books/{bookId}/audio/{fileId}/transcode/{*}", s.handleTranscodedAudio)
}

// handleStreamAudio streams audio files with range request support.
func (s *Server) handleStreamAudio(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookId")
	fileID := chi.URLParam(r, "fileId")

	// Extract token from query or header
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = authHeader[7:]
		}
	}

	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify token
	user, _, err := s.services.Auth.VerifyAccessToken(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Get book to verify access
	book, err := s.store.GetBook(r.Context(), bookID, user.ID)
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	// Find the audio file
	audioFile := book.GetAudioFileByID(fileID)
	if audioFile == nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// Open the file directly from the filesystem
	file, err := os.Open(audioFile.Path)
	if err != nil {
		http.Error(w, "failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get file info for modtime
	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, "failed to stat file", http.StatusInternalServerError)
		return
	}

	// Set content type based on format
	w.Header().Set("Content-Type", getMimeType(audioFile.Format))

	// ServeContent handles Range requests, Content-Length, and HEAD automatically
	http.ServeContent(w, r, audioFile.Path, fileInfo.ModTime(), file)
}

func (s *Server) handleTranscodedAudio(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookId")
	fileID := chi.URLParam(r, "fileId")
	transcodePath := chi.URLParam(r, "*")

	// Extract token
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = authHeader[7:]
		}
	}

	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify token
	user, _, err := s.services.Auth.VerifyAccessToken(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Verify book access
	_, err = s.store.GetBook(r.Context(), bookID, user.ID)
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	// Get HLS path from transcode service
	hlsPath, ok := s.services.Transcode.GetHLSPath(r.Context(), fileID, nil)
	if !ok {
		http.Error(w, "transcoded file not ready", http.StatusNotFound)
		return
	}

	// Serve the requested file from the HLS directory
	filePath := filepath.Join(hlsPath, transcodePath)

	// Validate the file is within the HLS directory (security check)
	// Clean both paths and ensure separator suffix to prevent bypass
	// (e.g., /tmp/hls matching /tmp/hls_malicious)
	cleanHlsPath := filepath.Clean(hlsPath) + string(filepath.Separator)
	cleanPath := filepath.Clean(filePath)
	if !strings.HasPrefix(cleanPath, cleanHlsPath) && cleanPath != filepath.Clean(hlsPath) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "transcoded file not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, "failed to stat file", http.StatusInternalServerError)
		return
	}

	// Determine content type
	contentType := "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(transcodePath))
	switch ext {
	case ".m3u8":
		contentType = "application/vnd.apple.mpegurl"
	case ".ts":
		contentType = "video/mp2t"
	case ".m4s":
		contentType = "video/iso.segment"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// ServeContent handles Range requests, Content-Length, and HEAD automatically
	http.ServeContent(w, r, transcodePath, fileInfo.ModTime(), file)
}

// getMimeType returns MIME type based on audio format.
func getMimeType(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "m4a", "m4b", "mp4", "aac":
		return "audio/mp4"
	case "opus":
		return "audio/opus"
	case "ogg":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}
