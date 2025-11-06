package m4a

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	"github.com/listenupapp/listenup-server/pkg/audiometa/internal/binary"
)

// parseMetadataTag extracts the string value from an iTunes metadata tag atom
func parseMetadataTag(sr *binary.SafeReader, tagAtom *Atom) (string, error) {
	// Tag atoms contain a "data" atom with the actual value
	// Format: tag atom → data atom → version/flags → value

	if tagAtom.DataSize() == 0 {
		return "", nil
	}

	// Find data atom inside tag
	dataAtom, err := findAtom(sr, tagAtom.DataOffset(), tagAtom.DataOffset()+int64(tagAtom.DataSize()), "data")
	if err != nil {
		// No data atom found - return empty
		return "", nil
	}

	// Skip version (1 byte) + flags (3 bytes) + reserved (4 bytes) = 8 bytes
	valueOffset := dataAtom.DataOffset() + 8
	valueSize := int64(dataAtom.DataSize()) - 8

	if valueSize <= 0 {
		return "", nil
	}

	// Read the string value
	buf := make([]byte, valueSize)
	if err := sr.ReadAt(buf, valueOffset, "metadata value"); err != nil {
		return "", err
	}

	// Trim null bytes and whitespace
	value := string(buf)
	value = strings.TrimRight(value, "\x00")
	value = strings.TrimSpace(value)

	return value, nil
}

// extractIlstMetadata parses all metadata items from the ilst atom
func extractIlstMetadata(sr *binary.SafeReader, ilstAtom *Atom, meta *audiometa.Metadata) error {
	offset := ilstAtom.DataOffset()
	end := offset + int64(ilstAtom.DataSize())

	for offset < end {
		// Read tag atom
		tagAtom, err := readAtomHeader(sr, offset)
		if err != nil {
			return err
		}

		// Parse tag value
		value, err := parseMetadataTag(sr, tagAtom)
		if err != nil {
			// Log error but continue parsing other tags
			fmt.Printf("warning: failed to parse tag %s: %v\n", tagAtom.Type, err)
		} else {
			// Map tag to metadata field
			mapTagToField(tagAtom.Type, value, meta)
		}

		// Move to next tag
		offset += int64(tagAtom.Size)
	}

	return nil
}

// mapTagToField maps an iTunes tag to the appropriate metadata field
// Note: In MP4, © is represented as byte 0xA9, so "©nam" is "\xA9nam" in Go strings
func mapTagToField(tag string, value string, meta *audiometa.Metadata) {
	switch tag {
	case "\xA9nam": // Title (©nam)
		meta.Title = value
	case "\xA9ART": // Artist (©ART)
		meta.Artist = value
	case "\xA9alb": // Album (©alb)
		meta.Album = value
	case "\xA9gen": // Genre (©gen)
		meta.Genre = value
	case "\xA9cmt": // Comment (©cmt)
		meta.Comment = value
	case "\xA9wrt": // Composer (©wrt)
		meta.Composer = value
	case "\xA9day": // Year (©day)
		if year, err := strconv.Atoi(value); err == nil {
			meta.Year = year
		}
	}
}