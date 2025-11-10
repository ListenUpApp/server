package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/listenupapp/listenup-server/internal/api"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/processor"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/listenupapp/listenup-server/internal/watcher"
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

	log.Info("Starting ListenUp Server",
		"environment", cfg.App.Environment,
		"log_level", cfg.Logger.Level,
		"metadata_path", cfg.Metadata.BasePath,
		"audiobook_path", cfg.Library.AudiobookPath,
	)

	// Initialize database
	dbPath := filepath.Join(cfg.Metadata.BasePath, "db")
	db, err := store.New(dbPath, log.Logger)
	if err != nil {
		log.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("Failed to close database", "error", err)
		}
	}()

	// Initialize file watcher
	fileWatcher, err := watcher.New(log.Logger, watcher.Options{
		IgnoreHidden: true,
	})
	if err != nil {
		log.Error("Failed to create file watcher", "error", err)
		os.Exit(1)
	}
	defer fileWatcher.Stop()

	// Start watching audiobook library
	if err := fileWatcher.Watch(cfg.Library.AudiobookPath); err != nil {
		log.Error("Failed to watch audiobook path", "error", err)
		os.Exit(1)
	}

	// Start watcher in background
	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	defer watcherCancel()

	go func() {
		if err := fileWatcher.Start(watcherCtx); err != nil {
			log.Error("File watcher error", "error", err)
		}
	}()

	// Initialize scanner
	fileScanner := scanner.NewScanner(db, log.Logger)

	// Initialize event processor
	eventProcessor := processor.NewEventProcessor(fileScanner, log.Logger)

	// Process file watcher events in background
	go func() {
		for {
			select {
			case event := <-fileWatcher.Events():
				if err := eventProcessor.ProcessEvent(watcherCtx, event); err != nil {
					log.Warn("failed to process event",
						"error", err,
						"type", event.Type,
						"path", event.Path,
					)
				}
			case err := <-fileWatcher.Errors():
				log.Warn("file watcher error", "error", err)
			case <-watcherCtx.Done():
				return
			}
		}
	}()

	log.Info("File watcher started", "path", cfg.Library.AudiobookPath)

	// Initialize services
	instanceService := service.NewInstanceService(db, log.Logger)

	// Check if server instance configuration exists, create if not (first run)
	ctx := context.Background()
	instanceConfig, err := instanceService.InitializeInstance(ctx)
	if err != nil {
		log.Error("Failed to initialize server instance configuration", "error", err)
		os.Exit(1)
	}

	// Log server instance state
	if instanceConfig.HasRootUser {
		log.Info("Server instance is configured and ready",
			"instance_id", instanceConfig.ID,
			"created_at", instanceConfig.CreatedAt,
		)
	} else {
		log.Warn("Server instance needs setup - no root user configured",
			"instance_id", instanceConfig.ID,
			"setup_required", true,
		)
	}

	// Create HTTP server with service layer
	httpServer := api.NewServer(instanceService, log.Logger)

	// Configure HTTP server
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      httpServer,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Info("HTTP server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	log.Info("Server running", "addr", srv.Addr)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server gracefully...")

	// Stop file watcher
	watcherCancel()
	if err := fileWatcher.Stop(); err != nil {
		log.Error("Failed to stop file watcher", "error", err)
	}

	// Graceful shutdown with 30s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	log.Info("Server stopped")
}
