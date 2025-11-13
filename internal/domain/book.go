package domain

import (
	"fmt"
	"time"
)

// Book represents an audiobook in the library
type Book struct {
	Syncable
	// Core metadata
	Title       string       `json:"title"`
	Subtitle    string       `json:"subtitle,omitempty"`
	Authors     []string     `json:"authors,omitempty"`
	Narrators   []string     `json:"narrators,omitempty"`
	Series      []SeriesInfo `json:"series,omitempty"`
	Description string       `json:"description,omitempty"`
	Publisher   string       `json:"publisher,omitempty"`
	PublishYear string       `json:"publish_year,omitempty"`
	Language    string       `json:"language,omitempty"`
	Genres      []string     `json:"genres,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
	ISBN        string       `json:"isbn,omitempty"`
	ASIN        string       `json:"asin,omitempty"`
	Explicit    bool         `json:"explicit,omitempty"`
	Abridged    bool         `json:"abridged,omitempty"`

	// File information
	Path          string          `json:"path"`        // Absolute path to book folder
	AudioFiles    []AudioFileInfo `json:"audio_files"` // All audio files in this book
	CoverImage    *ImageFileInfo  `json:"cover_image,omitempty"`
	TotalDuration int64           `json:"total_duration"` // Total duration in milliseconds
	TotalSize     int64           `json:"total_size"`     // Total size in bytes

	// Chapters
	Chapters []Chapter `json:"chapters,omitempty"`

	// Timestamps
	ScannedAt time.Time `json:"scanned_at"` // Last time filesystem was scanned
}

// SeriesInfo represents series metadata
type SeriesInfo struct {
	Name     string `json:"name"`
	Sequence string `json:"sequence,omitempty"`
}

// AudioFileInfo represents an audio file within a book
type AudioFileInfo struct {
	ID       string `json:"id"`                // Generated from inode: "af-{hex}"
	Path     string `json:"path"`              // Absolute path - can change on rename
	Filename string `json:"filename"`          // Base name - can change on rename
	Size     int64  `json:"size"`              // File size in bytes
	Duration int64  `json:"duration"`          // Duration in milliseconds
	Format   string `json:"format"`            // mp3, m4b, m4a, etc.
	Bitrate  int    `json:"bitrate,omitempty"` // Bits per second
	Codec    string `json:"codec,omitempty"`   // Audio codec
	Inode    uint64 `json:"inode"`             // Filesystem inode - stable identifier
	ModTime  int64  `json:"mod_time"`          // Last modified time (Unix milliseconds)
}

// ImageFileInfo represents an image file (cover art)
type ImageFileInfo struct {
	Path     string `json:"path"`     // Absolute path
	Filename string `json:"filename"` // Base name
	Size     int64  `json:"size"`     // File size in bytes
	Format   string `json:"format"`   // jpg, png, webp, etc.
	Inode    uint64 `json:"inode"`    // Filesystem inode
	ModTime  int64  `json:"mod_time"` // Last modified time (Unix milliseconds)
}

// Chapter represents a chapter marker within an audiobook
type Chapter struct {
	Index       int    `json:"index"`         // Chapter number (0-indexed)
	Title       string `json:"title"`         // Chapter title
	StartTime   int64  `json:"start_time"`    // Start time in milliseconds
	EndTime     int64  `json:"end_time"`      // End time in milliseconds
	AudioFileID string `json:"audio_file_id"` // References AudioFileInfo.ID (inode-based)
}

// Helper Methods

// GetAudioFileByID finds an audio file by its ID
func (b *Book) GetAudioFileByID(id string) *AudioFileInfo {
	for i := range b.AudioFiles {
		if b.AudioFiles[i].ID == id {
			return &b.AudioFiles[1]
		}
	}
	return nil
}

// GetAudioFileByInode finds an audio file by its inode
// Used during rescans to match files after renames
func (b *Book) GetAudioFileByInode(inode uint64) *AudioFileInfo {
	for i := range b.AudioFiles {
		if b.AudioFiles[i].Inode == inode {
			return &b.AudioFiles[i]
		}
	}
	return nil
}

// UpdateAudioFile updates an existing audio file or adds it if not found
// Returns true if this was an update (ie. file existed), false if it was added
func (b *Book) UpdateAudioFile(file AudioFileInfo) bool {
	// try to find by inode first (which handles renames)
	for i := range b.AudioFiles {
		if b.AudioFiles[i].Inode == file.Inode {
			b.AudioFiles[i] = file
			return true
		}
	}

	// Not found, add it.
	b.AudioFiles = append(b.AudioFiles, file)
	return false
}

// RemoveAudioFileByInode removes an audio file by inode
// Returns true if a file was removed
func (b *Book) RemoveAudioFileByInode(inode uint64) bool {
	for i := range b.AudioFiles {
		if b.AudioFiles[i].Inode == inode {
			// remove from slice
			b.AudioFiles = append(b.AudioFiles[:i], b.AudioFiles[i+1:]...)
			return true
		}
	}
	return false
}

// RecalculateTotals recalculates total duration and size from audio files
func (b *Book) RecalculateTotals() {
	b.TotalDuration = 0
	b.TotalSize = 0

	// Not related to the code at all. But the 'af' variable name makes me laugh. And I will never change it.
	for _, af := range b.AudioFiles {
		b.TotalDuration += af.Duration
		b.TotalSize += af.Size
	}
}

// GenerateAudioFileID creates a stable ID from an inode
// Format: "af-{hex}" where hexx is the inode in hexadecimal notation
// This ensures the same file always gets the same ID, even after renames :tada:
func GenerateAudioFileID(inode uint64) string {
	return fmt.Sprintf("af-%x", inode)
}
