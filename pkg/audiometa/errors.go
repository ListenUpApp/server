// Package audiometa provides high-performance audio metadata parsing
package audiometa

import "fmt"

// OutOfBoundsError is returned when attempting to read beyond file bounds
type OutOfBoundsError struct {
	Path   string
	Offset int64
	Length int
	Size   int64
	What   string // Context: what was being read
}

func (e *OutOfBoundsError) Error() string {
	if e.Offset >= e.Size {
		return fmt.Sprintf("%s: offset %d out of bounds (file size: %d) while reading %s",
			e.Path, e.Offset, e.Size, e.What)
	}
	return fmt.Sprintf("%s: read of %d bytes at offset %d would exceed file size %d while reading %s",
		e.Path, e.Length, e.Offset, e.Size, e.What)
}

// UnsupportedFormatError is returned when the file format is not M4B/M4A
type UnsupportedFormatError struct {
	Path   string
	Reason string
}

func (e *UnsupportedFormatError) Error() string {
	return fmt.Sprintf("%s: unsupported format: %s", e.Path, e.Reason)
}

// CorruptedFileError is returned when file structure is invalid
type CorruptedFileError struct {
	Path   string
	Offset int64
	Reason string
}

func (e *CorruptedFileError) Error() string {
	return fmt.Sprintf("%s: corrupted file at offset %d: %s", e.Path, e.Offset, e.Reason)
}
