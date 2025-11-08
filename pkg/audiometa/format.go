package audiometa

import (
	"io"

	"github.com/listenupapp/listenup-server/pkg/audiometa/internal/binary"
)

// Format represents the detected audio format
type Format int

const (
	FormatUnknown Format = iota
	FormatM4B
	FormatM4A
	FormatMP3
)

func (f Format) String() string {
	switch f {
	case FormatM4B:
		return "M4B"
	case FormatM4A:
		return "M4A"
	case FormatMP3:
		return "MP3"
	default:
		return "Unknown"
	}
}

// DetectFormat determines the audio file format (M4B, M4A, or MP3)
func DetectFormat(r io.ReaderAt, size int64, path string) (Format, error) {
	// File must be at least 8 bytes
	if size < 8 {
		return FormatUnknown, &UnsupportedFormatError{
			Path:   path,
			Reason: "file too small",
		}
	}

	sr := binary.NewSafeReader(r, size, path)

	// Check for ID3v2 tag (MP3)
	id3Header := make([]byte, 3)
	if err := sr.ReadAt(id3Header, 0, "file header"); err == nil {
		if string(id3Header) == "ID3" {
			return FormatMP3, nil
		}
	}

	// Check for MP3 frame sync (0xFFE or 0xFFF)
	// This catches MP3 files without ID3 tags
	frameHeader := make([]byte, 2)
	if err := sr.ReadAt(frameHeader, 0, "frame sync"); err == nil {
		if frameHeader[0] == 0xFF && (frameHeader[1]&0xE0) == 0xE0 {
			return FormatMP3, nil
		}
	}

	// Check for M4B/M4A ftyp atom
	// Read ftyp atom size (first 4 bytes)
	atomSize, err := binary.Read[uint32](sr, 0, "ftyp atom size")
	if err != nil {
		return FormatUnknown, &UnsupportedFormatError{
			Path:   path,
			Reason: "failed to read file header",
		}
	}

	// Read ftyp atom type (next 4 bytes)
	atomType, err := binary.Read[uint32](sr, 4, "ftyp atom type")
	if err != nil {
		return FormatUnknown, &UnsupportedFormatError{
			Path:   path,
			Reason: "failed to read file header",
		}
	}

	// Check if it's an ftyp atom (0x66747970 = "ftyp")
	ftypMagic := uint32(0x66747970)
	if atomType != ftypMagic {
		return FormatUnknown, &UnsupportedFormatError{
			Path:   path,
			Reason: "unsupported file format",
		}
	}

	// ftyp atom must be at least 16 bytes (size + type + brand + version)
	if atomSize < 16 {
		return FormatUnknown, &UnsupportedFormatError{
			Path:   path,
			Reason: "ftyp atom too small",
		}
	}

	// Read major brand (next 4 bytes)
	majorBrand, err := binary.Read[uint32](sr, 8, "major brand")
	if err != nil {
		return FormatUnknown, &UnsupportedFormatError{
			Path:   path,
			Reason: "failed to read major brand",
		}
	}

	// Check for M4B brand (0x4D344220 = "M4B ")
	m4bMagic := uint32(0x4D344220)
	if majorBrand == m4bMagic {
		return FormatM4B, nil
	}

	// Check for M4A brands
	// M4A  = 0x4D344120 = "M4A "
	// mp42 = 0x6D703432 = "mp42"
	// isom = 0x69736F6D = "isom"
	m4aMagic := uint32(0x4D344120)
	mp42Magic := uint32(0x6D703432)
	isomMagic := uint32(0x69736F6D)

	if majorBrand == m4aMagic || majorBrand == mp42Magic || majorBrand == isomMagic {
		return FormatM4A, nil
	}

	// Unsupported brand
	return FormatUnknown, &UnsupportedFormatError{
		Path:   path,
		Reason: "unsupported file brand",
	}
}
