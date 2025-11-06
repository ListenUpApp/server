package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/listenupapp/listenup-server/internal/scanner"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: scan-test <library-path>")
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	s := scanner.NewScanner(nil, logger) // nil store for now

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := s.Scan(ctx, os.Args[1], scanner.ScanOptions{
		Workers: 4,
		DryRun:  true, // Don't touch DB yet
		OnProgress: func(p *scanner.Progress) {
			fmt.Printf("[%s] %d/%d - %s\n",
				p.Phase, p.Current, p.Total, p.CurrentItem)
		},
	})

	if err != nil {
		logger.Error("scan failed", "error", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Scan Complete ===\n")
	fmt.Printf("Duration: %s\n", result.CompletedAt.Sub(result.StartedAt))
	fmt.Printf("Added: %d\n", result.Added)
	fmt.Printf("Updated: %d\n", result.Updated)
	fmt.Printf("Errors: %d\n", result.Errors)
}
