package scanner

import (
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertToBook_WithFullMetadata tests converting a library item with complete metadata.
func TestConvertToBook_WithFullMetadata(t *testing.T) {
	now := time.Now()
	item := &LibraryItemData{
		Path:    "/audiobooks/test-book",
		RelPath: "test-book",
		ModTime: now,
		Inode:   1000,
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/test-book/track01.mp3",
				Filename: "track01.mp3",
				Ext:      ".mp3",
				Size:     1024000,
				ModTime:  now,
				Inode:    1001,
				Metadata: &AudioMetadata{
					Duration: 5 * time.Minute,
					Bitrate:  128000,
					Codec:    "mp3",
				},
			},
		},
		ImageFiles: []ImageFileData{
			{
				Path:     "/audiobooks/test-book/cover.jpg",
				Filename: "cover.jpg",
				Ext:      ".jpg",
				Size:     50000,
				ModTime:  now,
				Inode:    1002,
			},
		},
		Metadata: &BookMetadata{
			Title:       "Test Book",
			Subtitle:    "A Test Subtitle",
			Authors:     []string{"Author One", "Author Two"},
			Narrators:   []string{"Narrator One"},
			Description: "This is a test book",
			Publisher:   "Test Publisher",
			PublishYear: "2024",
			Language:    "en",
			Genres:      []string{"Fiction", "Mystery"},
			Tags:        []string{"thriller", "suspense"},
			ISBN:        "1234567890",
			ASIN:        "B00TEST",
			Explicit:    false,
			Abridged:    false,
		},
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)
	assert.NotEmpty(t, book.ID)
	assert.True(t, book.ID[:5] == "book-")
	assert.Equal(t, "/audiobooks/test-book", book.Path)
	assert.Equal(t, "Test Book", book.Title)
	assert.Equal(t, "A Test Subtitle", book.Subtitle)
	assert.Equal(t, []string{"Author One", "Author Two"}, book.Authors)
	assert.Equal(t, []string{"Narrator One"}, book.Narrators)
	assert.Equal(t, "This is a test book", book.Description)
	assert.Equal(t, "Test Publisher", book.Publisher)
	assert.Equal(t, "2024", book.PublishYear)
	assert.Equal(t, "en", book.Language)
	assert.Equal(t, []string{"Fiction", "Mystery"}, book.Genres)
	assert.Equal(t, []string{"thriller", "suspense"}, book.Tags)
	assert.Equal(t, "1234567890", book.ISBN)
	assert.Equal(t, "B00TEST", book.ASIN)
	assert.False(t, book.Explicit)
	assert.False(t, book.Abridged)

	// Check audio file conversion.
	require.Len(t, book.AudioFiles, 1)
	assert.Equal(t, domain.GenerateAudioFileID(1001), book.AudioFiles[0].ID)
	assert.Equal(t, "/audiobooks/test-book/track01.mp3", book.AudioFiles[0].Path)
	assert.Equal(t, "track01.mp3", book.AudioFiles[0].Filename)
	assert.Equal(t, int64(1024000), book.AudioFiles[0].Size)
	assert.Equal(t, "mp3", book.AudioFiles[0].Format)
	assert.Equal(t, uint64(1001), book.AudioFiles[0].Inode)
	assert.Equal(t, int64(300000), book.AudioFiles[0].Duration) // 5 minutes in ms
	assert.Equal(t, 128000, book.AudioFiles[0].Bitrate)
	assert.Equal(t, "mp3", book.AudioFiles[0].Codec)

	// Check totals.
	assert.Equal(t, int64(300000), book.TotalDuration)
	assert.Equal(t, int64(1024000), book.TotalSize)

	// Check cover image.
	require.NotNil(t, book.CoverImage)
	assert.Equal(t, "/audiobooks/test-book/cover.jpg", book.CoverImage.Path)
	assert.Equal(t, "cover.jpg", book.CoverImage.Filename)
	assert.Equal(t, int64(50000), book.CoverImage.Size)
	assert.Equal(t, "jpg", book.CoverImage.Format)
	assert.Equal(t, uint64(1002), book.CoverImage.Inode)

	// Check timestamps.
	assert.False(t, book.CreatedAt.IsZero())
	assert.False(t, book.UpdatedAt.IsZero())
	assert.False(t, book.ScannedAt.IsZero())
}

