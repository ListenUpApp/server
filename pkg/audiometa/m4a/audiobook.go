package m4a

import (
	"fmt"
	"strings"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	"github.com/listenupapp/listenup-server/pkg/audiometa/internal/binary"
	"github.com/listenupapp/listenup-server/pkg/audiometa/parsing"
)

// parseAudiobookTags extracts narrator, series, publisher, etc. from custom atoms
func parseAudiobookTags(sr *binary.SafeReader, ilstAtom *Atom, meta *audiometa.Metadata) error {
	offset := ilstAtom.DataOffset()
	end := offset + int64(ilstAtom.DataSize())

	// Collect all custom tags for series part resolution
	customTags := make(map[string]string)

	// Scan through ilst for custom atoms (----)
	for offset < end {
		atom, err := readAtomHeader(sr, offset)
		if err != nil {
			// Skip corrupted atoms
			break
		}

		// Check if this is a custom atom (----)
		// Type: 0x2D2D2D2D = "----"
		if atom.Type == "----" {
			// Parse custom atom and collect tags
			if fieldName, value, err := parseCustomAtomWithTags(sr, atom, meta); err == nil {
				customTags[fieldName] = value
			}
		}

		offset += int64(atom.Size)
	}

	// Apply fallbacks
	// If no custom Narrator atom, use Composer as fallback
	if meta.Narrator == "" && meta.Composer != "" {
		meta.Narrator = meta.Composer
	}

	// If series exists, always resolve series part from multiple sources
	// This allows validation/override of potentially incorrect custom atom data
	if meta.Series != "" {
		meta.SeriesPart = resolveSeriesPart(sr, meta, customTags)
	}

	return nil
}

// parseCustomAtomWithTags parses a ---- custom atom and returns the field name and value
func parseCustomAtomWithTags(sr *binary.SafeReader, customAtom *Atom, meta *audiometa.Metadata) (string, string, error) {
	offset := customAtom.DataOffset()
	end := offset + int64(customAtom.DataSize())

	var namespace, fieldName, value string

	// Parse child atoms (mean, name, data)
	for offset < end {
		atom, err := readAtomHeader(sr, offset)
		if err != nil {
			break
		}

		switch atom.Type {
		case "mean":
			// Namespace (usually "com.apple.iTunes")
			// Skip version+flags (4 bytes)
			dataOffset := atom.DataOffset() + 4
			dataSize := int64(atom.DataSize()) - 4
			if dataSize > 0 {
				buf := make([]byte, dataSize)
				if err := sr.ReadAt(buf, dataOffset, "mean namespace"); err == nil {
					namespace = string(buf)
				}
			}

		case "name":
			// Field name (e.g., "Narrator", "Series")
			// Skip version+flags (4 bytes)
			dataOffset := atom.DataOffset() + 4
			dataSize := int64(atom.DataSize()) - 4
			if dataSize > 0 {
				buf := make([]byte, dataSize)
				if err := sr.ReadAt(buf, dataOffset, "name field"); err == nil {
					fieldName = string(buf)
				}
			}

		case "data":
			// Value - parse the data atom directly
			// Skip version (1 byte) + flags (3 bytes) + reserved (4 bytes) = 8 bytes
			valueOffset := atom.DataOffset() + 8
			valueSize := int64(atom.DataSize()) - 8
			if valueSize > 0 {
				buf := make([]byte, valueSize)
				if err := sr.ReadAt(buf, valueOffset, "data value"); err == nil {
					value = strings.TrimRight(string(buf), "\x00")
					value = strings.TrimSpace(value)
				}
			}
		}

		offset += int64(atom.Size)
	}

	// Map field name to metadata field
	// Namespace is usually "com.apple.iTunes" but can vary
	_ = namespace // Unused for now, but parsed for potential future filtering
	mapAudiobookField(fieldName, value, meta)

	return fieldName, value, nil
}

// mapAudiobookField maps custom field names to metadata fields
// NOTE: Series Part is NOT set here - it's resolved via resolveSeriesPart()
// to allow multi-source validation and fallback
func mapAudiobookField(fieldName, value string, meta *audiometa.Metadata) {
	// Normalize field name (case-insensitive)
	fieldName = strings.ToLower(fieldName)

	switch fieldName {
	case "narrator":
		meta.Narrator = value
	case "series":
		meta.Series = value
	// "series part", "seriespart", "part" - intentionally NOT set here
	// These are collected in customTags and resolved via resolveSeriesPart()
	case "publisher":
		meta.Publisher = value
	case "isbn":
		meta.ISBN = value
	case "asin":
		meta.ASIN = value
	}
}

// resolveSeriesPart determines series part from multiple sources
// Priority: Custom atoms > Track number > Title parsing > Album parsing > Path parsing
func resolveSeriesPart(sr *binary.SafeReader, meta *audiometa.Metadata, customTags map[string]string) string {
	// Priority 1: Custom iTunes atoms
	if part := customTags["Series Part"]; part != "" {
		return part
	}
	if part := customTags["Series Position"]; part != "" {
		return part
	}
	if part := customTags["Part"]; part != "" {
		return part
	}
	if part := customTags["Volume"]; part != "" {
		return part
	}

	// Priority 2: Track number (if likely series position)
	if parsing.IsLikelySeriesPosition(meta.TrackNumber, meta.TrackTotal) {
		return fmt.Sprintf("%d", meta.TrackNumber)
	}

	// Priority 3: Parse from title
	if part := parsing.ExtractSeriesPartFromText(meta.Title); part != "" {
		return part
	}

	// Priority 4: Parse from album
	if part := parsing.ExtractSeriesPartFromText(meta.Album); part != "" {
		return part
	}

	// Priority 5: Parse from file path (last resort)
	if part := parsing.ExtractSeriesPartFromPath(sr.Path()); part != "" {
		return part
	}

	return ""
}