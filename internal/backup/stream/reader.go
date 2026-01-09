package stream

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"io"
	"iter"

	"encoding/json/v2"
)

// ErrFileNotFound indicates a file was not found in the backup archive.
var ErrFileNotFound = errors.New("file not found in backup")

// OpenFile finds and opens a file from a zip archive.
func OpenFile(zr *zip.ReadCloser, path string) (io.ReadCloser, error) {
	for _, f := range zr.File {
		if f.Name == path {
			return f.Open()
		}
	}
	return nil, ErrFileNotFound
}

// Reader streams entities from a JSONL file in a zip archive.
type Reader[T any] struct {
	rc      io.ReadCloser
	scanner *bufio.Scanner
}

// NewReader creates a streaming reader for type T.
func NewReader[T any](rc io.ReadCloser) *Reader[T] {
	return &Reader[T]{
		rc:      rc,
		scanner: bufio.NewScanner(rc),
	}
}

// All returns an iterator over all entities in the file.
func (r *Reader[T]) All() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		defer r.rc.Close()

		for r.scanner.Scan() {
			line := r.scanner.Bytes()
			if len(line) == 0 {
				continue // Skip empty lines
			}

			var entity T
			if err := json.UnmarshalRead(bytes.NewReader(line), &entity); err != nil {
				var zero T
				if !yield(zero, err) {
					return
				}
				continue // Try next line on parse error
			}
			if !yield(entity, nil) {
				return
			}
		}

		if err := r.scanner.Err(); err != nil {
			var zero T
			yield(zero, err)
		}
	}
}
