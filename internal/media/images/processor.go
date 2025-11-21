package images

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/simonhull/audiometa"
)

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
// Returns the SHA256 hash of the cover for cache validation.
// Returns empty string (no error) if the audio file has no embedded cover.
func (p *Processor) ExtractAndProcess(ctx context.Context, audioFilePath string, bookID string) (string, error) {
	// Open audio file.
	file, err := audiometa.OpenContext(ctx, audioFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close() //nolint:errcheck // Defer close, nothing we can do about errors here

	// Extract artwork.
	artworks, err := file.ExtractArtwork()
	if err != nil {
		p.logger.Warn("failed to extract artwork",
			"path", audioFilePath,
			"error", err,
		)
		return "", fmt.Errorf("failed to extract artwork: %w", err)
	}

	// No artwork found.
	if len(artworks) == 0 {
		p.logger.Debug("no embedded cover found",
			"path", audioFilePath,
			"format", file.Format.String(),
		)
		return "", nil
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
		return "", fmt.Errorf("failed to save cover: %w", err)
	}

	// Compute hash for cache validation.
	hash, err := p.storage.Hash(bookID)
	if err != nil {
		return "", fmt.Errorf("failed to compute cover hash: %w", err)
	}

	p.logger.Debug("extracted and saved cover",
		"book_id", bookID,
		"path", audioFilePath,
		"size", len(artwork.Data),
		"hash", hash[:8]+"...",
	)

	return hash, nil
}
