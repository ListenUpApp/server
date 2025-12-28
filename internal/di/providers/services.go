package providers

import (
	"context"
	"fmt"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/scanner"
	"github.com/listenupapp/listenup-server/internal/service"
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

// ProvideListeningService provides the listening progress service.
func ProvideListeningService(i do.Injector) (*service.ListeningService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewListeningService(storeHandle.Store, sseHandle.Manager, log.Logger), nil
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
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewTagService(storeHandle.Store, log.Logger), nil
}

// ProvideInviteService provides the invite service.
func ProvideInviteService(i do.Injector) (*service.InviteService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sessionService := do.MustInvoke[*service.SessionService](i)
	log := do.MustInvoke[*logger.Logger](i)
	cfg := do.MustInvoke[*config.Config](i)

	// TODO: Get from config or auto-detect
	serverURL := fmt.Sprintf("http://localhost:%s", cfg.Server.Port)

	return service.NewInviteService(storeHandle.Store, sessionService, log.Logger, serverURL), nil
}

// ProvideAdminService provides the admin service.
func ProvideAdminService(i do.Injector) (*service.AdminService, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewAdminService(storeHandle.Store, log.Logger), nil
}
