package itunes

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// MaxCoverSize is the size we request from iTunes.
// iTunes will serve the largest available size up to this.
const MaxCoverSize = "7000x7000bb.jpg"

// sizePattern matches iTunes artwork size patterns like "100x100bb.jpg"
var sizePattern = regexp.MustCompile(`/\d+x\d+bb\.jpg$`)

// MaxCoverURL transforms an iTunes artwork URL to request maximum resolution.
// iTunes will serve the largest available size (e.g., 2400x2400 if that's the max).
func MaxCoverURL(url string) string {
	if url == "" {
		return ""
	}
	return sizePattern.ReplaceAllString(url, "/"+MaxCoverSize)
}

// GetImageDimensions fetches actual image dimensions by reading headers.
// Uses HTTP Range request to fetch only the bytes needed for JPEG/PNG parsing.
func GetImageDimensions(ctx context.Context, httpClient *http.Client, url string) (width, height int, err error) {
	if url == "" {
		return 0, 0, fmt.Errorf("empty URL")
	}

	// Request first 32KB - enough for image headers
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Range", "bytes=0-32767")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch image header: %w", err)
	}
	defer resp.Body.Close()

	// Accept both 200 (full content) and 206 (partial content)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Read the bytes we got
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32768))
	if err != nil {
		return 0, 0, fmt.Errorf("read image header: %w", err)
	}

	if len(data) == 0 {
		return 0, 0, fmt.Errorf("empty response body (status %d)", resp.StatusCode)
	}

	// Try to parse as JPEG first, then PNG
	if w, h, ok := parseJPEGDimensions(data); ok {
		return w, h, nil
	}
	if w, h, ok := parsePNGDimensions(data); ok {
		return w, h, nil
	}

	return 0, 0, fmt.Errorf("could not determine image dimensions (read %d bytes, first byte: 0x%02x)", len(data), data[0])
}

// parseJPEGDimensions extracts dimensions from JPEG data.
// Looks for SOF0, SOF1, or SOF2 markers which contain image dimensions.
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
			// SOF segment: length(2) + precision(1) + height(2) + width(2)
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
// Reads width and height from the IHDR chunk.
func parsePNGDimensions(data []byte) (width, height int, ok bool) {
	// PNG signature: 137 80 78 71 13 10 26 10
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(data) < 24 || !bytes.Equal(data[:8], pngSig) {
		return 0, 0, false // Not a PNG
	}

	// IHDR chunk starts at byte 8
	// Format: length(4) + type(4) + width(4) + height(4) + ...
	// The type should be "IHDR"
	if string(data[12:16]) != "IHDR" {
		return 0, 0, false
	}

	width = int(binary.BigEndian.Uint32(data[16:20]))
	height = int(binary.BigEndian.Uint32(data[20:24]))
	return width, height, true
}
