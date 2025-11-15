package scanner

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
)

// Storer defines the interface for creating contributors and series during scanning.
type Storer interface {
	GetOrCreateContributorByName(ctx context.Context, name string) (*domain.Contributor, error)
	GetOrCreateSeriesByName(ctx context.Context, name string) (*domain.Series, error)
}

// ConvertToBook converts a LibraryItemData (from scanner) to a domain.Book (for database).
// It creates contributor and series entities as needed using the provided store.
func ConvertToBook(ctx context.Context, item *LibraryItemData, store Storer) (*domain.Book, error) {
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

		// Extract and create contributors
		contributors, err := extractContributors(ctx, item.Metadata, store)
		if err != nil {
			return nil, fmt.Errorf("extract contributors: %w", err)
		}
		book.Contributors = contributors

		// Extract and create series
		seriesID, sequence, err := extractSeries(ctx, item.Metadata, store)
		if err != nil {
			return nil, fmt.Errorf("extract series: %w", err)
		}
		book.SeriesID = seriesID
		book.Sequence = sequence

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
	sort.Slice(files, func(i, j int) bool {
		return compareFilenames(files[i].Filename, files[j].Filename) < 0
	})
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

// parseContributorString parses a contributor string that may contain role information.
// Handles patterns like:
//   - "Emily Wilson - translator"
//   - "Michael Kramer - narrator"
//   - "Brandon Sanderson" (no role)
//   - Multiple: "Kate Reading - narrator; Michael Kramer - narrator"
func parseContributorString(input string, defaultRole domain.ContributorRole) map[string][]domain.ContributorRole {
	result := make(map[string][]domain.ContributorRole)

	// Split by semicolon or comma for multiple contributors.
	var entries []string
	switch {
	case strings.Contains(input, ";"):
		entries = strings.Split(input, ";")
	case strings.Contains(input, ","):
		entries = strings.Split(input, ",")
	default:
		entries = []string{input}
	}

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		var name string
		var role domain.ContributorRole

		// Check for "Name - Role" pattern.
		if strings.Contains(entry, " - ") {
			parts := strings.SplitN(entry, " - ", 2)
			name = strings.TrimSpace(parts[0])
			roleStr := strings.ToLower(strings.TrimSpace(parts[1]))
			role = parseRoleString(roleStr)
		} else {
			name = entry
			role = defaultRole
		}

		if name != "" && role != "" {
			result[name] = append(result[name], role)
		}
	}

	return result
}

// roleMap maps common role string variations to standard ContributorRole values.
var roleMap = map[string]domain.ContributorRole{
	"author":          domain.RoleAuthor,
	"writer":          domain.RoleAuthor,
	"narrator":        domain.RoleNarrator,
	"reader":          domain.RoleNarrator,
	"read by":         domain.RoleNarrator,
	"translator":      domain.RoleTranslator,
	"translated by":   domain.RoleTranslator,
	"editor":          domain.RoleEditor,
	"edited by":       domain.RoleEditor,
	"foreword":        domain.RoleForeword,
	"foreword by":     domain.RoleForeword,
	"introduction":    domain.RoleIntroduction,
	"introduction by": domain.RoleIntroduction,
	"intro":           domain.RoleIntroduction,
	"afterword":       domain.RoleAfterword,
	"afterword by":    domain.RoleAfterword,
	"producer":        domain.RoleProducer,
	"adaptation":      domain.RoleAdapter,
	"adapted by":      domain.RoleAdapter,
	"adapter":         domain.RoleAdapter,
	"illustrator":     domain.RoleIllustrator,
	"illustrated by":  domain.RoleIllustrator,
}

// parseRoleString maps common role strings to ContributorRole.
func parseRoleString(roleStr string) domain.ContributorRole {
	roleStr = strings.ToLower(strings.TrimSpace(roleStr))

	if role, ok := roleMap[roleStr]; ok {
		return role
	}

	return ""
}

// extractContributors creates or retrieves contributors from metadata.
func extractContributors(ctx context.Context, metadata *BookMetadata, store Storer) ([]domain.BookContributor, error) {
	contributorMap := make(map[string][]domain.ContributorRole) // name -> roles

	// Extract authors (with role parsing).
	for _, author := range metadata.Authors {
		parsed := parseContributorString(author, domain.RoleAuthor)
		for name, roles := range parsed {
			contributorMap[name] = append(contributorMap[name], roles...)
		}
	}

	// Extract narrators (with role parsing).
	for _, narrator := range metadata.Narrators {
		parsed := parseContributorString(narrator, domain.RoleNarrator)
		for name, roles := range parsed {
			contributorMap[name] = append(contributorMap[name], roles...)
		}
	}

	// Create or fetch contributors
	result := make([]domain.BookContributor, 0, len(contributorMap))
	for name, roles := range contributorMap {
		contributor, err := store.GetOrCreateContributorByName(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("get/create contributor %s: %w", name, err)
		}

		result = append(result, domain.BookContributor{
			ContributorID: contributor.ID,
			Roles:         roles,
		})
	}

	return result, nil
}

// extractSeries creates or retrieves series from metadata.
func extractSeries(ctx context.Context, metadata *BookMetadata, store Storer) (seriesID, sequence string, err error) {
	if len(metadata.Series) == 0 {
		return "", "", nil
	}

	// Use first series (most books are in one series)
	seriesInfo := metadata.Series[0]

	series, err := store.GetOrCreateSeriesByName(ctx, seriesInfo.Name)
	if err != nil {
		return "", "", fmt.Errorf("get/create series: %w", err)
	}

	return series.ID, seriesInfo.Sequence, nil
}

// UpdateBookFromScan updates an existing book with new scan database.
// this preserves the book ID and creation timestamp while updating everything else.
func UpdateBookFromScan(ctx context.Context, existingBook *domain.Book, item *LibraryItemData, store Storer) error {
	now := time.Now()

	// Preserve ID and creation timestamp.
	bookID := existingBook.ID
	createdAt := existingBook.CreatedAt

	// convert fresh scan data.
	// Maybe we should call ourselves the Febreeze brothers cause it's feeling so fresh right now.
	freshBook, err := ConvertToBook(ctx, item, store)
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
