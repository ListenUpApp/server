// cmd/audiometa-test/main.go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/listenupapp/listenup-server/pkg/audiometa/m4a"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: audiometa-test <file.m4b>")
		os.Exit(1)
	}

	meta, err := m4a.Parse(os.Args[1])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Metadata ===\n")
	fmt.Printf("Format:   %s\n", meta.Format)
	fmt.Printf("Title:    %s\n", meta.Title)
	fmt.Printf("Artist:   %s\n", meta.Artist)
	fmt.Printf("Album:    %s\n", meta.Album)
	fmt.Printf("Genre:    %s\n", meta.Genre)
	fmt.Printf("Year:     %d\n", meta.Year)
	fmt.Printf("Composer: %s\n", meta.Composer)
	fmt.Printf("Comment:  %s\n", meta.Comment)
	fmt.Printf("FileSize: %d bytes (%.2f MB)\n", meta.FileSize, float64(meta.FileSize)/1024/1024)

	// === NEW: Technical Info ===
	fmt.Printf("\n=== Technical Info ===\n")
	if meta.Duration > 0 {
		fmt.Printf("Duration:    %s\n", formatDuration(meta.Duration))
	}
	if meta.BitRate > 0 {
		fmt.Printf("Bitrate:     %d kbps\n", meta.BitRate/1000)
	}
	if meta.SampleRate > 0 {
		fmt.Printf("Sample Rate: %d Hz\n", meta.SampleRate)
	}
	if meta.Channels > 0 {
		fmt.Printf("Channels:    %d\n", meta.Channels)
	}
	if meta.Codec != "" {
		fmt.Printf("Codec:       %s\n", meta.Codec)
	}

	// === NEW: Audiobook Info ===
	fmt.Printf("\n=== Audiobook Info ===\n")
	if meta.Narrator != "" {
		fmt.Printf("Narrator:  %s\n", meta.Narrator)
	} else {
		fmt.Printf("Narrator:  (none)\n")
	}
	if meta.Series != "" {
		fmt.Printf("Series:    %s", meta.Series)
		if meta.SeriesPart != "" {
			fmt.Printf(" #%s", meta.SeriesPart)
		}
		fmt.Printf("\n")
	} else {
		fmt.Printf("Series:    (none)\n")
	}
	if meta.Publisher != "" {
		fmt.Printf("Publisher: %s\n", meta.Publisher)
	}
	if meta.ISBN != "" {
		fmt.Printf("ISBN:      %s\n", meta.ISBN)
	}
	if meta.ASIN != "" {
		fmt.Printf("ASIN:      %s\n", meta.ASIN)
	}

	// === NEW: Chapters ===
	fmt.Printf("\n=== Chapters ===\n")
	if len(meta.Chapters) > 0 {
		fmt.Printf("Total: %d chapters\n\n", len(meta.Chapters))

		// Show first 10 chapters
		showCount := len(meta.Chapters)
		if showCount > 10 {
			showCount = 10
		}

		for i := 0; i < showCount; i++ {
			ch := meta.Chapters[i]
			duration := ch.EndTime - ch.StartTime
			fmt.Printf("  [%2d] %s - %s (%s): %s\n",
				i+1,
				formatDuration(ch.StartTime),
				formatDuration(ch.EndTime),
				formatDuration(duration),
				ch.Title,
			)
		}

		if len(meta.Chapters) > 10 {
			fmt.Printf("  ... and %d more chapters\n", len(meta.Chapters)-10)
		}
	} else {
		fmt.Printf("No chapters found\n")
	}
}

// formatDuration formats a duration as HH:MM:SS
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
