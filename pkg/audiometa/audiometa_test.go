package audiometa_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/listenupapp/listenup-server/pkg/audiometa"
	"github.com/listenupapp/listenup-server/pkg/audiometa/m4a"
)

// createSimpleM4B creates a minimal M4B for testing
// This duplicates some logic from m4a/parser_test.go but keeps the public API tests independent
func createSimpleM4B() []byte {
	buf := &bytes.Buffer{}

	// ftyp atom
	ftypBuf := &bytes.Buffer{}
	ftypBuf.WriteString("M4B ")
	binary.Write(ftypBuf, binary.BigEndian, uint32(0))
	ftypBuf.WriteString("M4B ")

	// ftyp atom size
	ftypSize := uint32(8 + ftypBuf.Len())
	binary.Write(buf, binary.BigEndian, ftypSize)
	buf.WriteString("ftyp")
	buf.Write(ftypBuf.Bytes())

	// Simple moov atom (empty)
	binary.Write(buf, binary.BigEndian, uint32(8))
	buf.WriteString("moov")

	return buf.Bytes()
}

func TestParse_M4B(t *testing.T) {
	data := createSimpleM4B()

	tmpFile, err := os.CreateTemp("", "test*.m4b")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.Write(data)
	tmpFile.Close()

	// For Phase 1, use m4a.Parse directly
	meta, err := m4a.Parse(tmpFile.Name())
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if meta.Format != audiometa.FormatM4B {
		t.Errorf("expected FormatM4B, got %v", meta.Format)
	}
}

func TestParse_FileNotFound(t *testing.T) {
	_, err := m4a.Parse("/nonexistent/path.m4b")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParse_UnsupportedFormat(t *testing.T) {
	// Create a file with unsupported format
	tmpFile, err := os.CreateTemp("", "test*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Write some random data
	tmpFile.Write([]byte("not a valid M4B file"))
	tmpFile.Close()

	_, err = m4a.Parse(tmpFile.Name())
	if err == nil {
		t.Error("expected error for unsupported format")
	}

	if _, ok := err.(*audiometa.UnsupportedFormatError); !ok {
		t.Errorf("expected UnsupportedFormatError, got %T", err)
	}
}