// TestConvertToBook_WithoutMetadata tests converting without metadata (uses folder name).
func TestConvertToBook_WithoutMetadata(t *testing.T) {
	now := time.Now()
	item := &LibraryItemData{
		Path:    "/audiobooks/My Awesome Book",
		RelPath: "My Awesome Book",
		ModTime: now,
		Inode:   1000,
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/My Awesome Book/file.mp3",
				Filename: "file.mp3",
				Ext:      ".mp3",
				Size:     1024000,
				ModTime:  now,
				Inode:    1001,
			},
		},
		Metadata: nil,
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)
	assert.Equal(t, "My Awesome Book", book.Title) // Fallback to folder name
	assert.Empty(t, book.Subtitle)
	assert.Empty(t, book.Authors)
	assert.Empty(t, book.Narrators)
}

// TestConvertToBook_MultipleAudioFiles tests converting with multiple audio files.
func TestConvertToBook_MultipleAudioFiles(t *testing.T) {
	now := time.Now()
	item := &LibraryItemData{
		Path:    "/audiobooks/test",
		RelPath: "test",
		ModTime: now,
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/test/track02.mp3",
				Filename: "track02.mp3",
				Ext:      ".mp3",
				Size:     2000000,
				ModTime:  now,
				Inode:    2002,
				Metadata: &AudioMetadata{Duration: 10 * time.Minute},
			},
			{
				Path:     "/audiobooks/test/track01.mp3",
				Filename: "track01.mp3",
				Ext:      ".mp3",
				Size:     1000000,
				ModTime:  now,
				Inode:    2001,
				Metadata: &AudioMetadata{Duration: 5 * time.Minute},
			},
			{
				Path:     "/audiobooks/test/track03.mp3",
				Filename: "track03.mp3",
				Ext:      ".mp3",
				Size:     3000000,
				ModTime:  now,
				Inode:    2003,
				Metadata: &AudioMetadata{Duration: 15 * time.Minute},
			},
		},
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)

	// Should be sorted by filename.
	require.Len(t, book.AudioFiles, 3)
	assert.Equal(t, "track01.mp3", book.AudioFiles[0].Filename)
	assert.Equal(t, "track02.mp3", book.AudioFiles[1].Filename)
	assert.Equal(t, "track03.mp3", book.AudioFiles[2].Filename)

	// Total duration should be sum of all files.
	assert.Equal(t, int64(1800000), book.TotalDuration) // 30 minutes in ms
	assert.Equal(t, int64(6000000), book.TotalSize)
}

// TestConvertToBook_WithSeries tests converting with series information.
func TestConvertToBook_WithSeries(t *testing.T) {
	item := &LibraryItemData{
		Path: "/audiobooks/test",
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/test/file.mp3",
				Filename: "file.mp3",
				Ext:      ".mp3",
				Size:     1000,
				Inode:    1001,
			},
		},
		Metadata: &BookMetadata{
			Title: "Book Three",
			Series: []SeriesInfo{
				{Name: "The Great Series", Sequence: "3"},
				{Name: "Another Series", Sequence: "1"},
			},
		},
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)
	require.Len(t, book.Series, 2)
	assert.Equal(t, "The Great Series", book.Series[0].Name)
	assert.Equal(t, "3", book.Series[0].Sequence)
	assert.Equal(t, "Another Series", book.Series[1].Name)
	assert.Equal(t, "1", book.Series[1].Sequence)
}

