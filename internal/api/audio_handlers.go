package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// NOTE: Audio streaming routes are registered directly on chi (not Huma) because they
// serve raw binary audio data with range request support. They do NOT appear in /openapi.json.
// Routes:
//   GET /api/v1/audio/{bookId}/{fileId} - Stream audio file
//   GET /api/v1/books/{bookId}/audio/{fileId} - Stream audio file (alias)
//   GET /api/v1/audio/{bookId}/{fileId}/transcode/{*} - Stream transcoded audio
//   GET /api/v1/books/{bookId}/audio/{fileId}/transcode/{*} - Stream transcoded audio (alias)
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
	s.router.Get("/api/v1/books/{bookId}/audio/{fileId}/transcode/{*}", s.handleTranscodedAudio)
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

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, "failed to stat file", http.StatusInternalServerError)
		return
	}

	// Set content type based on format
	contentType := getMimeType(audioFile.Format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")

	// Handle range requests
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		s.handleRangeRequest(w, r, file, fileInfo.Size(), rangeHeader)
		return
	}

	// Full file
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, file)
	}
}

func (s *Server) handleRangeRequest(w http.ResponseWriter, r *http.Request, reader io.ReadSeeker, fileSize int64, rangeHeader string) {
	// Parse range header: "bytes=start-end"
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		http.Error(w, "invalid range header", http.StatusBadRequest)
		return
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		http.Error(w, "invalid range format", http.StatusBadRequest)
		return
	}

	var start, end int64
	var err error

	if parts[0] == "" {
		// Suffix range: -500 means last 500 bytes
		end = fileSize - 1
		suffixLen, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			http.Error(w, "invalid range", http.StatusBadRequest)
			return
		}
		start = max(fileSize-suffixLen, 0)
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "invalid range start", http.StatusBadRequest)
			return
		}

		if parts[1] == "" {
			end = fileSize - 1
		} else {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				http.Error(w, "invalid range end", http.StatusBadRequest)
				return
			}
		}
	}

	// Validate range
	if start < 0 || start >= fileSize || end < start || end >= fileSize {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
		http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Seek to start position
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		http.Error(w, "seek failed", http.StatusInternalServerError)
		return
	}

	// Set headers
	contentLength := end - start + 1
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
	w.WriteHeader(http.StatusPartialContent)

	if r.Method != http.MethodHead {
		_, _ = io.CopyN(w, reader, contentLength)
	}
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
	_, _ = io.Copy(w, file)
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
