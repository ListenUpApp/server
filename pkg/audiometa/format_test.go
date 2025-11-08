package audiometa

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// createMockM4B creates a minimal valid M4B/M4A file header
func createMockM4B(brand string) []byte {
	buf := &bytes.Buffer{}

	// ftyp atom size (28 bytes)
	binary.Write(buf, binary.BigEndian, uint32(28))
	// ftyp atom type
	buf.WriteString("ftyp")
	// major brand
	buf.WriteString(brand)
	// minor version
	binary.Write(buf, binary.BigEndian, uint32(0))
	// compatible brands (just repeat the brand)
	buf.WriteString(brand)
	buf.WriteString(brand)

	return buf.Bytes()
}

// createInvalidFile creates a file with invalid ftyp
func createInvalidFile() []byte {
	buf := &bytes.Buffer{}
	// Invalid atom size
	binary.Write(buf, binary.BigEndian, uint32(8))
	// Wrong type
	buf.WriteString("XXXX")
	return buf.Bytes()
}

func TestDetectFormat_M4B(t *testing.T) {
	data := createMockM4B("M4B ")

	format, err := DetectFormat(bytes.NewReader(data), int64(len(data)), "test.m4b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if format != FormatM4B {
		t.Errorf("expected FormatM4B, got %v", format)
	}
}

func TestDetectFormat_M4A(t *testing.T) {
	data := createMockM4B("M4A ")

	format, err := DetectFormat(bytes.NewReader(data), int64(len(data)), "test.m4a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if format != FormatM4A {
		t.Errorf("expected FormatM4A, got %v", format)
	}
}

func TestDetectFormat_MP42(t *testing.T) {
	// mp42 is also valid M4A
	data := createMockM4B("mp42")

	format, err := DetectFormat(bytes.NewReader(data), int64(len(data)), "test.m4a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if format != FormatM4A {
		t.Errorf("expected FormatM4A for mp42, got %v", format)
	}
}

func TestDetectFormat_TooSmall(t *testing.T) {
	// File too small to contain ftyp
	data := []byte{0x00, 0x00}

	_, err := DetectFormat(bytes.NewReader(data), int64(len(data)), "tiny.m4b")
	if err == nil {
		t.Fatal("expected error for file too small")
	}

	if _, ok := err.(*UnsupportedFormatError); !ok {
		t.Errorf("expected UnsupportedFormatError, got %T", err)
	}
}

func TestDetectFormat_InvalidFtyp(t *testing.T) {
	data := createInvalidFile()

	_, err := DetectFormat(bytes.NewReader(data), int64(len(data)), "invalid.m4b")
	if err == nil {
		t.Fatal("expected error for invalid ftyp")
	}

	if _, ok := err.(*UnsupportedFormatError); !ok {
		t.Errorf("expected UnsupportedFormatError, got %T", err)
	}
}

func TestDetectFormat_UnsupportedBrand(t *testing.T) {
	// Create file with unsupported brand
	data := createMockM4B("XXXX")

	_, err := DetectFormat(bytes.NewReader(data), int64(len(data)), "unsupported.mp4")
	if err == nil {
		t.Fatal("expected error for unsupported brand")
	}

	if _, ok := err.(*UnsupportedFormatError); !ok {
		t.Errorf("expected UnsupportedFormatError, got %T", err)
	}
}

func TestFormat_String(t *testing.T) {
	tests := []struct {
		format   Format
		expected string
	}{
		{FormatM4B, "M4B"},
		{FormatM4A, "M4A"},
		{FormatUnknown, "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.format.String(); got != tt.expected {
			t.Errorf("Format(%d).String() = %s, want %s", tt.format, got, tt.expected)
		}
	}
}