// TestConvertToBook_WithChapters_SingleFile tests chapter conversion for single-file audiobook.
func TestConvertToBook_WithChapters_SingleFile(t *testing.T) {
	item := &LibraryItemData{
		Path: "/audiobooks/test",
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/test/book.m4b",
				Filename: "book.m4b",
				Ext:      ".m4b",
				Size:     10000000,
				Inode:    1001,
				Metadata: &AudioMetadata{Duration: 60 * time.Minute},
			},
		},
		Metadata: &BookMetadata{
			Title: "Test Book",
			Chapters: []Chapter{
				{ID: 1, Title: "Chapter 1", StartTime: 0, EndTime: 10 * time.Minute},
				{ID: 2, Title: "Chapter 2", StartTime: 10 * time.Minute, EndTime: 25 * time.Minute},
				{ID: 3, Title: "Chapter 3", StartTime: 25 * time.Minute, EndTime: 60 * time.Minute},
			},
		},
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)
	require.Len(t, book.Chapters, 3)

	// All chapters should reference the single audio file.
	audioFileID := book.AudioFiles[0].ID
	assert.Equal(t, 0, book.Chapters[0].Index)
	assert.Equal(t, "Chapter 1", book.Chapters[0].Title)
	assert.Equal(t, int64(0), book.Chapters[0].StartTime)
	assert.Equal(t, int64(600000), book.Chapters[0].EndTime)
	assert.Equal(t, audioFileID, book.Chapters[0].AudioFileID)

	assert.Equal(t, 1, book.Chapters[1].Index)
	assert.Equal(t, audioFileID, book.Chapters[1].AudioFileID)

	assert.Equal(t, 2, book.Chapters[2].Index)
	assert.Equal(t, audioFileID, book.Chapters[2].AudioFileID)
}

// TestConvertToBook_WithChapters_MultiFile tests chapter conversion for multi-file audiobook.
func TestConvertToBook_WithChapters_MultiFile(t *testing.T) {
	item := &LibraryItemData{
		Path: "/audiobooks/test",
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/test/part1.mp3",
				Filename: "part1.mp3",
				Ext:      ".mp3",
				Size:     1000000,
				Inode:    1001,
				Metadata: &AudioMetadata{Duration: 20 * time.Minute},
			},
			{
				Path:     "/audiobooks/test/part2.mp3",
				Filename: "part2.mp3",
				Ext:      ".mp3",
				Size:     2000000,
				Inode:    1002,
				Metadata: &AudioMetadata{Duration: 30 * time.Minute},
			},
		},
		Metadata: &BookMetadata{
			Title: "Test Book",
			Chapters: []Chapter{
				{ID: 1, Title: "Chapter 1", StartTime: 0, EndTime: 10 * time.Minute},
				{ID: 2, Title: "Chapter 2", StartTime: 10 * time.Minute, EndTime: 20 * time.Minute},
				{ID: 3, Title: "Chapter 3", StartTime: 20 * time.Minute, EndTime: 35 * time.Minute},
				{ID: 4, Title: "Chapter 4", StartTime: 35 * time.Minute, EndTime: 50 * time.Minute},
			},
		},
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)
	require.Len(t, book.Chapters, 4)

	file1ID := book.AudioFiles[0].ID
	file2ID := book.AudioFiles[1].ID

	// Chapters 1 & 2 in first file (0-20 minutes).
	assert.Equal(t, file1ID, book.Chapters[0].AudioFileID)
	assert.Equal(t, file1ID, book.Chapters[1].AudioFileID)

	// Chapters 3 & 4 in second file (20-50 minutes).
	assert.Equal(t, file2ID, book.Chapters[2].AudioFileID)
	assert.Equal(t, file2ID, book.Chapters[3].AudioFileID)
}

// TestConvertToBook_NoCoverImage tests conversion without cover image.
func TestConvertToBook_NoCoverImage(t *testing.T) {
	item := &LibraryItemData{
		Path: "/audiobooks/test",
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/test/file.mp3",
				Filename: "file.mp3",
				Ext:      ".mp3",
				Size:     1000,
				Inode:    1001,
			},
		},
		ImageFiles: []ImageFileData{},
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)
	assert.Nil(t, book.CoverImage)
}

