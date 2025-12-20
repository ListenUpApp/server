// Package main provides the entry point for the ListenUp server application.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/di"
	"github.com/listenupapp/listenup-server/internal/di/providers"
	"github.com/listenupapp/listenup-server/internal/logger"
)

func main() {
	// Create DI container
	injector := di.NewContainer()

	// Bootstrap all services
	if err := di.Bootstrap(injector); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bootstrap server: %v\n", err)
		os.Exit(1)
	}

	// Get logger for shutdown messages
	log := do.MustInvoke[*logger.Logger](injector)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server gracefully...")

	// Shutdown all services in reverse order
	// The DI container handles shutdown order automatically
	if err := injector.Shutdown(); err != nil {
		log.Error("Shutdown error", "error", err)
	}

	// Manually shutdown remaining services that need explicit cleanup
	// (services that implement do.Shutdownable are handled automatically)

	// Database and search index need explicit shutdown since they use wrapper types
	if storeHandle, err := do.Invoke[*providers.StoreHandle](injector); err == nil {
		log.Info("Closing database...")
		if err := storeHandle.Shutdown(); err != nil {
			log.Error("Failed to close database", "error", err)
		} else {
			log.Info("Database closed successfully")
		}
	}

	if searchHandle, err := do.Invoke[*providers.SearchIndexHandle](injector); err == nil {
		log.Info("Closing search index...")
		if err := searchHandle.Shutdown(); err != nil {
			log.Error("Failed to close search index", "error", err)
		} else {
			log.Info("Search index closed successfully")
		}
	}

	log.Info("See you space cowboy...")
}
