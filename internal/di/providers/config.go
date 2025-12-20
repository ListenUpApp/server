// Package providers contains dependency injection providers for the ListenUp server.
package providers

import (
	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
)

// ProvideConfig provides the application configuration.
func ProvideConfig(i do.Injector) (*config.Config, error) {
	return config.LoadConfig()
}

// ProvideLogger provides the structured logger.
func ProvideLogger(i do.Injector) (*logger.Logger, error) {
	cfg := do.MustInvoke[*config.Config](i)

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

	return log, nil
}
