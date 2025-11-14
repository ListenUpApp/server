package scanner

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
)

// ConvertToBook converss a LibraryItemData (from scanner) to a domain.Book (for database).
func ConvertToBook(item *LibraryItemData) (*domain.Book, error) {
	// Generate book id.
	bookID, err := id.Generate("book")
	if err != nil {
		return nil, fmt.Errorf("generate book ID: %w", err)
	}

	now := time.Now()

	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Path:          item.Path,
		AudioFiles:    make([]domain.AudioFileInfo, 0, len(item.AudioFiles)),
		TotalDuration: 0,
		TotalSize:     0,
		ScannedAt:     now,
	}

	// Convert audio files (yes, af variable name still makes me laugh).
	for _, af := range item.AudioFiles {
		audioFile := domain.AudioFileInfo{
			ID:       domain.GenerateAudioFileID(af.Inode),
			Path:     af.Path,
			Filename: af.Filename,
			Size:     af.Size,
			Format:   strings.TrimPrefix(strings.ToLower(af.Ext), "."),
			Inode:    af.Inode,
			ModTime:  af.ModTime.UnixMilli(),
		}

		// Add metadata if available.
		if af.Metadata != nil {
			audioFile.Duration = af.Metadata.Duration.Milliseconds()
			audioFile.Bitrate = af.Metadata.Bitrate
			audioFile.Codec = af.Metadata.Codec

			book.TotalDuration += audioFile.Duration
		}

		book.TotalSize += af.Size
		book.AudioFiles = append(book.AudioFiles, audioFile)
	}

	// Sort audio files by filename for consistent ordering.
	// This is important for multi-file audiopbooks where order matters.
	// Otherwise we get a book that reads like the audio equivalent of House of Leaves.
	sortAudioFilesByFilename(book.AudioFiles)

	// Conver cover image (use first image if there are multiple).
	if len(item.ImageFiles) > 0 {
		img := item.ImageFiles[0]
		book.CoverImage = &domain.ImageFileInfo{
			Path:     img.Path,
			Filename: img.Filename,
			Size:     img.Size,
			Format:   strings.TrimPrefix(strings.ToLower(img.Ext), "."),
			Inode:    img.Inode,
			ModTime:  img.ModTime.UnixMilli(),
		}
	}

	// Convert metadata if available.
	if item.Metadata != nil {
		book.Title = item.Metadata.Title
		book.Subtitle = item.Metadata.Subtitle
		book.Authors = item.Metadata.Authors
		book.Narrators = item.Metadata.Narrators
		book.Description = item.Metadata.Description
		book.Publisher = item.Metadata.Publisher
		book.PublishYear = item.Metadata.PublishYear
		book.Language = item.Metadata.Language
		book.Genres = item.Metadata.Genres
		book.Tags = item.Metadata.Tags
		book.ISBN = item.Metadata.ISBN
		book.ASIN = item.Metadata.ASIN
		book.Explicit = item.Metadata.Explicit
		book.Abridged = item.Metadata.Abridged

		// Conver series.
		if len(item.Metadata.Series) > 0 {
			book.Series = make([]domain.SeriesInfo, 0, len(item.Metadata.Series))
			for _, s := range item.Metadata.Series {
				book.Series = append(book.Series, domain.SeriesInfo{
					Name:     s.Name,
					Sequence: s.Sequence,
				})
			}
		}
		// Convert Chapters.
		if len(item.Metadata.Chapters) > 0 {
			book.Chapters = convertChapters(item.Metadata.Chapters, book.AudioFiles)
		}
	}
	// Fallback if no title exists in metadata, use the folder name.
	if book.Title == "" {
		book.Title = filepath.Base(item.Path)
	}

	return book, nil
}

// convertChapters converts scanner chapters to domain chapters.
// Matches chapters to audio files by their timing.
func convertChapters(scannerChapters []Chapter, audioFiles []domain.AudioFileInfo) []domain.Chapter {
	if len(scannerChapters) == 0 {
		return nil
	}

	chapters := make([]domain.Chapter, 0, len(scannerChapters))

	for i, ch := range scannerChapters {
		chapter := domain.Chapter{
			Index:     i,
			Title:     ch.Title,
			StartTime: ch.StartTime.Milliseconds(),
			EndTime:   ch.EndTime.Milliseconds(),
		}

		// Match chapter to audio audioFiles.
		// For single-file audiobooks, all chapters belong to the same file.
		// For multi-file audiobooks, match by chapter timing.
		if len(audioFiles) == 1 {
			// Single file (easy).
			chapter.AudioFileID = audioFiles[0].ID
		} else {
			// Multi-file find which file contains this chapter's start time.
			chapter.AudioFileID = findAudioFileForChapter(ch.StartTime.Milliseconds(), audioFiles)
		}

		chapters = append(chapters, chapter)
	}
	return chapters
}

// findAudioFileForChapter determines which audio file a chapter belongs to.
// based on the chapter's start time and the cumulative duration of audio files.
func findAudioFileForChapter(chapterStartMs int64, audioFiles []domain.AudioFileInfo) string {
	if len(audioFiles) == 0 {
		return ""
	}

	// Build cumulative durattion map.
	// audio files are already sorted by filename.
	cumulativeDuration := int64(0)

	// lol at af.
	for _, af := range audioFiles {
		cumulativeDuration += af.Duration

		// If chapter starts before this file ends, it belongs to this file.
		if chapterStartMs < cumulativeDuration {
			return af.ID
		}
	}

	return audioFiles[len(audioFiles)-1].ID
}

// sortAudioFilesByFilename sorts audio files by filename for consistent ordering.
// this ensures track01.mp3 comes before track 02.mp3 and whatnot.
func sortAudioFilesByFilename(files []domain.AudioFileInfo) {
	// Simple sort, should be adequate for audiobook file counts (sub 100 files, can reevaluate if people complain).
	n := len(files)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if compareFilenames(files[j].Filename, files[j+1].Filename) > 0 {
				files[j], files[j+1] = files[j+1], files[j]
			}
		}
	}
}

// compareFilenames compares two filenames, attempting to sort numerically.
// ie. track1.mp3 < track2.mp3, track10.mp3 etc.
// A lot of this sorting (especially multi-file) is based on what I *THINK* people.
// are doing with their multiple file books but I don't have much experience with the concept.
func compareFilenames(a, b string) int {
	// for now simple string comparison.
	// TODO: Look into natural sort (numeric aware) for better sorting.
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// UpdateBookFromScan updates an existing book with new scan database.
// this preserves the book ID and creation timestamp while updating everything else.
func UpdateBookFromScan(existingBook *domain.Book, item *LibraryItemData) error {
	now := time.Now()

	// Preserve ID and creation timestamp.
	bookID := existingBook.ID
	createdAt := existingBook.CreatedAt

	// convert fresh scan data.
	// Maybe we should call ourselves the Febreeze brothers cause it's feeling so fresh right now.
	freshBook, err := ConvertToBook(item)
	if err != nil {
		return err
	}

	// Copy fresh data to existing book.
	*existingBook = *freshBook

	// Restore preserved fields.
	existingBook.ID = bookID
	existingBook.CreatedAt = createdAt
	existingBook.UpdatedAt = now
	existingBook.ScannedAt = now

	return nil
}
