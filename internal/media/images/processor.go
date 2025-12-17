package images

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/simonhull/audiometa"
)

// MIME type for unknown binary data.
const mimeOctetStream = "mimeOctetStream"

// CoverInfo contains metadata about an extracted cover image.
type CoverInfo struct {
	Hash   string // SHA256 hash of the cover for cache validation
	Size   int64  // Size in bytes
	Format string // MIME type (e.g., "image/jpeg")
}

// Processor extracts and stores cover images from audio files.
type Processor struct {
	storage *Storage
	logger  *slog.Logger
}

// NewProcessor creates a new Processor instance.
func NewProcessor(storage *Storage, logger *slog.Logger) *Processor {
	return &Processor{
		storage: storage,
		logger:  logger,
	}
}

// ExtractAndProcess extracts cover art from an audio file and stores it.
// Returns CoverInfo containing the hash, size, and format for database storage.
// Returns nil (no error) if the audio file has no embedded cover.
func (p *Processor) ExtractAndProcess(ctx context.Context, audioFilePath, bookID string) (*CoverInfo, error) {
	// Open audio file.
	file, err := audiometa.OpenContext(ctx, audioFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close() //nolint:errcheck // Defer close, nothing we can do about errors here

	// Extract artwork.
	artworks, err := file.ExtractArtwork()
	if err != nil {
		p.logger.Warn("failed to extract artwork",
			"path", audioFilePath,
			"error", err,
		)
		return nil, fmt.Errorf("failed to extract artwork: %w", err)
	}

	// No artwork found.
	if len(artworks) == 0 {
		p.logger.Debug("no embedded cover found",
			"path", audioFilePath,
			"format", file.Format.String(),
		)
		return nil, nil
	}

	p.logger.Debug("extracted cover art",
		"path", audioFilePath,
		"count", len(artworks),
		"size", len(artworks[0].Data),
	)

	// Use the first artwork (typically the front cover).
	artwork := artworks[0]

	// Save original artwork data to storage.
	if err := p.storage.Save(bookID, artwork.Data); err != nil {
		return nil, fmt.Errorf("failed to save cover: %w", err)
	}

	// Compute hash for cache validation.
	hash, err := p.storage.Hash(bookID)
	if err != nil {
		return nil, fmt.Errorf("failed to compute cover hash: %w", err)
	}

	// Determine MIME type from artwork data.
	format := detectImageFormat(artwork.Data)

	p.logger.Debug("extracted and saved cover",
		"book_id", bookID,
		"path", audioFilePath,
		"size", len(artwork.Data),
		"hash", hash[:8]+"...",
	)

	return &CoverInfo{
		Hash:   hash,
		Size:   int64(len(artwork.Data)),
		Format: format,
	}, nil
}

// detectImageFormat detects the MIME type from image data magic bytes.
func detectImageFormat(data []byte) string {
	if len(data) < 4 {
		return mimeOctetStream
	}

	// Check magic bytes
	switch {
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return "image/png"
	case data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46:
		return "image/gif"
	case data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46:
		// Could be WebP (RIFF....WEBP)
		if len(data) >= 12 && data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
			return "image/webp"
		}
		return mimeOctetStream
	default:
		return mimeOctetStream
	}
}

// ProcessExternalCover reads an external cover image file and stores it.
// This is used as a fallback when no embedded artwork is found.
// Returns CoverInfo containing the hash, size, and format.
func (p *Processor) ProcessExternalCover(coverPath, bookID string) (*CoverInfo, error) {
	// Read the external cover file.
	data, err := os.ReadFile(coverPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read external cover: %w", err)
	}

	// Save to storage.
	if err := p.storage.Save(bookID, data); err != nil {
		return nil, fmt.Errorf("failed to save external cover: %w", err)
	}

	// Compute hash for cache validation.
	hash, err := p.storage.Hash(bookID)
	if err != nil {
		return nil, fmt.Errorf("failed to compute cover hash: %w", err)
	}

	// Determine MIME type from image data.
	format := detectImageFormat(data)

	p.logger.Debug("processed external cover",
		"book_id", bookID,
		"path", coverPath,
		"size", len(data),
		"hash", hash[:8]+"...",
	)

	return &CoverInfo{
		Hash:   hash,
		Size:   int64(len(data)),
		Format: format,
	}, nil
}
