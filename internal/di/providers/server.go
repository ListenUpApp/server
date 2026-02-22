package providers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/api"
	"github.com/listenupapp/listenup-server/internal/backup"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/mdns"
	"github.com/listenupapp/listenup-server/internal/service"
	"github.com/listenupapp/listenup-server/internal/sse"
)

// HTTPServerHandle wraps http.Server with Shutdownable.
type HTTPServerHandle struct {
	*http.Server
}

// sseTokenVerifier adapts AuthService to the sse.TokenVerifier interface.
type sseTokenVerifier struct {
	authService *service.AuthService
}

// VerifyAccessToken implements sse.TokenVerifier.
func (v *sseTokenVerifier) VerifyAccessToken(ctx context.Context, token string) (*domain.User, error) {
	user, _, err := v.authService.VerifyAccessToken(ctx, token)
	return user, err
}

// Shutdown implements do.Shutdownable.
func (h *HTTPServerHandle) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return h.Server.Shutdown(ctx)
}

// ProvideHTTPServer provides the HTTP server.
func ProvideHTTPServer(i do.Injector) (*HTTPServerHandle, error) {
	cfg := do.MustInvoke[*config.Config](i)
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	registrationBroadcaster := do.MustInvoke[*sse.RegistrationBroadcaster](i)
	log := do.MustInvoke[*logger.Logger](i)
	storages := do.MustInvoke[*ImageStorages](i)

	// Get all services
	instanceService := do.MustInvoke[*service.InstanceService](i)
	authService := do.MustInvoke[*service.AuthService](i)
	bookService := do.MustInvoke[*service.BookService](i)
	collectionService := do.MustInvoke[*service.CollectionService](i)
	sharingService := do.MustInvoke[*service.SharingService](i)
	syncService := do.MustInvoke[*service.SyncService](i)
	listeningService := do.MustInvoke[*service.ListeningService](i)
	statsService := do.MustInvoke[*service.StatsService](i)
	socialService := do.MustInvoke[*service.SocialService](i)
	genreService := do.MustInvoke[*service.GenreService](i)
	tagService := do.MustInvoke[*service.TagService](i)
	searchService := do.MustInvoke[*service.SearchService](i)
	inviteService := do.MustInvoke[*service.InviteService](i)
	adminService := do.MustInvoke[*service.AdminService](i)
	transcodeHandle := do.MustInvoke[*TranscodeServiceHandle](i)
	metadataHandle := do.MustInvoke[*MetadataServiceHandle](i)
	chapterService := do.MustInvoke[*service.ChapterService](i)
	coverService := do.MustInvoke[*service.CoverService](i)
	shelfService := do.MustInvoke[*service.ShelfService](i)
	inboxService := do.MustInvoke[*service.InboxService](i)
	settingsService := do.MustInvoke[*service.SettingsService](i)
	readingSessionService := do.MustInvoke[*service.ReadingSessionService](i)
	activityService := do.MustInvoke[*service.ActivityService](i)
	profileService := do.MustInvoke[*service.ProfileService](i)
	libraryService := do.MustInvoke[*service.LibraryService](i)

	// Wire up activity recording to reading session service
	readingSessionService.SetActivityRecorder(activityService)

	// Wire up milestone tracking to listening service
	listeningService.SetMilestoneRecorder(activityService)
	listeningService.SetStreakCalculator(socialService)

	// Wire up activity recording to shelf service
	shelfService.SetActivityRecorder(activityService)

	tokenVerifier := &sseTokenVerifier{authService: authService}
	sseHandler := sse.NewHandler(sseHandle.Manager, log.Logger, tokenVerifier)

	services := &api.Services{
		Instance:       instanceService,
		Auth:           authService,
		Book:           bookService,
		Collection:     collectionService,
		Sharing:        sharingService,
		Sync:           syncService,
		Listening:      listeningService,
		Stats:          statsService,
		Genre:          genreService,
		Tag:            tagService,
		Search:         searchService,
		Invite:         inviteService,
		Admin:          adminService,
		Transcode:      transcodeHandle.TranscodeService,
		Metadata:       metadataHandle.MetadataService,
		Chapter:        chapterService,
		Cover:          coverService,
		Shelf:          shelfService,
		Inbox:          inboxService,
		Settings:       settingsService,
		Social:         socialService,
		ReadingSession: readingSessionService,
		Activity:       activityService,
		Profile:        profileService,
		Library:        libraryService,
	}

	storage := &api.StorageServices{
		Covers:            storages.Covers,
		ContributorImages: storages.ContributorImages,
		SeriesCovers:      storages.SeriesCovers,
		Avatars:           storages.Avatars,
	}

	// Create backup services
	dataDir := cfg.Metadata.BasePath
	backupDir := dataDir + "/backups"
	backupSvc := backup.NewBackupService(storeHandle.Store, backupDir, dataDir, "dev", log.Logger)
	restoreSvc := backup.NewRestoreService(storeHandle.Store, dataDir, log.Logger)

	handler := api.NewServer(storeHandle.Store, services, storage, sseHandler, sseHandle.Manager, registrationBroadcaster, backupSvc, restoreSvc, log.Logger)

	// Wire mDNS refresh callback for when instance settings change
	mdnsHandle := do.MustInvoke[*MDNSServiceHandle](i)
	if mdnsHandle.Service != nil && mdnsHandle.started {
		port := 8080
		fmt.Sscanf(cfg.Server.Port, "%d", &port)
		handler.SetOnInstanceUpdated(func(instance *domain.Instance) {
			if err := mdnsHandle.Service.Refresh(instance, port); err != nil {
				log.Warn("Failed to refresh mDNS after instance update", "error", err)
			}
		})
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start in background
	go func() {
		log.Info("HTTP server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", "error", err)
		}
	}()

	log.Info("Server running", "addr", srv.Addr)

	return &HTTPServerHandle{Server: srv}, nil
}

// MDNSServiceHandle wraps mdns.Service with Shutdownable.
type MDNSServiceHandle struct {
	*mdns.Service
	started bool
}

// Shutdown implements do.Shutdownable.
func (h *MDNSServiceHandle) Shutdown() error {
	if h.started && h.Service != nil {
		h.Stop()
	}
	return nil
}

// ProvideMDNSService provides the mDNS advertisement service.
func ProvideMDNSService(i do.Injector) (*MDNSServiceHandle, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)
	instanceService := do.MustInvoke[*service.InstanceService](i)

	// Always initialize instance regardless of mDNS config.
	ctx := context.Background()
	instanceConfig, err := instanceService.InitializeInstance(ctx)
	if err != nil {
		return nil, err
	}

	// Log server instance state
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

	if !cfg.Server.AdvertiseMDNS {
		log.Info("mDNS advertisement disabled by configuration")
		return &MDNSServiceHandle{Service: nil, started: false}, nil
	}

	svc := mdns.NewService(log.Logger)

	// Parse port
	port := 8080
	if _, err := fmt.Sscanf(cfg.Server.Port, "%d", &port); err != nil {
		log.Warn("Failed to parse server port for mDNS, using default", "port", cfg.Server.Port)
	}

	if err := svc.Start(instanceConfig, port); err != nil {
		log.Warn("mDNS advertisement unavailable", "error", err)
		// Non-fatal: server works without mDNS (e.g., Docker, cloud)
		return &MDNSServiceHandle{Service: svc, started: false}, nil
	}

	return &MDNSServiceHandle{Service: svc, started: true}, nil
}
