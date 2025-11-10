package processor

import (
	"testing"
)

func TestDetermineBookFolder(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected string
	}{
		// Regular folders - file in directory becomes book
		{
			name:     "Single file in root",
			filePath: "/library/book.m4b",
			expected: "/library",
		},
		{
			name:     "File in author/book structure",
			filePath: "/library/Author/Book/01.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "File in simple book folder",
			filePath: "/library/MyBook/chapter.mp3",
			expected: "/library/MyBook",
		},
		{
			name:     "File in deeply nested folder",
			filePath: "/media/audiobooks/Fiction/Author/Book/file.m4b",
			expected: "/media/audiobooks/Fiction/Author/Book",
		},

		// Disc folders - CD patterns (lowercase)
		{
			name:     "CD1 folder",
			filePath: "/library/Author/Book/cd1/01.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "CD2 folder",
			filePath: "/library/Author/Book/cd2/05.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "CD with zero padding",
			filePath: "/library/Author/Book/cd01/track.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "CD with space",
			filePath: "/library/Author/Book/cd 1/file.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "CD with space and zero padding",
			filePath: "/library/Author/Book/cd 01/audio.mp3",
			expected: "/library/Author/Book",
		},

		// Disc folders - CD patterns (uppercase)
		{
			name:     "CD1 folder uppercase",
			filePath: "/library/Author/Book/CD1/01.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "CD2 folder uppercase",
			filePath: "/library/Author/Book/CD2/track.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "CD with space uppercase",
			filePath: "/library/Author/Book/CD 1/file.mp3",
			expected: "/library/Author/Book",
		},

		// Disc folders - Disc patterns
		{
			name:     "Disc 1 folder",
			filePath: "/library/Author/Book/Disc 1/01.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "Disc 2 folder",
			filePath: "/library/Author/Book/Disc 2/chapter.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "Disc1 no space",
			filePath: "/library/Author/Book/Disc1/audio.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "Disc 01 with zero padding",
			filePath: "/library/Author/Book/Disc 01/track.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "Disc lowercase",
			filePath: "/library/Author/Book/disc 1/file.mp3",
			expected: "/library/Author/Book",
		},

		// Disc folders - Disk patterns (alternate spelling)
		{
			name:     "Disk 1 folder",
			filePath: "/library/Author/Book/Disk 1/01.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "Disk 2 folder",
			filePath: "/library/Author/Book/Disk 2/audio.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "Disk1 no space",
			filePath: "/library/Author/Book/Disk1/chapter.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "disk lowercase",
			filePath: "/library/Author/Book/disk 1/track.mp3",
			expected: "/library/Author/Book",
		},

		// Mixed case disc folders
		{
			name:     "Mixed case CD",
			filePath: "/library/Author/Book/Cd1/file.mp3",
			expected: "/library/Author/Book",
		},
		{
			name:     "Mixed case Disc",
			filePath: "/library/Author/Book/DiSc 1/audio.mp3",
			expected: "/library/Author/Book",
		},

		// Edge cases - NOT disc folders (should use as-is)
		{
			name:     "Folder starting with CD but no number",
			filePath: "/library/Author/CDBook/01.mp3",
			expected: "/library/Author/CDBook",
		},
		{
			name:     "Folder containing CD in middle",
			filePath: "/library/Author/The CD Collection/01.mp3",
			expected: "/library/Author/The CD Collection",
		},
		{
			name:     "Disc word but no number",
			filePath: "/library/Author/Disc/audio.mp3",
			expected: "/library/Author/Disc",
		},
		{
			name:     "Folder with disc in name but not pattern",
			filePath: "/library/Discworld/01.mp3",
			expected: "/library/Discworld",
		},
		{
			name:     "Folder named 'Discs' (plural)",
			filePath: "/library/Author/Book/Discs/01.mp3",
			expected: "/library/Author/Book/Discs",
		},

		// Real-world patterns
		{
			name:     "Typical multi-disc structure CD1",
			filePath: "/audiobooks/Stephen King/The Stand/CD1/Track01.mp3",
			expected: "/audiobooks/Stephen King/The Stand",
		},
		{
			name:     "Typical multi-disc structure CD2",
			filePath: "/audiobooks/Stephen King/The Stand/CD2/Track01.mp3",
			expected: "/audiobooks/Stephen King/The Stand",
		},
		{
			name:     "Single M4B in author folder",
			filePath: "/audiobooks/Brandon Sanderson/Mistborn.m4b",
			expected: "/audiobooks/Brandon Sanderson",
		},

		// Cover art and metadata in disc folders
		{
			name:     "Cover art in disc folder",
			filePath: "/library/Author/Book/CD1/cover.jpg",
			expected: "/library/Author/Book",
		},
		{
			name:     "Metadata in disc folder",
			filePath: "/library/Author/Book/Disc 1/metadata.nfo",
			expected: "/library/Author/Book",
		},

		// Note: Windows paths are not tested as this is a Linux-first system
		// filepath.Dir() behavior is platform-specific and will work correctly
		// on the target platform (Linux)

		// Files with special characters
		{
			name:     "Book with spaces and special chars",
			filePath: "/library/Author Name/Book Title (2023)/CD1/01 - Chapter.mp3",
			expected: "/library/Author Name/Book Title (2023)",
		},
		{
			name:     "Book with unicode characters",
			filePath: "/library/作者/书名/CD1/01.mp3",
			expected: "/library/作者/书名",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineBookFolder(tt.filePath)
			if result != tt.expected {
				t.Errorf("determineBookFolder(%q) = %q, expected %q",
					tt.filePath, result, tt.expected)
			}
		})
	}
}

