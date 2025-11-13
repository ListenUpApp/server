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
	// Defer close as safety net (also explicitly closed in shutdown sequence)
	defer func() {
		if err := db.Close(); err != nil {
			// Only log if not already closed
			log.Error("Failed to close database (defer)", "error", err)
		}
	}()

	// Bootstrap library and collections (ensures they exist)
	ctx := context.Background()
	bootstrap, err := db.EnsureLibrary(ctx, cfg.Library.AudiobookPath)
	if err != nil {
		log.Error("Failed to bootstrap library", "error", err)
		os.Exit(1)
	}

	log.Info("Library ready",
		"library_id", bootstrap.Library.ID,
		"library_name", bootstrap.Library.Name,
		"scan_paths", len(bootstrap.Library.ScanPaths),
		"is_new", bootstrap.IsNewLibrary,
		"default_collection", bootstrap.DefaultCollection.ID,
		"inbox_collection", bootstrap.InboxCollection.ID,
	)

	// Initialize scanner
	fileScanner := scanner.NewScanner(db, log.Logger)

	// If new library, trigger initial full scan
	if bootstrap.IsNewLibrary {
		log.Info("New library detected, starting initial scan")
		go func() {
			for _, scanPath := range bootstrap.Library.ScanPaths {
				log.Info("Running initial scan", "path", scanPath)
				if _, err := fileScanner.Scan(ctx, scanPath, scanner.ScanOptions{}); err != nil {
					log.Error("Initial scan failed", "path", scanPath, "error", err)
				}
			}
		}()
	}

	// Initialize file watcher
	fileWatcher, err := watcher.New(log.Logger, watcher.Options{
		IgnoreHidden: true,
	})
	if err != nil {
		log.Error("Failed to create file watcher", "error", err)
		os.Exit(1)
	}
	defer fileWatcher.Stop()

	// Watch all library scan paths
	for _, scanPath := range bootstrap.Library.ScanPaths {
		if err := fileWatcher.Watch(scanPath); err != nil {
			log.Error("Failed to watch scan path", "path", scanPath, "error", err)
			os.Exit(1)
		}
		log.Info("Watching scan path", "path", scanPath)
	}

	// Start watcher in background
	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	defer watcherCancel()

	go func() {
		if err := fileWatcher.Start(watcherCtx); err != nil {
			log.Error("File watcher error", "error", err)
		}
	}()

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

	log.Info("File watcher started", "scan_paths", len(bootstrap.Library.ScanPaths))

	// Initialize services
	instanceService := service.NewInstanceService(db, log.Logger)
	bookService := service.NewBookService(db, fileScanner, log.Logger)
	syncService := service.NewSyncService(db, log.Logger)

	// Check if server instance configuration exists, create if not (first run)
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
	// TODO: Future note to self: This is going to get old fast depending on how many
	// services we need to instantiate. Let's look into a better solution
	httpServer := api.NewServer(instanceService, bookService, syncService, log.Logger)

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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
	}

	// Explicitly close database before exit
	log.Info("Closing database...")
	if err := db.Close(); err != nil {
		log.Error("Failed to close database", "error", err)
	} else {
		log.Info("Database closed successfully")
	}

	log.Info("Server stopped")
}
