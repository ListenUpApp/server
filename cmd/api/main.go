// Package main provides the entry point for the ListenUp server application.
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
	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/processor"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/listenupapp/listenup-server/internal/watcher"
)

//nolint:gocyclo,gocritic // gocyclo: Main has high complexity; gocritic: os.Exit is intentional, critical cleanup done explicitly
func main() {
	// Load configuration.
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger.
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

	// Load or generate authentication key.
	authKey, err := auth.LoadOrGenerateKey(cfg.Metadata.BasePath)
	if err != nil {
		log.Error("Failed to load authentication key", "error", err)
		os.Exit(1)
	}
	cfg.Auth.AccessTokenKey = authKey
	log.Info("Authentication key loaded",
		"access_token_duration", cfg.Auth.AccessTokenDuration,
		"refresh_token_duration", cfg.Auth.RefreshTokenDuration,
	)

	// Initialize SSE manager first (required by store).
	sseManager := sse.NewManager(log.Logger)
	sseCtx, sseCancel := context.WithCancel(context.Background())
	go sseManager.Start(sseCtx)

	// Initialize database with SSE manager for event broadcasting.
	dbPath := filepath.Join(cfg.Metadata.BasePath, "db")
	db, err := store.New(dbPath, log.Logger, sseManager)
	if err != nil {
		log.Error("Failed to initialize database", "error", err)
		sseCancel() // Cancel SSE context before exit
		os.Exit(1)
	}
	// Defer close as safety net (also explicitly closed in shutdown sequence).
	defer func() {
		if err := db.Close(); err != nil {
			// Only log if not already closed.
			log.Error("Failed to close database (defer)", "error", err)
		}
	}()

	// Bootstrap library and collections (ensures they exist).
	// Note: Use placeholder owner ID "system" for initial bootstrap.
	// This will be updated to the actual root user ID when setup completes.
	ctx := context.Background()
	bootstrap, err := db.EnsureLibrary(ctx, cfg.Library.AudiobookPath, "system")
	if err != nil {
		log.Error("Failed to bootstrap library", "error", err)
		sseCancel()
		os.Exit(1)
	}

	log.Info("Library ready",
		"library_id", bootstrap.Library.ID,
		"library_name", bootstrap.Library.Name,
		"owner_id", bootstrap.Library.OwnerID,
		"scan_paths", len(bootstrap.Library.ScanPaths),
		"is_new", bootstrap.IsNewLibrary,
		"inbox_collection", bootstrap.InboxCollection.ID,
	)

	// Initialize image storage and processor for cover art extraction.
	imageStorage, err := images.NewStorage(cfg.Metadata.BasePath)
	if err != nil {
		log.Error("Failed to initialize image storage", "error", err)
		sseCancel()
		os.Exit(1)
	}
	imageProcessor := images.NewProcessor(imageStorage, log.Logger)

	// Initialize scanner with SSE event emitter and image processor.
	fileScanner := scanner.NewScanner(db, sseManager, imageProcessor, log.Logger)

	// If new library, trigger initial full scan.
	if bootstrap.IsNewLibrary {
		log.Info("New library detected, starting initial scan")
		go func() {
			for _, scanPath := range bootstrap.Library.ScanPaths {
				log.Info("Running initial scan", "path", scanPath)
				if _, err := fileScanner.Scan(ctx, scanPath, scanner.ScanOptions{
					LibraryID: bootstrap.Library.ID,
				}); err != nil {
					log.Error("Initial scan failed", "path", scanPath, "error", err)
				}
			}
		}()
	}

	// Initialize file watcher.
	fileWatcher, err := watcher.New(log.Logger, watcher.Options{
		IgnoreHidden: true,
	})
	if err != nil {
		log.Error("Failed to create file watcher", "error", err)
		sseCancel()
		os.Exit(1)
	}
	defer func() {
		if err := fileWatcher.Stop(); err != nil {
			log.Error("Failed to stop file watcher (defer)", "error", err)
		}
	}()

	// Watch all library scan paths.
	for _, scanPath := range bootstrap.Library.ScanPaths {
		if err := fileWatcher.Watch(scanPath); err != nil {
			log.Error("Failed to watch scan path", "path", scanPath, "error", err)
			sseCancel()
			os.Exit(1)
		}
		log.Info("Watching scan path", "path", scanPath)
	}

	// Start watcher in background.
	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	defer watcherCancel()

	go func() {
		if err := fileWatcher.Start(watcherCtx); err != nil {
			log.Error("File watcher error", "error", err)
		}
	}()

	// Initialize event processor.
	eventProcessor := processor.NewEventProcessor(fileScanner, db, log.Logger)

	// Process file watcher events in background.
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

	// Start periodic session cleanup job.
	// Cleans up expired sessions every hour to prevent database bloat.
	sessionCleanupCtx, sessionCleanupCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// Run initial cleanup on startup.
		if count, err := db.DeleteExpiredSessions(sessionCleanupCtx); err != nil {
			log.Warn("Initial session cleanup failed", "error", err)
		} else if count > 0 {
			log.Info("Initial session cleanup completed", "deleted", count)
		}

		for {
			select {
			case <-ticker.C:
				if count, err := db.DeleteExpiredSessions(sessionCleanupCtx); err != nil {
					log.Warn("Session cleanup failed", "error", err)
				} else if count > 0 {
					log.Info("Session cleanup completed", "deleted", count)
				}
			case <-sessionCleanupCtx.Done():
				return
			}
		}
	}()

	// Initialize services.
	instanceService := service.NewInstanceService(db, log.Logger, cfg)
	bookService := service.NewBookService(db, fileScanner, log.Logger)
	collectionService := service.NewCollectionService(db, log.Logger)
	sharingService := service.NewSharingService(db, log.Logger)
	syncService := service.NewSyncService(db, log.Logger)
	listeningService := service.NewListeningService(db, sseManager, log.Logger)
	genreService := service.NewGenreService(db, log.Logger)
	tagService := service.NewTagService(db, log.Logger)

	// Initialize auth services.
	// Convert key bytes to hex string for token service
	keyHex := fmt.Sprintf("%x", cfg.Auth.AccessTokenKey)
	tokenService, err := auth.NewTokenService(keyHex, cfg.Auth.AccessTokenDuration, cfg.Auth.RefreshTokenDuration)
	if err != nil {
		log.Error("Failed to create token service", "error", err)
		sseCancel()
		os.Exit(1)
	}
	sessionService := service.NewSessionService(db, tokenService, log.Logger)
	authService := service.NewAuthService(db, tokenService, sessionService, instanceService, log.Logger)

	sseHandler := sse.NewHandler(sseManager, log.Logger)

	// Seed default genres (no-op if already seeded).
	if err := genreService.SeedDefaultGenres(ctx); err != nil {
		log.Error("Failed to seed default genres", "error", err)
		// Non-fatal - continue without genres.
	} else {
		log.Info("Default genres seeded successfully")
	}

	// Check if server instance configuration exists, create if not (first run).
	instanceConfig, err := instanceService.InitializeInstance(ctx)
	if err != nil {
		log.Error("Failed to initialize server instance configuration", "error", err)
		sseCancel()
		os.Exit(1)
	}

	// Log server instance state.
	if !instanceConfig.IsSetupRequired() {
		log.Info("Server instance is configured and ready",
			"instance_id", instanceConfig.ID,
			"root_user_id", instanceConfig.RootUserID,
			"created_at", instanceConfig.CreatedAt,
		)
	} else {
		log.Warn("Server instance needs setup - no root user configured",
			"instance_id", instanceConfig.ID,
			"setup_required", true,
		)
	}

	// Create HTTP server with service layer.
	// TODO: Future note to self: This is going to get old fast depending on how many
	// services we need to instantiate. Let's look into a better solution.
	httpServer := api.NewServer(db, instanceService, authService, bookService, collectionService, sharingService, syncService, listeningService, genreService, tagService, sseHandler, imageStorage, log.Logger)

	// Configure HTTP server.
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      httpServer,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine.
	go func() {
		log.Info("HTTP server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", "error", err)
			sseCancel()
			os.Exit(1)
		}
	}()

	log.Info("Server running", "addr", srv.Addr)

	// Wait for interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server gracefully...")

	// Shutdown sequence (order matters!):
	// 1. Stop session cleanup job.
	// 2. Stop file watcher (no more file events).
	// 3. Shutdown SSE manager (no more event broadcasts).
	// 4. Shutdown HTTP server (no more requests).
	// 5. Close database (no more data access).

	// Stop session cleanup job.
	sessionCleanupCancel()

	// Stop file watcher.
	watcherCancel()
	if err := fileWatcher.Stop(); err != nil {
		log.Error("Failed to stop file watcher", "error", err)
	}

	// Shutdown SSE manager gracefully.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sseCancel() // Cancel SSE context to stop broadcast loop
	if err := sseManager.Shutdown(shutdownCtx); err != nil {
		log.Error("Failed to shutdown SSE manager", "error", err)
	}

	// Shutdown HTTP server.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
	}

	// Explicitly close database before exit.
	log.Info("Closing database...")
	if err := db.Close(); err != nil {
		log.Error("Failed to close database", "error", err)
	} else {
		log.Info("Database closed successfully")
	}

	log.Info("See you space cowboy...")
}
