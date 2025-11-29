package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/simonhull/audiometa"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: chaptest <audio_file>")
	}

	path := os.Args[1]
	fmt.Printf("Testing: %s\n\n", path)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	file, err := audiometa.OpenContext(ctx, path)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	fmt.Printf("Format: %s\n", file.Format.String())
	fmt.Printf("Duration: %s\n", file.Audio.Duration)
	fmt.Printf("Album: %s\n", file.Tags.Album)
	fmt.Printf("Title: %s\n", file.Tags.Title)
	fmt.Printf("Artist: %s\n", file.Tags.Artist)
	fmt.Println()

	fmt.Printf("Chapters: %d\n", len(file.Chapters))
	for i, ch := range file.Chapters {
		if i < 10 { // Show first 10 chapters
			fmt.Printf("  [%d] %s (%.1f - %.1f sec)\n",
				ch.Index, ch.Title,
				ch.StartTime.Seconds(),
				ch.EndTime.Seconds())
		}
	}
	if len(file.Chapters) > 10 {
		fmt.Printf("  ... and %d more chapters\n", len(file.Chapters)-10)
	}
}
