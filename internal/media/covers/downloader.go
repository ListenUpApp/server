// Package covers provides cover image downloading and processing.
package covers

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/media/images"
)

const (
	// maxCoverSize limits download size to prevent memory exhaustion.
	maxCoverSize = 10 * 1024 * 1024 // 10MB

	// downloadTimeout is the maximum time for a cover download.
	downloadTimeout = 30 * time.Second
)

// DownloadResult contains the result of a cover download operation.
type DownloadResult struct {
	Success bool   // Whether the download and storage succeeded
	Width   int    // Actual image width
	Height  int    // Actual image height
	Size    int64  // File size in bytes
	Source  string // Source identifier (e.g., "audible", "itunes")
	Error   error  // Error if Success is false
}

// Downloader handles cover image downloads from various sources.
type Downloader struct {
	httpClient *http.Client
	storage    *images.Storage
	logger     *slog.Logger
}

// NewDownloader creates a new cover downloader.
func NewDownloader(storage *images.Storage, logger *slog.Logger) *Downloader {
	return &Downloader{
		httpClient: &http.Client{
			Timeout: downloadTimeout,
		},
		storage: storage,
		logger:  logger,
	}
}

// Download fetches a cover from the URL and stores it for the given book ID.
// Returns detailed results including dimensions and success status.
func (d *Downloader) Download(ctx context.Context, bookID, url, source string) *DownloadResult {
	result := &DownloadResult{Source: source}

	if url == "" {
		result.Error = errors.New("empty cover URL")
		return result
	}

	// Create timeout context
	downloadCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = fmt.Errorf("create request: %w", err)
		return result
	}

	// Execute request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("download: %w", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("download failed: status %d", resp.StatusCode)
		return result
	}

	// Read with size limit
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverSize))
	if err != nil {
		result.Error = fmt.Errorf("read data: %w", err)
		return result
	}

	result.Size = int64(len(data))

	// Parse dimensions before storing
	width, height, err := parseImageDimensions(data)
	if err != nil {
		d.logger.Warn("failed to parse cover dimensions",
			"book_id", bookID,
			"url", url,
			"error", err,
		)
		// Continue without dimensions - the image is still valid
	} else {
		result.Width = width
		result.Height = height
	}

	// Store the cover
	if err := d.storage.Save(bookID, data); err != nil {
		result.Error = fmt.Errorf("store: %w", err)
		return result
	}

	result.Success = true
	d.logger.Info("downloaded cover",
		"book_id", bookID,
		"source", source,
		"size", result.Size,
		"width", result.Width,
		"height", result.Height,
	)

	return result
}

// parseImageDimensions extracts dimensions from image data.
// Supports JPEG and PNG formats.
func parseImageDimensions(data []byte) (width, height int, err error) {
	if len(data) < 24 {
		return 0, 0, errors.New("data too small")
	}

	// Try JPEG first
	if w, h, ok := parseJPEGDimensions(data); ok {
		return w, h, nil
	}

	// Try PNG
	if w, h, ok := parsePNGDimensions(data); ok {
		return w, h, nil
	}

	return 0, 0, errors.New("unsupported format")
}

// parseJPEGDimensions extracts dimensions from JPEG data.
func parseJPEGDimensions(data []byte) (width, height int, ok bool) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, false // Not a JPEG
	}

	// Scan for SOF markers
	i := 2
	for i < len(data)-9 {
		if data[i] != 0xFF {
			i++
			continue
		}

		marker := data[i+1]

		// SOF0 (baseline), SOF1 (extended), SOF2 (progressive)
		if marker == 0xC0 || marker == 0xC1 || marker == 0xC2 {
			if i+9 > len(data) {
				return 0, 0, false
			}
			height = int(binary.BigEndian.Uint16(data[i+5 : i+7]))
			width = int(binary.BigEndian.Uint16(data[i+7 : i+9]))
			return width, height, true
		}

		// Skip to next marker
		if i+3 >= len(data) {
			break
		}
		segmentLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		i += 2 + segmentLen
	}

	return 0, 0, false
}

// parsePNGDimensions extracts dimensions from PNG data.
func parsePNGDimensions(data []byte) (width, height int, ok bool) {
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(data) < 24 || !bytes.Equal(data[:8], pngSig) {
		return 0, 0, false
	}

	if string(data[12:16]) != "IHDR" {
		return 0, 0, false
	}

	width = int(binary.BigEndian.Uint32(data[16:20]))
	height = int(binary.BigEndian.Uint32(data[20:24]))
	return width, height, true
}

// DetectSource determines the cover source from a URL.
func DetectSource(url string) string {
	switch {
	case strings.Contains(url, "mzstatic.com"):
		return "itunes"
	case strings.Contains(url, "amazon.com") || strings.Contains(url, "media-amazon.com"):
		return "audible"
	default:
		return "unknown"
	}
}
