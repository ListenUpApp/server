package main

import (
	"fmt"
	"os"

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
}