// TestConvertToBook_MultipleCoverImages tests that only first image is used.
func TestConvertToBook_MultipleCoverImages(t *testing.T) {
	now := time.Now()
	item := &LibraryItemData{
		Path: "/audiobooks/test",
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/test/file.mp3",
				Filename: "file.mp3",
				Ext:      ".mp3",
				Size:     1000,
				Inode:    1001,
			},
		},
		ImageFiles: []ImageFileData{
			{
				Path:     "/audiobooks/test/cover.jpg",
				Filename: "cover.jpg",
				Ext:      ".jpg",
				Size:     50000,
				ModTime:  now,
				Inode:    2001,
			},
			{
				Path:     "/audiobooks/test/back.jpg",
				Filename: "back.jpg",
				Ext:      ".jpg",
				Size:     40000,
				ModTime:  now,
				Inode:    2002,
			},
		},
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)
	require.NotNil(t, book.CoverImage)
	assert.Equal(t, "cover.jpg", book.CoverImage.Filename) // Only first image used
}

// TestConvertToBook_FileExtensions tests various file extension handling.
func TestConvertToBook_FileExtensions(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".mp3", "mp3"},
		{".MP3", "mp3"},
		{".m4b", "m4b"},
		{".M4B", "m4b"},
		{".jpg", "jpg"},
		{".JPG", "jpg"},
		{".png", "png"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			item := &LibraryItemData{
				Path: "/audiobooks/test",
				AudioFiles: []AudioFileData{
					{
						Path:     "/audiobooks/test/file" + tt.ext,
						Filename: "file" + tt.ext,
						Ext:      tt.ext,
						Size:     1000,
						Inode:    1001,
					},
				},
			}

			book, err := ConvertToBook(item)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, book.AudioFiles[0].Format)
		})
	}
}

// TestSortAudioFilesByFilename tests the sorting function.
func TestSortAudioFilesByFilename(t *testing.T) {
	files := []domain.AudioFileInfo{
		{Filename: "track10.mp3"},
		{Filename: "track2.mp3"},
		{Filename: "track1.mp3"},
		{Filename: "track03.mp3"},
	}

	sortAudioFilesByFilename(files)

	// Simple string comparison (not natural sort yet).
	assert.Equal(t, "track03.mp3", files[0].Filename)
	assert.Equal(t, "track1.mp3", files[1].Filename)
	assert.Equal(t, "track10.mp3", files[2].Filename)
	assert.Equal(t, "track2.mp3", files[3].Filename)
}

// TestSortAudioFilesByFilename_Empty tests sorting empty slice.
func TestSortAudioFilesByFilename_Empty(t *testing.T) {
	files := []domain.AudioFileInfo{}
	sortAudioFilesByFilename(files)
	assert.Empty(t, files)
}

// TestSortAudioFilesByFilename_Single tests sorting single file.
func TestSortAudioFilesByFilename_Single(t *testing.T) {
	files := []domain.AudioFileInfo{
		{Filename: "track1.mp3"},
	}
	sortAudioFilesByFilename(files)
	assert.Len(t, files, 1)
	assert.Equal(t, "track1.mp3", files[0].Filename)
}