func TestDetermineBookFolder_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected string
	}{
		{
			name:     "Root file",
			filePath: "/book.mp3",
			expected: "/",
		},
		{
			name:     "Current directory reference",
			filePath: "./book.mp3",
			expected: ".",
		},
		{
			name:     "File in current directory",
			filePath: "book.mp3",
			expected: ".",
		},
		{
			name:     "Empty string",
			filePath: "",
			expected: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineBookFolder(tt.filePath)
			if result != tt.expected {
				t.Errorf("determineBookFolder(%q) = %q, expected %q",
					tt.filePath, result, tt.expected)
			}
		})
	}
}

// TestIsDiscDir tests the disc directory detection logic
// This should match the behavior from internal/scanner/grouper.go
func TestIsDiscDir(t *testing.T) {
	tests := []struct {
		name     string
		dirName  string
		expected bool
	}{
		// Positive cases - should be detected as disc dirs
		{
			name:     "cd1 lowercase",
			dirName:  "cd1",
			expected: true,
		},
		{
			name:     "CD1 uppercase",
			dirName:  "CD1",
			expected: true,
		},
		{
			name:     "cd 1 with space",
			dirName:  "cd 1",
			expected: true,
		},
		{
			name:     "CD 1 uppercase with space",
			dirName:  "CD 1",
			expected: true,
		},
		{
			name:     "cd01 zero padded",
			dirName:  "cd01",
			expected: true,
		},
		{
			name:     "CD 01 uppercase zero padded",
			dirName:  "CD 01",
			expected: true,
		},
		{
			name:     "Disc 1",
			dirName:  "Disc 1",
			expected: true,
		},
		{
			name:     "disc 1 lowercase",
			dirName:  "disc 1",
			expected: true,
		},
		{
			name:     "Disc1 no space",
			dirName:  "Disc1",
			expected: true,
		},
		{
			name:     "DISC 2 uppercase",
			dirName:  "DISC 2",
			expected: true,
		},
		{
			name:     "Disc 01 zero padded",
			dirName:  "Disc 01",
			expected: true,
		},
		{
			name:     "Disk 1",
			dirName:  "Disk 1",
			expected: true,
		},
		{
			name:     "disk 1 lowercase",
			dirName:  "disk 1",
			expected: true,
		},
		{
			name:     "Disk1 no space",
			dirName:  "Disk1",
			expected: true,
		},
		{
			name:     "disk2 lowercase no space",
			dirName:  "disk2",
			expected: true,
		},
		{
			name:     "CD9 single digit",
			dirName:  "CD9",
			expected: true,
		},
		{
			name:     "Disc 10 double digit",
			dirName:  "Disc 10",
			expected: true,
		},
		{
			name:     "Mixed case Cd1",
			dirName:  "Cd1",
			expected: true,
		},

		// Negative cases - should NOT be detected as disc dirs
		{
			name:     "CD without number",
			dirName:  "CD",
			expected: false,
		},
		{
			name:     "Disc without number",
			dirName:  "Disc",
			expected: false,
		},
		{
			name:     "CDBook (CD prefix but not disc)",
			dirName:  "CDBook",
			expected: false,
		},
		{
			name:     "Discworld (Disc prefix but not disc)",
			dirName:  "Discworld",
			expected: false,
		},
		{
			name:     "The CD Collection",
			dirName:  "The CD Collection",
			expected: false,
		},
		{
			name:     "Discs (plural)",
			dirName:  "Discs",
			expected: false,
		},
		{
			name:     "Regular folder name",
			dirName:  "Book Title",
			expected: false,
		},
		{
			name:     "Empty string",
			dirName:  "",
			expected: false,
		},
		{
			name:     "Number alone",
			dirName:  "1",
			expected: false,
		},
		{
			name:     "CD followed by letter",
			dirName:  "CDA",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDiscDir(tt.dirName)
			if result != tt.expected {
				t.Errorf("isDiscDir(%q) = %v, expected %v",
					tt.dirName, result, tt.expected)
			}
		})
	}
}

// Benchmark folder determination (should be extremely fast)
func BenchmarkDetermineBookFolder(b *testing.B) {
	paths := []string{
		"/library/Author/Book/01.mp3",
		"/library/Author/Book/CD1/01.mp3",
		"/library/Author/Book/Disc 1/chapter.mp3",
		"/audiobooks/Stephen King/The Stand/CD2/Track01.mp3",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		determineBookFolder(paths[i%len(paths)])
	}
}

func BenchmarkIsDiscDir(b *testing.B) {
	dirNames := []string{
		"CD1",
		"Disc 1",
		"Book Title",
		"cd 01",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isDiscDir(dirNames[i%len(dirNames)])
	}
}
