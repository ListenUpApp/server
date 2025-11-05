package main

import (
	"fmt"
	"os"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(logger.Config{
		Level:       logger.ParseLevel(cfg.Logger.Level),
		AddSource:   cfg.App.Environment == "development",
		Environment: cfg.App.Environment,
	})

	// Log startup information
	log.Info("Starting ListenUp Server",
		"environment", cfg.App.Environment,
		"log_level", cfg.Logger.Level,
		"metadata_path", cfg.Metadata.BasePath,
	)

	log.Info("Server running")
}
