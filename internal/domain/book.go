// Package domain contains the core business entities and domain logic for the ListenUp audiobook library.
package domain

import (
	"fmt"
	"time"
)

// BookSeries links a book to a series with its sequence position.
// A book can belong to multiple series (e.g., "Mistborn", "Mistborn Era 1", "The Cosmere").
type BookSeries struct {
	SeriesID string `json:"series_id"`
	Sequence string `json:"sequence,omitempty"` // "1", "1.5", "Book Zero" - flexible for edge cases
}

// Book represents an audiobook in the library.
type Book struct {
	Syncable
	ScannedAt     time.Time         `json:"scanned_at"`
	CoverImage    *ImageFileInfo    `json:"cover_image,omitempty"`
	ISBN          string            `json:"isbn,omitempty"`
	Title         string            `json:"title"`
	Subtitle      string            `json:"subtitle,omitempty"`
	Path          string            `json:"path"`
	Description   string            `json:"description,omitempty"`
	Publisher     string            `json:"publisher,omitempty"`
	PublishYear   string            `json:"publish_year,omitempty"`
	Language      string            `json:"language,omitempty"`
	ASIN          string            `json:"asin,omitempty"`
	AudibleRegion string            `json:"audible_region,omitempty"` // Region used for ASIN lookup
	GenreIDs      []string          `json:"genre_ids,omitempty"`      // References to normalized Genre entities
	Contributors  []BookContributor `json:"contributors"`
	Series        []BookSeries      `json:"series,omitempty"` // Multiple series with sequence per series
	AudioFiles    []AudioFileInfo   `json:"audio_files"`
	Chapters      []Chapter         `json:"chapters,omitempty"`
	TotalDuration int64             `json:"total_duration"`
	TotalSize     int64             `json:"total_size"`
	Abridged      bool              `json:"abridged,omitempty"`
}

// AudioFileInfo represents an audio file within a book.
type AudioFileInfo struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Format   string `json:"format"`
	Codec    string `json:"codec,omitempty"`
	Size     int64  `json:"size"`
	Duration int64  `json:"duration"`
	Bitrate  int    `json:"bitrate,omitempty"`
	Inode    uint64 `json:"inode"`
	ModTime  int64  `json:"mod_time"`
}

// ImageFileInfo represents an image file (cover art).
type ImageFileInfo struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Format   string `json:"format"`
	Size     int64  `json:"size"`
	Inode    uint64 `json:"inode"`
	ModTime  int64  `json:"mod_time"`
	BlurHash string `json:"blur_hash,omitempty"`
}

// Chapter represents a chapter marker within an audiobook.
type Chapter struct {
	Title       string `json:"title"`
	AudioFileID string `json:"audio_file_id"`
	Index       int    `json:"index"`
	StartTime   int64  `json:"start_time"`
	EndTime     int64  `json:"end_time"`
}

// Helper Methods.

// GetAudioFileByID finds an audio file by its ID.
func (b *Book) GetAudioFileByID(id string) *AudioFileInfo {
	for i := range b.AudioFiles {
		if b.AudioFiles[i].ID == id {
			return &b.AudioFiles[i]
		}
	}
	return nil
}

// GetAudioFileByInode finds an audio file by its inode.
// Used during rescans to match files after renames.
func (b *Book) GetAudioFileByInode(inode uint64) *AudioFileInfo {
	for i := range b.AudioFiles {
		if b.AudioFiles[i].Inode == inode {
			return &b.AudioFiles[i]
		}
	}
	return nil
}

// UpdateAudioFile updates an existing audio file or adds it if not found.
// Returns true if this was an update (ie. file existed), false if it was added.
func (b *Book) UpdateAudioFile(file AudioFileInfo) bool {
	// try to find by inode first (which handles renames).
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

// RemoveAudioFileByInode removes an audio file by inode.
// Returns true if a file was removed.
func (b *Book) RemoveAudioFileByInode(inode uint64) bool {
	for i := range b.AudioFiles {
		if b.AudioFiles[i].Inode == inode {
			// remove from slice.
			b.AudioFiles = append(b.AudioFiles[:i], b.AudioFiles[i+1:]...)
			return true
		}
	}
	return false
}

// RecalculateTotals recalculates total duration and size from audio files.
func (b *Book) RecalculateTotals() {
	b.TotalDuration = 0
	b.TotalSize = 0

	// Not related to the code at all. But the 'af' variable name makes me laugh. And I will never change it.
	for _, af := range b.AudioFiles {
		b.TotalDuration += af.Duration
		b.TotalSize += af.Size
	}
}

// GenerateAudioFileID creates a stable ID from an inode.
// Format: "af-{hex}" where hexx is the inode in hexadecimal notation.
// This ensures the same file always gets the same ID, even after renames :tada:.
func GenerateAudioFileID(inode uint64) string {
	return fmt.Sprintf("af-%x", inode)
}
