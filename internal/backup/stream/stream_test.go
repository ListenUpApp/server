package stream

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type testEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestWriterReader_RoundTrip(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")

	// Write entities
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)

	w, err := NewWriter(zw, "entities/test.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	entities := []testEntity{
		{ID: "1", Name: "First"},
		{ID: "2", Name: "Second"},
		{ID: "3", Name: "Third"},
	}

	for _, e := range entities {
		if err := w.Write(e); err != nil {
			t.Fatal(err)
		}
	}

	if w.Count() != 3 {
		t.Errorf("Count() = %d, want 3", w.Count())
	}

	zw.Close()
	f.Close()

	// Read entities back
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	rc, err := OpenFile(zr, "entities/test.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	reader := NewReader[testEntity](rc)

	var got []testEntity
	for entity, err := range reader.All() {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, entity)
	}

	if len(got) != len(entities) {
		t.Errorf("got %d entities, want %d", len(got), len(entities))
	}

	for i, e := range got {
		if e.ID != entities[i].ID || e.Name != entities[i].Name {
			t.Errorf("entity %d: got %+v, want %+v", i, e, entities[i])
		}
	}
}

func TestOpenFile_NotFound(t *testing.T) {
	// Create empty zip in temp file
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "empty.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)
	zw.Close()
	f.Close()

	// Open and try to find nonexistent file
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	_, err = OpenFile(zr, "nonexistent.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReader_ContinuesOnParseError(t *testing.T) {
	// Create JSONL with one bad line
	jsonl := `{"id":"1","name":"Good"}
{bad json}
{"id":"2","name":"Also Good"}
`
	rc := io.NopCloser(bytes.NewReader([]byte(jsonl)))
	reader := NewReader[testEntity](rc)

	var good []testEntity
	var errors int

	for entity, err := range reader.All() {
		if err != nil {
			errors++
			continue
		}
		good = append(good, entity)
	}

	if len(good) != 2 {
		t.Errorf("got %d good entities, want 2", len(good))
	}
	if errors != 1 {
		t.Errorf("got %d errors, want 1", errors)
	}
}
