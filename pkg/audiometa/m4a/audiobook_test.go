package m4a

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	audiobinary "github.com/listenupapp/listenup-server/pkg/audiometa/internal/binary"
)

// createMockSafeReader creates a SafeReader for testing with a given file path
func createMockSafeReader(path string) *audiobinary.SafeReader {
	buf := make([]byte, 0)
	return audiobinary.NewSafeReader(bytes.NewReader(buf), 0, path)
}

// createCustomAtom creates a custom (----) atom with mean, name, and data
func createCustomAtom(namespace, fieldName, value string) []byte {
	buf := &bytes.Buffer{}

	// Create mean atom (namespace)
	// mean atom has version+flags (4 bytes) + data
	meanBuf := &bytes.Buffer{}
	binary.Write(meanBuf, binary.BigEndian, uint32(0)) // version+flags
	meanBuf.WriteString(namespace)
	meanAtom := createAtomWithType([]byte("mean"), meanBuf.Bytes())
	buf.Write(meanAtom)

	// Create name atom (field name)
	// name atom has version+flags (4 bytes) + data
	nameBuf := &bytes.Buffer{}
	binary.Write(nameBuf, binary.BigEndian, uint32(0)) // version+flags
	nameBuf.WriteString(fieldName)
	nameAtom := createAtomWithType([]byte("name"), nameBuf.Bytes())
	buf.Write(nameAtom)

	// Create data atom (value)
	dataAtom := createDataAtom(value)
	buf.Write(dataAtom)

	// Wrap in ---- atom
	customType := []byte{0x2D, 0x2D, 0x2D, 0x2D} // "----"
	return createAtomWithType(customType, buf.Bytes())
}

// createAtomWithType creates an atom with a specific type (as bytes)
func createAtomWithType(atomType []byte, data []byte) []byte {
	buf := &bytes.Buffer{}
	size := uint32(8 + len(data))
	binary.Write(buf, binary.BigEndian, size)
	buf.Write(atomType)
	buf.Write(data)
	return buf.Bytes()
}

