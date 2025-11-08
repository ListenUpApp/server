package m4a

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
)

// createMinimalM4B creates a minimal M4B file with ftyp, moov, udta, meta, and ilst atoms
func createMinimalM4B(title, artist, album string) []byte {
	buf := &bytes.Buffer{}

	// 1. ftyp atom
	ftypBuf := &bytes.Buffer{}
	ftypBuf.WriteString("M4B ")                        // major brand
	binary.Write(ftypBuf, binary.BigEndian, uint32(0)) // minor version
	ftypBuf.WriteString("M4B ")                        // compatible brand
	ftypAtom := createMockAtom("ftyp", ftypBuf.Bytes())
	buf.Write(ftypAtom)

	// 2. Build metadata atoms from inside out: ilst → meta → udta → moov

	// Create ilst with metadata items
	var ilstData []byte
	if title != "" {
		ilstData = append(ilstData, createMetadataItem([]byte{0xA9, 'n', 'a', 'm'}, title)...)
	}
	if artist != "" {
		ilstData = append(ilstData, createMetadataItem([]byte{0xA9, 'A', 'R', 'T'}, artist)...)
	}
	if album != "" {
		ilstData = append(ilstData, createMetadataItem([]byte{0xA9, 'a', 'l', 'b'}, album)...)
	}
	ilstAtom := createMockAtom("ilst", ilstData)

	// meta atom contains ilst
	// meta atom has 4 bytes of version+flags before the data
	metaData := make([]byte, 4)
	binary.BigEndian.PutUint32(metaData, 0) // version=0, flags=0
	metaData = append(metaData, ilstAtom...)
	metaAtom := createMockAtom("meta", metaData)

	// udta contains meta
	udtaAtom := createMockAtom("udta", metaAtom)

	// moov contains udta
	moovAtom := createMockAtom("moov", udtaAtom)

	buf.Write(moovAtom)

	return buf.Bytes()
}

func TestParse_Success(t *testing.T) {
	data := createMinimalM4B("My Audiobook", "Author Name", "Series 1")

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "test*.m4b")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Parse the file
	meta, err := Parse(tmpFile.Name())
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}

	if meta.Title != "My Audiobook" {
		t.Errorf("expected title 'My Audiobook', got '%s'", meta.Title)
	}

	if meta.Artist != "Author Name" {
		t.Errorf("expected artist 'Author Name', got '%s'", meta.Artist)
	}

	if meta.Album != "Series 1" {
		t.Errorf("expected album 'Series 1', got '%s'", meta.Album)
	}

	if meta.Format != audiometa.FormatM4B {
		t.Errorf("expected format M4B, got %v", meta.Format)
	}

	if meta.FileSize == 0 {
		t.Error("expected file size to be set")
	}
}

func TestParse_FileNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/file.m4b")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParse_UnsupportedFormat(t *testing.T) {
	// Create a file with wrong ftyp
	data := createMockAtom("ftyp", []byte("XXXX")) // Wrong brand

	tmpFile, err := os.CreateTemp("", "test*.m4b")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.Write(data)
	tmpFile.Close()

	_, err = Parse(tmpFile.Name())
	if err == nil {
		t.Error("expected error for unsupported format")
	}

	if _, ok := err.(*audiometa.UnsupportedFormatError); !ok {
		t.Errorf("expected UnsupportedFormatError, got %T", err)
	}
}

func TestParse_NoMetadata(t *testing.T) {
	// Create M4B with no metadata
	data := createMinimalM4B("", "", "")

	tmpFile, err := os.CreateTemp("", "test*.m4b")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.Write(data)
	tmpFile.Close()

	meta, err := Parse(tmpFile.Name())
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should succeed but have empty metadata
	if meta.Title != "" || meta.Artist != "" || meta.Album != "" {
		t.Error("expected empty metadata")
	}
}
