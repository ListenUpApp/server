// Package stream provides JSONL streaming to/from zip archives.
package stream

import (
	"archive/zip"
	"io"

	"encoding/json/v2"
)

// Writer streams entities as JSONL to a zip archive.
type Writer struct {
	zw    *zip.Writer
	w     io.Writer
	count int
}

// NewWriter creates a JSONL writer for a path within the zip.
func NewWriter(zw *zip.Writer, path string) (*Writer, error) {
	w, err := zw.Create(path)
	if err != nil {
		return nil, err
	}

	return &Writer{
		zw: zw,
		w:  w,
	}, nil
}

// Write encodes a single entity as a JSON line.
func (w *Writer) Write(entity any) error {
	if err := json.MarshalWrite(w.w, entity); err != nil {
		return err
	}
	// Add newline for JSONL format
	if _, err := w.w.Write([]byte{'\n'}); err != nil {
		return err
	}
	w.count++
	return nil
}

// Count returns entities written so far.
func (w *Writer) Count() int {
	return w.count
}
