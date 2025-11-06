package m4a

import (
	"os"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	"github.com/listenupapp/listenup-server/pkg/audiometa/internal/binary"
)

// Parse parses an M4A/M4B file and extracts metadata
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
	sr := binary.NewSafeReader(file, size, path)

	// Detect format
	format, err := audiometa.DetectFormat(file, size, path)
	if err != nil {
		return nil, err
	}

	// Initialize metadata
	meta := &audiometa.Metadata{
		Format:   format,
		FileSize: size,
	}

	// Find moov atom (movie container)
	moovAtom, err := findAtom(sr, 0, size, "moov")
	if err != nil {
		// No moov atom - return basic metadata
		return meta, nil
	}

	// Find udta atom (user data) inside moov
	udtaAtom, err := findAtom(sr, moovAtom.DataOffset(), moovAtom.DataOffset()+int64(moovAtom.DataSize()), "udta")
	if err != nil {
		// No udta - return basic metadata
		return meta, nil
	}

	// Find meta atom inside udta
	metaAtom, err := findAtom(sr, udtaAtom.DataOffset(), udtaAtom.DataOffset()+int64(udtaAtom.DataSize()), "meta")
	if err != nil {
		// No meta - return basic metadata
		return meta, nil
	}

	// meta atom has 4 bytes of version+flags before the data
	metaDataOffset := metaAtom.DataOffset() + 4
	metaDataEnd := metaAtom.DataOffset() + int64(metaAtom.DataSize())

	// Find ilst atom (iTunes metadata list) inside meta
	ilstAtom, err := findAtom(sr, metaDataOffset, metaDataEnd, "ilst")
	if err != nil {
		// No ilst - return basic metadata
		return meta, nil
	}

	// Extract metadata from ilst
	if err := extractIlstMetadata(sr, ilstAtom, meta); err != nil {
		// Log error but don't fail - return partial metadata
		// In production, we'd use proper logging here
	}

	return meta, nil
}
