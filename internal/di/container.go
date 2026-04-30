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
	do.Provide(injector, providers.ProvideReadingSessionService)
	do.Provide(injector, providers.ProvideActivityService)
	do.Provide(injector, providers.ProvideListeningService)
	do.Provide(injector, providers.ProvideStatsService)
	do.Provide(injector, providers.ProvideSocialService)
	do.Provide(injector, providers.ProvideProfileService)
	do.Provide(injector, providers.ProvideGenreService)
	do.Provide(injector, providers.ProvideTagService)
	do.Provide(injector, providers.ProvideInviteService)
	do.Provide(injector, providers.ProvideShelfService)
	do.Provide(injector, providers.ProvideInboxService)
	do.Provide(injector, providers.ProvideSettingsService)
	do.Provide(injector, providers.ProvideAdminService)
	do.Provide(injector, providers.ProvideLibraryService)

	// Workers
	do.Provide(injector, providers.ProvideTranscodeService)
	do.Provide(injector, providers.ProvideFileWatcher)
	do.Provide(injector, providers.ProvideSessionCleanupJob)
	do.Provide(injector, providers.ProvideEventLogCleanupJob)

	// Server
	do.Provide(injector, providers.ProvideHTTPServer)
	do.Provide(injector, providers.ProvideMDNSService)

	return injector
}

// Bootstrap eagerly resolves every registered service so the DI graph fails
// fast at startup instead of surfacing a misconfiguration on the first request.
//
// We deliberately do not rely on do/v2 lazy resolution here. Many providers in
// this graph are not pure constructors:
//   - ProvideStore opens the SQLite database and wires SSE access checkers.
//   - ProvideBootstrap runs EnsureLibrary (filesystem + DB writes).
//   - ProvideHTTPServer launches http.Server.ListenAndServe in a goroutine.
//   - ProvideMDNSService initializes the server instance and (optionally)
//     starts mDNS advertisement.
//   - ProvideTranscodeService, ProvideFileWatcher, ProvideSessionCleanupJob,
//     ProvideEventLogCleanupJob start background workers.
//   - ProvideGenreService seeds default genres into the database.
//
// If we left these to be resolved lazily on first use, `cmd/server` would
// return from main() without ever opening the DB, starting the HTTP listener,
// or kicking off background workers. The enumeration below is the explicit
// "bring the graph up" step. Adding a new provider with side effects MUST
// also add an entry here.
//
// After services are up, this also triggers one-shot startup tasks
// (search reindex, user-stats backfill, initial scan for a fresh library).
func Bootstrap(injector *do.RootScope) error {
	// Eagerly resolve every registered service. The list is in dependency
	// order for readability, but do/v2 handles ordering on its own — what
	// matters is that every entry is resolved before we return.
	eagerInvokes := []func(*do.RootScope){
		// Core infrastructure
		func(i *do.RootScope) { _ = do.MustInvoke[*config.Config](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*logger.Logger](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[providers.AuthKey](i) },

		// Database & SSE
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.SSEManagerHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.StoreHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.Bootstrap](i) },

		// Storage / processing
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.ImageStorages](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*images.Processor](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*scanner.Scanner](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*processor.EventProcessor](i) },

		// Search
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.SearchIndexHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.SearchService](i) },

		// Metadata
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.AudibleClientHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.MetadataServiceHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.ITunesClientHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.CoverService](i) },

		// Auth
		func(i *do.RootScope) { _ = do.MustInvoke[*auth.TokenService](i) },

		// Business services
		func(i *do.RootScope) { _ = do.MustInvoke[*service.InstanceService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.SessionService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.AuthService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.BookService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.ChapterService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.CollectionService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.SharingService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.SyncService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.ReadingSessionService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.ListeningService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.StatsService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.ProfileService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.GenreService](i) }, // seeds default genres
		func(i *do.RootScope) { _ = do.MustInvoke[*service.TagService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.InviteService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.ShelfService](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*service.AdminService](i) },

		// Background workers (each starts goroutines on construction)
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.TranscodeServiceHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.FileWatcherHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.SessionCleanupJob](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.EventLogCleanupJob](i) },

		// Server (HTTP listener + mDNS announce both spawn at construction)
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.HTTPServerHandle](i) },
		func(i *do.RootScope) { _ = do.MustInvoke[*providers.MDNSServiceHandle](i) },
	}
	for _, invoke := range eagerInvokes {
		invoke(injector)
	}

	// One-shot startup tasks that depend on the graph being live.
	providers.TriggerSearchReindexIfNeeded(injector)
	providers.BackfillUserStatsIfNeeded(injector)

	// Run initial scan if this is a fresh library.
	bootstrap := do.MustInvoke[*providers.Bootstrap](injector)
	if bootstrap.IsNewLibrary {
		go providers.RunInitialScan(injector, bootstrap)
	}

	return nil
}
