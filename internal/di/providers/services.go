package providers

import (
	"context"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
)

// ProvideInstanceService provides the server instance service.
func ProvideInstanceService(i do.Injector) (*service.InstanceService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)
	cfg := do.MustInvoke[*config.Config](i)

	return service.NewInstanceService(storeHandle.Store, log.Logger, cfg), nil
}

// ProvideSessionService provides the session management service.
func ProvideSessionService(i do.Injector) (*service.SessionService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	tokenService := do.MustInvoke[*auth.TokenService](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewSessionService(storeHandle.Store, tokenService, log.Logger), nil
}

// ProvideAuthService provides the authentication service.
func ProvideAuthService(i do.Injector) (*service.AuthService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	tokenService := do.MustInvoke[*auth.TokenService](i)
	sessionService := do.MustInvoke[*service.SessionService](i)
	instanceService := do.MustInvoke[*service.InstanceService](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewAuthService(storeHandle.Store, tokenService, sessionService, instanceService, log.Logger), nil
}

// ProvideBookService provides the book service.
func ProvideBookService(i do.Injector) (*service.BookService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	fileScanner := do.MustInvoke[*scanner.Scanner](i)
	metadataHandle := do.MustInvoke[*MetadataServiceHandle](i)
	coverService := do.MustInvoke[*service.CoverService](i)
	storages := do.MustInvoke[*ImageStorages](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewBookService(
		storeHandle.Store,
		fileScanner,
		metadataHandle.MetadataService,
		coverService,
		storages.Covers,
		log.Logger,
	), nil
}

// ProvideChapterService provides the chapter service.
func ProvideChapterService(i do.Injector) (*service.ChapterService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	metadataHandle := do.MustInvoke[*MetadataServiceHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewChapterService(
		storeHandle.Store,
		metadataHandle.MetadataService,
		log.Logger,
	), nil
}

// ProvideCollectionService provides the collection service.
func ProvideCollectionService(i do.Injector) (*service.CollectionService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewCollectionService(storeHandle.Store, log.Logger), nil
}

// ProvideSharingService provides the sharing service.
func ProvideSharingService(i do.Injector) (*service.SharingService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewSharingService(storeHandle.Store, log.Logger), nil
}

// ProvideSyncService provides the sync service.
func ProvideSyncService(i do.Injector) (*service.SyncService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewSyncService(storeHandle.Store, log.Logger), nil
}

// ProvideReadingSessionService provides the reading session management service.
func ProvideReadingSessionService(i do.Injector) (*service.ReadingSessionService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewReadingSessionService(storeHandle.Store, sseHandle.Manager, log.Logger), nil
}

// ProvideActivityService provides the activity feed service.
func ProvideActivityService(i do.Injector) (*service.ActivityService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewActivityService(storeHandle.Store, sseHandle.Manager, log.Logger), nil
}

// ProvideListeningService provides the listening progress service.
func ProvideListeningService(i do.Injector) (*service.ListeningService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	readingSessionService := do.MustInvoke[*service.ReadingSessionService](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewListeningService(storeHandle.Store, sseHandle.Manager, readingSessionService, log.Logger), nil
}

// ProvideStatsService provides the listening statistics service.
func ProvideStatsService(i do.Injector) (*service.StatsService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewStatsService(storeHandle.Store, log.Logger), nil
}

// ProvideSocialService provides the social features service.
func ProvideSocialService(i do.Injector) (*service.SocialService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewSocialService(storeHandle.Store, log.Logger), nil
}

// ProvideProfileService provides the user profile service.
func ProvideProfileService(i do.Injector) (*service.ProfileService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	storages := do.MustInvoke[*ImageStorages](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	statsService := do.MustInvoke[*service.StatsService](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewProfileService(
		storeHandle.Store,
		storages.Avatars,
		sseHandle.Manager,
		statsService,
		log.Logger,
	), nil
}

// ProvideGenreService provides the genre service.
func ProvideGenreService(i do.Injector) (*service.GenreService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	svc := service.NewGenreService(storeHandle.Store, log.Logger)

	// Seed default genres
	ctx := context.Background()
	if err := svc.SeedDefaultGenres(ctx); err != nil {
		log.Error("Failed to seed default genres", "error", err)
		// Non-fatal - continue without genres
	} else {
		log.Info("Default genres seeded successfully")
	}

	return svc, nil
}

// ProvideTagService provides the tag service.
func ProvideTagService(i do.Injector) (*service.TagService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	searchService := do.MustInvoke[*service.SearchService](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewTagService(storeHandle.Store, sseHandle.Manager, searchService, log.Logger), nil
}

// ProvideInviteService provides the invite service.
func ProvideInviteService(i do.Injector) (*service.InviteService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sessionService := do.MustInvoke[*service.SessionService](i)
	log := do.MustInvoke[*logger.Logger](i)
	cfg := do.MustInvoke[*config.Config](i)

	// TODO: Get from config or auto-detect
	serverURL := "http://localhost:" + cfg.Server.Port

	return service.NewInviteService(storeHandle.Store, sessionService, log.Logger, serverURL), nil
}

// ProvideAdminService provides the admin service.
func ProvideAdminService(i do.Injector) (*service.AdminService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)
	registrationBroadcaster := do.MustInvoke[*sse.RegistrationBroadcaster](i)
	shelfService := do.MustInvoke[*service.ShelfService](i)

	return service.NewAdminService(storeHandle.Store, log.Logger, registrationBroadcaster, shelfService), nil
}

// ProvideShelfService provides the shelf service.
func ProvideShelfService(i do.Injector) (*service.ShelfService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewShelfService(storeHandle.Store, sseHandle.Manager, log.Logger), nil
}

// ProvideInboxService provides the inbox staging workflow service.
func ProvideInboxService(i do.Injector) (*service.InboxService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	enricher := dto.NewEnricher(storeHandle.Store)
	return service.NewInboxService(storeHandle.Store, enricher, sseHandle.Manager, log.Logger), nil
}

// ProvideSettingsService provides the server settings service.
func ProvideSettingsService(i do.Injector) (*service.SettingsService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	inboxService := do.MustInvoke[*service.InboxService](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewSettingsService(storeHandle.Store, inboxService, log.Logger), nil
}

// ProvideLibraryService provides the library management service.
func ProvideLibraryService(i do.Injector) (*service.LibraryService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewLibraryService(storeHandle.Store, sseHandle.Manager, log.Logger), nil
}
