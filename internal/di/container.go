// Package di provides dependency injection configuration for the ListenUp server.
package di

import (
	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/di/providers"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/processor"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
)

// NewContainer creates and configures the DI container with all providers.
func NewContainer() *do.RootScope {
	injector := do.New()

	// Core infrastructure
	do.Provide(injector, providers.ProvideConfig)
	do.Provide(injector, providers.ProvideLogger)
	do.Provide(injector, providers.ProvideAuthKey)

	// Database layer
	do.Provide(injector, providers.ProvideSSEManager)
	do.Provide(injector, providers.ProvideRegistrationBroadcaster)
	do.Provide(injector, providers.ProvideStore)
	do.Provide(injector, providers.ProvideBootstrap)

	// Storage layer
	do.Provide(injector, providers.ProvideImageStorages)
	do.Provide(injector, providers.ProvideImageProcessor)

	// Scanner layer
	do.Provide(injector, providers.ProvideScanner)
	do.Provide(injector, providers.ProvideEventProcessor)

	// Search layer
	do.Provide(injector, providers.ProvideSearchIndex)
	do.Provide(injector, providers.ProvideSearchService)

	// Metadata layer
	do.Provide(injector, providers.ProvideAudibleClient)
	do.Provide(injector, providers.ProvideMetadataService)
	do.Provide(injector, providers.ProvideITunesClient)
	do.Provide(injector, providers.ProvideCoverService)

	// Auth layer
	do.Provide(injector, providers.ProvideTokenService)

	// Business services
	do.Provide(injector, providers.ProvideInstanceService)
	do.Provide(injector, providers.ProvideSessionService)
	do.Provide(injector, providers.ProvideAuthService)
	do.Provide(injector, providers.ProvideBookService)
	do.Provide(injector, providers.ProvideChapterService)
	do.Provide(injector, providers.ProvideCollectionService)
	do.Provide(injector, providers.ProvideSharingService)
	do.Provide(injector, providers.ProvideSyncService)
	do.Provide(injector, providers.ProvideListeningService)
	do.Provide(injector, providers.ProvideGenreService)
	do.Provide(injector, providers.ProvideTagService)
	do.Provide(injector, providers.ProvideInviteService)
	do.Provide(injector, providers.ProvideLensService)
	do.Provide(injector, providers.ProvideInboxService)
	do.Provide(injector, providers.ProvideSettingsService)
	do.Provide(injector, providers.ProvideAdminService)

	// Workers
	do.Provide(injector, providers.ProvideTranscodeService)
	do.Provide(injector, providers.ProvideFileWatcher)
	do.Provide(injector, providers.ProvideSessionCleanupJob)

	// Server
	do.Provide(injector, providers.ProvideHTTPServer)
	do.Provide(injector, providers.ProvideMDNSService)

	return injector
}

// Bootstrap initializes all services and returns handles for lifecycle management.
// This triggers lazy initialization of all core services.
func Bootstrap(injector *do.RootScope) error {
	// Invoke core services to trigger initialization
	_ = do.MustInvoke[*config.Config](injector)
	_ = do.MustInvoke[*logger.Logger](injector)
	_ = do.MustInvoke[providers.AuthKey](injector)
	_ = do.MustInvoke[*providers.SSEManagerHandle](injector)
	_ = do.MustInvoke[*providers.StoreHandle](injector)
	_ = do.MustInvoke[*providers.Bootstrap](injector)
	_ = do.MustInvoke[*providers.ImageStorages](injector)
	_ = do.MustInvoke[*images.Processor](injector)
	_ = do.MustInvoke[*scanner.Scanner](injector)
	_ = do.MustInvoke[*processor.EventProcessor](injector)
	_ = do.MustInvoke[*providers.SearchIndexHandle](injector)
	_ = do.MustInvoke[*service.SearchService](injector)
	_ = do.MustInvoke[*providers.AudibleClientHandle](injector)
	_ = do.MustInvoke[*providers.MetadataServiceHandle](injector)
	_ = do.MustInvoke[*providers.ITunesClientHandle](injector)
	_ = do.MustInvoke[*service.CoverService](injector)
	_ = do.MustInvoke[*auth.TokenService](injector)

	// Business services
	_ = do.MustInvoke[*service.InstanceService](injector)
	_ = do.MustInvoke[*service.SessionService](injector)
	_ = do.MustInvoke[*service.AuthService](injector)
	_ = do.MustInvoke[*service.BookService](injector)
	_ = do.MustInvoke[*service.ChapterService](injector)
	_ = do.MustInvoke[*service.CollectionService](injector)
	_ = do.MustInvoke[*service.SharingService](injector)
	_ = do.MustInvoke[*service.SyncService](injector)
	_ = do.MustInvoke[*service.ListeningService](injector)
	_ = do.MustInvoke[*service.GenreService](injector)
	_ = do.MustInvoke[*service.TagService](injector)
	_ = do.MustInvoke[*service.InviteService](injector)
	_ = do.MustInvoke[*service.LensService](injector)
	_ = do.MustInvoke[*service.AdminService](injector)

	// Workers
	_ = do.MustInvoke[*providers.TranscodeServiceHandle](injector)
	_ = do.MustInvoke[*providers.FileWatcherHandle](injector)
	_ = do.MustInvoke[*providers.SessionCleanupJob](injector)

	// Server
	_ = do.MustInvoke[*providers.HTTPServerHandle](injector)
	_ = do.MustInvoke[*providers.MDNSServiceHandle](injector)

	// Trigger search reindex if needed
	providers.TriggerSearchReindexIfNeeded(injector)

	// Run initial scan if new library
	bootstrap := do.MustInvoke[*providers.Bootstrap](injector)
	if bootstrap.IsNewLibrary {
		go providers.RunInitialScan(injector, bootstrap)
	}

	return nil
}
