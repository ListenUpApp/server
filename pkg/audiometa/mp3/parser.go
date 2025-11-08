package mp3

import (
	"os"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	binutil "github.com/listenupapp/listenup-server/pkg/audiometa/internal/binary"
)

// Parse parses a single MP3 file and extracts metadata
func Parse(path string) (*audiometa.Metadata, error) {
	// Open file
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := stat.Size()

	// Create safe reader
	sr := binutil.NewSafeReader(file, size, path)

	// Initialize metadata
	meta := &audiometa.Metadata{
		Format:   audiometa.FormatMP3,
		FileSize: size,
	}

	// Parse ID3v2 tag (if present)
	tagSize, err := parseID3v2(sr, meta)
	if err != nil {
		// Not an ID3v2 file or parse error
		// Try to find MP3 frames anyway
		meta.AddWarning("ID3v2 parsing failed: %v", err)
		tagSize = 0
	}

	// Parse MP3 frame headers for technical info (bitrate, duration, etc.)
	if err := parseTechnicalInfo(sr, tagSize, size, meta); err != nil {
		meta.AddWarning("failed to parse MP3 technical info: %v", err)
	}

	// Apply fallbacks
	// If no narrator from TXXX, use composer
	if meta.Narrator == "" && meta.Composer != "" {
		meta.Narrator = meta.Composer
	}

	return meta, nil
}