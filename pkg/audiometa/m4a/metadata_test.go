package m4a

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	audiobinary "github.com/listenupapp/listenup-server/pkg/audiometa/internal/binary"
)

// createDataAtom creates a data atom with string content
func createDataAtom(value string) []byte {
	buf := &bytes.Buffer{}

	// data atom size
	dataSize := uint32(8 + 8 + len(value)) // header + version/flags/reserved + value
	binary.Write(buf, binary.BigEndian, dataSize)

	// data atom type
	buf.WriteString("data")

	// version (1) + flags (3) + reserved (4)
	binary.Write(buf, binary.BigEndian, uint32(1))      // version=0, flags=1 (UTF-8 text)
	binary.Write(buf, binary.BigEndian, uint32(0))      // reserved

	// value
	buf.WriteString(value)

	return buf.Bytes()
}

// createMetadataItem creates an iTunes metadata item (e.g., ©nam for title)
// For iTunes tags, itemType should be the 4-byte tag code as bytes (e.g., []byte{0xA9, 'n', 'a', 'm'} for ©nam)
func createMetadataItem(itemType []byte, value string) []byte {
	if len(itemType) != 4 {
		panic("itemType must be exactly 4 bytes")
	}

	buf := &bytes.Buffer{}

	dataAtom := createDataAtom(value)

	// item size (header + data atom)
	itemSize := uint32(8 + len(dataAtom))
	binary.Write(buf, binary.BigEndian, itemSize)

	// item type (4 bytes)
	buf.Write(itemType)

	// data atom
	buf.Write(dataAtom)

	return buf.Bytes()
}

func TestParseMetadataTag_String(t *testing.T) {
	// Create ©nam (title) item - 0xA9 = © in MP4 tags
	data := createMetadataItem([]byte{0xA9, 'n', 'a', 'm'}, "Test Title")

	sr := audiobinary.NewSafeReader(bytes.NewReader(data), int64(len(data)), "test.m4b")
	atom, err := readAtomHeader(sr, 0)
	if err != nil {
		t.Fatalf("failed to read atom header: %v", err)
	}

	t.Logf("Atom type: %s, size: %d, data size: %d", atom.Type, atom.Size, atom.DataSize())

	value, err := parseMetadataTag(sr, atom)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "Test Title" {
		t.Errorf("expected 'Test Title', got '%s'", value)
	}
}

func TestParseMetadataTag_EmptyData(t *testing.T) {
	// Create item with no data atom
	buf := &bytes.Buffer{}
	itemSize := uint32(8) // just header
	binary.Write(buf, binary.BigEndian, itemSize)
	buf.Write([]byte{0xA9, 'n', 'a', 'm'})

	data := buf.Bytes()
	sr := audiobinary.NewSafeReader(bytes.NewReader(data), int64(len(data)), "test.m4b")
	atom, _ := readAtomHeader(sr, 0)

	value, err := parseMetadataTag(sr, atom)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "" {
		t.Errorf("expected empty string, got '%s'", value)
	}
}

func TestExtractIlstMetadata(t *testing.T) {
	// Create ilst atom with multiple items
	titleItem := createMetadataItem([]byte{0xA9, 'n', 'a', 'm'}, "My Book")      // ©nam
	artistItem := createMetadataItem([]byte{0xA9, 'A', 'R', 'T'}, "Author Name") // ©ART
	albumItem := createMetadataItem([]byte{0xA9, 'a', 'l', 'b'}, "Album Name")   // ©alb

	var ilstData []byte
	ilstData = append(ilstData, titleItem...)
	ilstData = append(ilstData, artistItem...)
	ilstData = append(ilstData, albumItem...)

	ilst := createMockAtom("ilst", ilstData)

	sr := audiobinary.NewSafeReader(bytes.NewReader(ilst), int64(len(ilst)), "test.m4b")
	ilstAtom, _ := readAtomHeader(sr, 0)

	meta := &audiometa.Metadata{}
	err := extractIlstMetadata(sr, ilstAtom, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.Title != "My Book" {
		t.Errorf("expected title 'My Book', got '%s'", meta.Title)
	}

	if meta.Artist != "Author Name" {
		t.Errorf("expected artist 'Author Name', got '%s'", meta.Artist)
	}

	if meta.Album != "Album Name" {
		t.Errorf("expected album 'Album Name', got '%s'", meta.Album)
	}
}

func TestMapTagToField(t *testing.T) {
	tests := []struct {
		tag      string
		value    string
		checkFn  func(*audiometa.Metadata) string
		expected string
	}{
		{"\xA9nam", "Title", func(m *audiometa.Metadata) string { return m.Title }, "Title"},     // ©nam
		{"\xA9ART", "Artist", func(m *audiometa.Metadata) string { return m.Artist }, "Artist"},  // ©ART
		{"\xA9alb", "Album", func(m *audiometa.Metadata) string { return m.Album }, "Album"},     // ©alb
		{"\xA9gen", "Genre", func(m *audiometa.Metadata) string { return m.Genre }, "Genre"},     // ©gen
		{"\xA9cmt", "Comment", func(m *audiometa.Metadata) string { return m.Comment }, "Comment"}, // ©cmt
	}

	for _, tt := range tests {
		meta := &audiometa.Metadata{}
		mapTagToField(tt.tag, tt.value, meta)

		got := tt.checkFn(meta)
		if got != tt.expected {
			t.Errorf("tag %s: expected '%s', got '%s'", tt.tag, tt.expected, got)
		}
	}
}