// TestCompareFilenames tests filename comparison.
func TestCompareFilenames(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		expected int
	}{
		{"a.mp3", "b.mp3", -1},
		{"b.mp3", "a.mp3", 1},
		{"a.mp3", "a.mp3", 0},
		{"track1.mp3", "track2.mp3", -1},
		{"track10.mp3", "track2.mp3", -1}, // String comparison, not natural
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			result := compareFilenames(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFindAudioFileForChapter tests chapter-to-file matching.
func TestFindAudioFileForChapter(t *testing.T) {
	files := []domain.AudioFileInfo{
		{ID: "af-1", Duration: 600000},  // 10 minutes (0-10)
		{ID: "af-2", Duration: 900000},  // 15 minutes (10-25)
		{ID: "af-3", Duration: 1200000}, // 20 minutes (25-45)
	}

	tests := []struct {
		name       string
		expectedID string
		startMs    int64
	}{
		{name: "start of first file", startMs: 0, expectedID: "af-1"},
		{name: "middle of first file", startMs: 300000, expectedID: "af-1"},  // 5 min
		{name: "start of second file", startMs: 600000, expectedID: "af-2"},  // 10 min
		{name: "middle of second file", startMs: 900000, expectedID: "af-2"}, // 15 min
		{name: "start of third file", startMs: 1500000, expectedID: "af-3"},  // 25 min
		{name: "end of last file", startMs: 2700000, expectedID: "af-3"},     // 45 min (beyond end -> last file)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findAudioFileForChapter(tt.startMs, files)
			assert.Equal(t, tt.expectedID, result)
		})
	}
}

// TestFindAudioFileForChapter_EmptyFiles tests with no audio files.
func TestFindAudioFileForChapter_EmptyFiles(t *testing.T) {
	result := findAudioFileForChapter(1000, []domain.AudioFileInfo{})
	assert.Empty(t, result)
}

// TestConvertChapters_Empty tests converting empty chapters.
func TestConvertChapters_Empty(t *testing.T) {
	audioFiles := []domain.AudioFileInfo{
		{ID: "af-1", Duration: 600000},
	}

	result := convertChapters([]Chapter{}, audioFiles)
	assert.Nil(t, result)
}

// TestConvertChapters_SingleFile tests chapter conversion for single file.
func TestConvertChapters_SingleFile(t *testing.T) {
	audioFiles := []domain.AudioFileInfo{
		{ID: "af-1", Duration: 3600000}, // 60 minutes
	}

	chapters := []Chapter{
		{ID: 1, Title: "Intro", StartTime: 0, EndTime: 5 * time.Minute},
		{ID: 2, Title: "Main", StartTime: 5 * time.Minute, EndTime: 50 * time.Minute},
		{ID: 3, Title: "Outro", StartTime: 50 * time.Minute, EndTime: 60 * time.Minute},
	}

	result := convertChapters(chapters, audioFiles)
	require.Len(t, result, 3)

	// All should reference the same file.
	for i, ch := range result {
		assert.Equal(t, i, ch.Index)
		assert.Equal(t, "af-1", ch.AudioFileID)
	}

	assert.Equal(t, "Intro", result[0].Title)
	assert.Equal(t, int64(0), result[0].StartTime)
	assert.Equal(t, int64(300000), result[0].EndTime)
}

// TestConvertChapters_MultiFile tests chapter conversion for multiple files.
func TestConvertChapters_MultiFile(t *testing.T) {
	audioFiles := []domain.AudioFileInfo{
		{ID: "af-1", Duration: 1200000}, // 20 minutes
		{ID: "af-2", Duration: 1800000}, // 30 minutes
	}

	chapters := []Chapter{
		{ID: 1, Title: "Ch1", StartTime: 0, EndTime: 10 * time.Minute},
		{ID: 2, Title: "Ch2", StartTime: 10 * time.Minute, EndTime: 20 * time.Minute},
		{ID: 3, Title: "Ch3", StartTime: 20 * time.Minute, EndTime: 35 * time.Minute},
		{ID: 4, Title: "Ch4", StartTime: 35 * time.Minute, EndTime: 50 * time.Minute},
	}

	result := convertChapters(chapters, audioFiles)
	require.Len(t, result, 4)

	// First two chapters in first file.
	assert.Equal(t, "af-1", result[0].AudioFileID)
	assert.Equal(t, "af-1", result[1].AudioFileID)

	// Last two chapters in second file.
	assert.Equal(t, "af-2", result[2].AudioFileID)
	assert.Equal(t, "af-2", result[3].AudioFileID)
}