func TestParseAudiobookTags_Narrator(t *testing.T) {
	// Create ilst with custom narrator atom
	narratorAtom := createCustomAtom("com.apple.iTunes", "Narrator", "Wil Wheaton")
	ilst := createMockAtom("ilst", narratorAtom)

	sr := audiobinary.NewSafeReader(bytes.NewReader(ilst), int64(len(ilst)), "test.m4b")
	ilstAtom, _ := readAtomHeader(sr, 0)

	meta := &audiometa.Metadata{}
	err := parseAudiobookTags(sr, ilstAtom, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Narrator != "Wil Wheaton" {
		t.Errorf("expected narrator 'Wil Wheaton', got '%s'", meta.Narrator)
	}
}

func TestParseAudiobookTags_Series(t *testing.T) {
	seriesAtom := createCustomAtom("com.apple.iTunes", "Series", "The Expanse")
	ilst := createMockAtom("ilst", seriesAtom)

	sr := audiobinary.NewSafeReader(bytes.NewReader(ilst), int64(len(ilst)), "test.m4b")
	ilstAtom, _ := readAtomHeader(sr, 0)

	meta := &audiometa.Metadata{}
	err := parseAudiobookTags(sr, ilstAtom, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Series != "The Expanse" {
		t.Errorf("expected series 'The Expanse', got '%s'", meta.Series)
	}
}

func TestParseAudiobookTags_MultipleFields(t *testing.T) {
	// Create ilst with multiple custom atoms
	var ilstData []byte
	ilstData = append(ilstData, createCustomAtom("com.apple.iTunes", "Narrator", "Andy Serkis")...)
	ilstData = append(ilstData, createCustomAtom("com.apple.iTunes", "Publisher", "Hachette Audio")...)
	ilstData = append(ilstData, createCustomAtom("com.apple.iTunes", "Series", "Lord of the Rings")...)
	ilstData = append(ilstData, createCustomAtom("com.apple.iTunes", "Series Part", "1")...)

	ilst := createMockAtom("ilst", ilstData)

	sr := audiobinary.NewSafeReader(bytes.NewReader(ilst), int64(len(ilst)), "test.m4b")
	ilstAtom, _ := readAtomHeader(sr, 0)

	meta := &audiometa.Metadata{}
	err := parseAudiobookTags(sr, ilstAtom, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Narrator != "Andy Serkis" {
		t.Errorf("expected narrator 'Andy Serkis', got '%s'", meta.Narrator)
	}
	if meta.Publisher != "Hachette Audio" {
		t.Errorf("expected publisher 'Hachette Audio', got '%s'", meta.Publisher)
	}
	if meta.Series != "Lord of the Rings" {
		t.Errorf("expected series 'Lord of the Rings', got '%s'", meta.Series)
	}
	if meta.SeriesPart != "1" {
		t.Errorf("expected series part '1', got '%s'", meta.SeriesPart)
	}
}

func TestParseAudiobookTags_NarratorFallback(t *testing.T) {
	// Create ilst with ©wrt (Composer) but no custom Narrator atom
	composerItem := createMetadataItem([]byte{0xA9, 'w', 'r', 't'}, "Stephen Fry")
	ilst := createMockAtom("ilst", composerItem)

	sr := audiobinary.NewSafeReader(bytes.NewReader(ilst), int64(len(ilst)), "test.m4b")
	ilstAtom, _ := readAtomHeader(sr, 0)

	meta := &audiometa.Metadata{
		Composer: "Stephen Fry", // Already parsed in main metadata
	}

	err := parseAudiobookTags(sr, ilstAtom, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// If no custom Narrator atom, should fall back to Composer
	// (This logic will be in the main parser, but test the custom atom parsing)
	// For now, just verify it doesn't overwrite
	if meta.Narrator != "" && meta.Narrator != "Stephen Fry" {
		t.Errorf("unexpected narrator value: '%s'", meta.Narrator)
	}
}

func TestParseAudiobookTags_NoCustomAtoms(t *testing.T) {
	// ilst with only standard atoms, no custom atoms
	titleItem := createMetadataItem([]byte{0xA9, 'n', 'a', 'm'}, "Test Book")
	ilst := createMockAtom("ilst", titleItem)

	sr := audiobinary.NewSafeReader(bytes.NewReader(ilst), int64(len(ilst)), "test.m4b")
	ilstAtom, _ := readAtomHeader(sr, 0)

	meta := &audiometa.Metadata{}
	err := parseAudiobookTags(sr, ilstAtom, meta)

	// Should not error, just no audiobook tags extracted
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if meta.Narrator != "" || meta.Series != "" || meta.Publisher != "" {
		t.Error("expected no audiobook tags to be set")
	}
}

func TestResolveSeriesPart_FromCustomAtom(t *testing.T) {
	tests := []struct {
		name       string
		customTags map[string]string
		meta       *audiometa.Metadata
		expected   string
	}{
		{
			name:       "Series Part custom atom",
			customTags: map[string]string{"Series Part": "2"},
			meta:       &audiometa.Metadata{},
			expected:   "2",
		},
		{
			name:       "Series Position custom atom",
			customTags: map[string]string{"Series Position": "3"},
			meta:       &audiometa.Metadata{},
			expected:   "3",
		},
		{
			name:       "Part custom atom",
			customTags: map[string]string{"Part": "4"},
			meta:       &audiometa.Metadata{},
			expected:   "4",
		},
		{
			name:       "Volume custom atom",
			customTags: map[string]string{"Volume": "5"},
			meta:       &audiometa.Metadata{},
			expected:   "5",
		},
		{
			name:       "Priority: Series Part wins",
			customTags: map[string]string{"Series Part": "2", "Part": "99"},
			meta:       &audiometa.Metadata{TrackNumber: 3, TrackTotal: 4},
			expected:   "2",
		},
	}

	sr := createMockSafeReader("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveSeriesPart(sr, tt.meta, tt.customTags)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestResolveSeriesPart_FromTrackNumber(t *testing.T) {
	tests := []struct {
		name       string
		meta       *audiometa.Metadata
		customTags map[string]string
		expected   string
	}{
		{
			name: "Track 2 of 4 (likely series)",
			meta: &audiometa.Metadata{
				TrackNumber: 2,
				TrackTotal:  4,
			},
			customTags: map[string]string{},
			expected:   "2",
		},
		{
			name: "Track 15 of 69 (likely chapters, not series)",
			meta: &audiometa.Metadata{
				TrackNumber: 15,
				TrackTotal:  69,
			},
			customTags: map[string]string{},
			expected:   "",
		},
		{
			name: "Track 0 of 4 (invalid)",
			meta: &audiometa.Metadata{
				TrackNumber: 0,
				TrackTotal:  4,
			},
			customTags: map[string]string{},
			expected:   "",
		},
	}

	sr := createMockSafeReader("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveSeriesPart(sr, tt.meta, tt.customTags)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestResolveSeriesPart_FromTitleParsing(t *testing.T) {
	tests := []struct {
		name       string
		meta       *audiometa.Metadata
		customTags map[string]string
		expected   string
	}{
		{
			name: "Book 2 in title",
			meta: &audiometa.Metadata{
				Title: "The Wingfeather Saga, Book 2: North or Be Eaten",
			},
			customTags: map[string]string{},
			expected:   "2",
		},
		{
			name: "Part 3 in title",
			meta: &audiometa.Metadata{
				Title: "Part 3: The Monster in the Hollows",
			},
			customTags: map[string]string{},
			expected:   "3",
		},
		{
			name: "No series info in title",
			meta: &audiometa.Metadata{
				Title: "The Martian",
			},
			customTags: map[string]string{},
			expected:   "",
		},
	}

	sr := createMockSafeReader("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveSeriesPart(sr, tt.meta, tt.customTags)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestResolveSeriesPart_FromAlbumParsing(t *testing.T) {
	tests := []struct {
		name       string
		meta       *audiometa.Metadata
		customTags map[string]string
		expected   string
	}{
		{
			name: "Book 2 in album",
			meta: &audiometa.Metadata{
				Title: "North or Be Eaten",
				Album: "The Wingfeather Saga, Book 2",
			},
			customTags: map[string]string{},
			expected:   "2",
		},
		{
			name: "Title takes priority over album",
			meta: &audiometa.Metadata{
				Title: "Book 3: The Title",
				Album: "Book 2: The Album",
			},
			customTags: map[string]string{},
			expected:   "3",
		},
	}

	sr := createMockSafeReader("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveSeriesPart(sr, tt.meta, tt.customTags)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestResolveSeriesPart_PriorityOrder(t *testing.T) {
	sr := createMockSafeReader("")

	// Test that custom atoms take priority over everything
	meta := &audiometa.Metadata{
		Title:       "Book 99: Wrong",
		Album:       "Book 98: Also Wrong",
		TrackNumber: 97,
		TrackTotal:  100,
	}
	customTags := map[string]string{"Series Part": "2"}

	result := resolveSeriesPart(sr, meta, customTags)
	if result != "2" {
		t.Errorf("custom atom should win, expected '2', got '%s'", result)
	}

	// Test that track number takes priority over title when custom atom absent
	meta2 := &audiometa.Metadata{
		Title:       "Book 99: Wrong",
		TrackNumber: 3,
		TrackTotal:  4,
	}
	customTags2 := map[string]string{}

	result2 := resolveSeriesPart(sr, meta2, customTags2)
	if result2 != "3" {
		t.Errorf("track number should win, expected '3', got '%s'", result2)
	}
}

func TestResolveSeriesPart_FromPathParsing(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		meta       *audiometa.Metadata
		customTags map[string]string
		expected   string
	}{
		{
			name: "Path parsing as last resort",
			path: "/audiobooks/Author/Series/2 - North or Be Eaten/file.m4b",
			meta: &audiometa.Metadata{
				Title: "North or Be Eaten",
				Album: "The Wingfeather Saga",
			},
			customTags: map[string]string{},
			expected:   "2",
		},
		{
			name: "Path with Book keyword",
			path: "/audiobooks/C.S. Lewis/Narnia/Book 3 - The Horse and His Boy/file.m4b",
			meta: &audiometa.Metadata{
				Title: "The Horse and His Boy",
			},
			customTags: map[string]string{},
			expected:   "3",
		},
		{
			name: "Custom atom overrides path",
			path: "/audiobooks/Author/Series/2 - Wrong Folder/file.m4b",
			meta: &audiometa.Metadata{
				Title: "Some Book",
			},
			customTags: map[string]string{"Series Part": "5"},
			expected:   "5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := createMockSafeReader(tt.path)
			result := resolveSeriesPart(sr, tt.meta, tt.customTags)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
