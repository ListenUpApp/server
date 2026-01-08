package processor

import (
	"testing"
)

func TestClassifyFile(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected FileType
	}{
		// Audio files - all supported extensions.
		{
			name:     "MP3 audio file",
			path:     "/library/book.mp3",
			expected: FileTypeAudio,
		},
		{
			name:     "M4B audiobook file",
			path:     "/library/book.m4b",
			expected: FileTypeAudio,
		},
		{
			name:     "M4A audio file",
			path:     "/library/chapter.m4a",
			expected: FileTypeAudio,
		},
		{
			name:     "FLAC audio file",
			path:     "/library/track.flac",
			expected: FileTypeAudio,
		},
		{
			name:     "OGG audio file",
			path:     "/library/audio.ogg",
			expected: FileTypeAudio,
		},
		{
			name:     "OPUS audio file",
			path:     "/library/voice.opus",
			expected: FileTypeAudio,
		},
		{
			name:     "AAC audio file",
			path:     "/library/chapter.aac",
			expected: FileTypeAudio,
		},
		{
			name:     "WMA audio file",
			path:     "/library/book.wma",
			expected: FileTypeAudio,
		},
		{
			name:     "WAV audio file",
			path:     "/library/sample.wav",
			expected: FileTypeAudio,
		},
		{
			name:     "Audio file with uppercase extension",
			path:     "/library/BOOK.MP3",
			expected: FileTypeAudio,
		},
		{
			name:     "Audio file with mixed case extension",
			path:     "/library/chapter.M4b",
			expected: FileTypeAudio,
		},
		{
			name:     "Audio file in nested directory",
			path:     "/library/Author/Book/CD1/01.mp3",
			expected: FileTypeAudio,
		},

		// Cover art files.
		{
			name:     "JPG cover file",
			path:     "/library/cover.jpg",
			expected: FileTypeCover,
		},
		{
			name:     "JPEG cover file",
			path:     "/library/cover.jpeg",
			expected: FileTypeCover,
		},
		{
			name:     "PNG cover file",
			path:     "/library/artwork.png",
			expected: FileTypeCover,
		},
		{
			name:     "WEBP cover file",
			path:     "/library/image.webp",
			expected: FileTypeCover,
		},
		{
			name:     "Cover file with uppercase extension",
			path:     "/library/COVER.JPG",
			expected: FileTypeCover,
		},
		{
			name:     "Cover file with mixed case extension",
			path:     "/library/art.PnG",
			expected: FileTypeCover,
		},

		// Metadata files.
		{
			name:     "NFO metadata file",
			path:     "/library/info.nfo",
			expected: FileTypeMetadata,
		},
		{
			name:     "TXT metadata file",
			path:     "/library/description.txt",
			expected: FileTypeMetadata,
		},
		{
			name:     "JSON metadata file",
			path:     "/library/metadata.json",
			expected: FileTypeMetadata,
		},
		{
			name:     "Metadata file with uppercase extension",
			path:     "/library/INFO.NFO",
			expected: FileTypeMetadata,
		},

		// Ignored files.
		{
			name:     "CUE file (ignored)",
			path:     "/library/tracks.cue",
			expected: FileTypeIgnored,
		},
		{
			name:     "LOG file (ignored)",
			path:     "/library/rip.log",
			expected: FileTypeIgnored,
		},
		{
			name:     "Hidden file (ignored)",
			path:     "/library/.DS_Store",
			expected: FileTypeIgnored,
		},
		{
			name:     "Temporary file (ignored)",
			path:     "/library/file.tmp",
			expected: FileTypeIgnored,
		},
		{
			name:     "Unknown extension (ignored)",
			path:     "/library/document.pdf",
			expected: FileTypeIgnored,
		},
		{
			name:     "No extension (ignored)",
			path:     "/library/README",
			expected: FileTypeIgnored,
		},
		{
			name:     "Executable file (ignored)",
			path:     "/library/script.sh",
			expected: FileTypeIgnored,
		},
		{
			name:     "Video file (ignored)",
			path:     "/library/video.mp4",
			expected: FileTypeIgnored,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyFile(tt.path)
			if result != tt.expected {
				t.Errorf("classifyFile(%q) = %v, expected %v",
					tt.path, result, tt.expected)
			}
		})
	}
}

func TestFileType_String(t *testing.T) {
	tests := []struct {
		expected string
		fileType FileType
	}{
		{fileType: FileTypeAudio, expected: "audio"},
		{fileType: FileTypeCover, expected: "cover"},
		{fileType: FileTypeMetadata, expected: "metadata"},
		{fileType: FileTypeIgnored, expected: "ignored"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.fileType.String()
			if result != tt.expected {
				t.Errorf("FileType.String() = %q, expected %q",
					result, tt.expected)
			}
		})
	}
}

func TestClassifyFile_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected FileType
	}{
		{
			name:     "Empty string",
			path:     "",
			expected: FileTypeIgnored,
		},
		{
			name:     "Only extension",
			path:     ".mp3",
			expected: FileTypeAudio,
		},
		{
			name:     "Multiple dots in filename",
			path:     "/library/file.backup.mp3",
			expected: FileTypeAudio,
		},
		{
			name:     "Windows path separator",
			path:     "C:\\library\\book.m4b",
			expected: FileTypeAudio,
		},
		{
			name:     "Path with spaces",
			path:     "/library/My Book/Chapter 01.mp3",
			expected: FileTypeAudio,
		},
		{
			name:     "Path with special characters",
			path:     "/library/Author - Book (2023)/01.mp3",
			expected: FileTypeAudio,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyFile(tt.path)
			if result != tt.expected {
				t.Errorf("classifyFile(%q) = %v, expected %v",
					tt.path, result, tt.expected)
			}
		})
	}
}

// Benchmark to ensure classification is fast (critical for event processing).
func BenchmarkClassifyFile(b *testing.B) {
	paths := []string{
		"/library/book.mp3",
		"/library/cover.jpg",
		"/library/metadata.nfo",
		"/library/track.cue",
	}

	for i := 0; b.Loop(); i++ {
		classifyFile(paths[i%len(paths)])
	}
}