// TestUpdateBookFromScan tests updating existing book with new scan data.
func TestUpdateBookFromScan(t *testing.T) {
	originalCreatedAt := time.Now().Add(-24 * time.Hour)
	existingBook := &domain.Book{
		Syncable: domain.Syncable{
			ID:        "book-existing-id",
			CreatedAt: originalCreatedAt,
			UpdatedAt: originalCreatedAt,
		},
		Title:      "Old Title",
		Path:       "/old/path",
		Authors:    []string{"Old Author"},
		ScannedAt:  originalCreatedAt,
		AudioFiles: []domain.AudioFileInfo{},
		CoverImage: nil,
	}

	newItem := &LibraryItemData{
		Path: "/new/path",
		AudioFiles: []AudioFileData{
			{
				Path:     "/new/path/file.mp3",
				Filename: "file.mp3",
				Ext:      ".mp3",
				Size:     2048000,
				Inode:    2001,
				Metadata: &AudioMetadata{
					Duration: 10 * time.Minute,
				},
			},
		},
		Metadata: &BookMetadata{
			Title:   "New Title",
			Authors: []string{"New Author"},
		},
	}

	err := UpdateBookFromScan(existingBook, newItem)
	require.NoError(t, err)

	// ID and CreatedAt should be preserved.
	assert.Equal(t, "book-existing-id", existingBook.ID)
	assert.Equal(t, originalCreatedAt, existingBook.CreatedAt)

	// Everything else should be updated.
	assert.Equal(t, "New Title", existingBook.Title)
	assert.Equal(t, "/new/path", existingBook.Path)
	assert.Equal(t, []string{"New Author"}, existingBook.Authors)
	assert.Len(t, existingBook.AudioFiles, 1)

	// UpdatedAt and ScannedAt should be refreshed.
	assert.True(t, existingBook.UpdatedAt.After(originalCreatedAt))
	assert.True(t, existingBook.ScannedAt.After(originalCreatedAt))
}

// TestUpdateBookFromScan_PreservesID tests that book ID is never changed.
func TestUpdateBookFromScan_PreservesID(t *testing.T) {
	existingBook := &domain.Book{
		Syncable: domain.Syncable{
			ID:        "book-preserve-this-id",
			CreatedAt: time.Now().Add(-1 * time.Hour),
		},
		Title: "Old",
	}

	newItem := &LibraryItemData{
		Path: "/test",
		AudioFiles: []AudioFileData{
			{
				Path:     "/test/file.mp3",
				Filename: "file.mp3",
				Ext:      ".mp3",
				Size:     1000,
				Inode:    1001,
			},
		},
		Metadata: &BookMetadata{Title: "New"},
	}

	err := UpdateBookFromScan(existingBook, newItem)
	require.NoError(t, err)

	// ID must never change.
	assert.Equal(t, "book-preserve-this-id", existingBook.ID)
}

// TestConvertToBook_AudioFileWithoutMetadata tests audio file without metadata.
func TestConvertToBook_AudioFileWithoutMetadata(t *testing.T) {
	item := &LibraryItemData{
		Path: "/audiobooks/test",
		AudioFiles: []AudioFileData{
			{
				Path:     "/audiobooks/test/file.mp3",
				Filename: "file.mp3",
				Ext:      ".mp3",
				Size:     1024000,
				Inode:    1001,
				Metadata: nil, // No metadata
			},
		},
	}

	book, err := ConvertToBook(item)
	require.NoError(t, err)
	require.Len(t, book.AudioFiles, 1)

	// Duration should be 0 if no metadata.
	assert.Equal(t, int64(0), book.AudioFiles[0].Duration)
	assert.Equal(t, 0, book.AudioFiles[0].Bitrate)
	assert.Empty(t, book.AudioFiles[0].Codec)

	// TotalDuration should be 0.
	assert.Equal(t, int64(0), book.TotalDuration)

	// TotalSize should still be set.
	assert.Equal(t, int64(1024000), book.TotalSize)
}
