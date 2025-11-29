package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = os.ExpandEnv("$HOME/listenUp/db")
	}

	opts := badger.DefaultOptions(dbPath).
		WithReadOnly(true).
		WithLogger(nil)

	db, err := badger.Open(opts)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fmt.Println("=== Database Inspection ===")
	fmt.Println()

	// Count books and check chapters
	bookCount := 0
	booksWithChapters := 0
	booksWithoutChapters := 0
	totalChapters := 0

	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("book:")
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte("book:")); it.ValidForPrefix([]byte("book:")); it.Next() {
			item := it.Item()
			key := string(item.Key())

			// Skip index keys
			if strings.Contains(key, ":") && !strings.HasPrefix(key, "book:") {
				continue
			}

			err := item.Value(func(val []byte) error {
				var book domain.Book
				if err := json.Unmarshal(val, &book); err != nil {
					return err
				}

				bookCount++
				chapterCount := len(book.Chapters)
				totalChapters += chapterCount

				if chapterCount > 0 {
					booksWithChapters++
					// Show first few books with chapters
					if booksWithChapters <= 3 {
						fmt.Printf("Book: %s\n", book.Title)
						fmt.Printf("  ID: %s\n", book.ID)
						fmt.Printf("  Chapters: %d\n", chapterCount)
						fmt.Printf("  Audio Files: %d\n", len(book.AudioFiles))
						for i, ch := range book.Chapters {
							if i < 5 { // Show first 5 chapters
								fmt.Printf("    [%d] %s (%.1f - %.1f sec)\n",
									ch.Index, ch.Title,
									float64(ch.StartTime)/1000,
									float64(ch.EndTime)/1000)
							}
						}
						if chapterCount > 5 {
							fmt.Printf("    ... and %d more chapters\n", chapterCount-5)
						}
						fmt.Println()
					}
				} else {
					booksWithoutChapters++
					// Show first few books without chapters
					if booksWithoutChapters <= 3 {
						fmt.Printf("Book (NO CHAPTERS): %s\n", book.Title)
						fmt.Printf("  ID: %s\n", book.ID)
						fmt.Printf("  Audio Files: %d\n", len(book.AudioFiles))
						fmt.Printf("  Path: %s\n", book.Path)
						fmt.Println()
					}
				}

				return nil
			})
			if err != nil {
				log.Printf("Error reading book %s: %v", key, err)
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error iterating database: %v", err)
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Total books: %d\n", bookCount)
	fmt.Printf("Books with chapters: %d\n", booksWithChapters)
	fmt.Printf("Books without chapters: %d\n", booksWithoutChapters)
	fmt.Printf("Total chapters: %d\n", totalChapters)
	if bookCount > 0 {
		fmt.Printf("Average chapters per book: %.1f\n", float64(totalChapters)/float64(bookCount))
	}
}
