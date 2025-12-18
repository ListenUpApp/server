package api

import (
	"archive/tar"
	"net/http"
	"strings"

	"github.com/listenupapp/listenup-server/internal/http/response"
)

// handleBatchCovers returns multiple covers in a single TAR stream.
// GET /api/v1/covers/batch?ids=book_1,book_2,book_3
func (s *Server) handleBatchCovers(w http.ResponseWriter, r *http.Request) {
	idsParam := r.URL.Query().Get("ids")
	if idsParam == "" {
		response.BadRequest(w, "ids parameter required", s.logger)
		return
	}

	ids := strings.Split(idsParam, ",")
	if len(ids) > 100 {
		response.BadRequest(w, "max 100 covers per request", s.logger)
		return
	}

	w.Header().Set("Content-Type", "application/x-tar")
	tw := tar.NewWriter(w)
	defer tw.Close()

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		coverBytes, err := s.storage.Covers.Get(id)
		if err != nil {
			// Skip missing covers silently
			continue
		}

		err = tw.WriteHeader(&tar.Header{
			Name: id + ".jpg",
			Mode: 0644,
			Size: int64(len(coverBytes)),
		})
		if err != nil {
			s.logger.Error("Failed to write TAR header", "error", err, "book_id", id)
			continue
		}

		_, err = tw.Write(coverBytes)
		if err != nil {
			s.logger.Error("Failed to write TAR data", "error", err, "book_id", id)
			continue
		}

		// Flush for streaming
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}
