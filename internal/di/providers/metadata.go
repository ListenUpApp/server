package providers

import (
	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
	"github.com/listenupapp/listenup-server/internal/service"
)

// AudibleClientHandle wraps the Audible client with shutdown capability.
type AudibleClientHandle struct {
	*audible.Client
}

// Shutdown implements do.Shutdownable.
func (h *AudibleClientHandle) Shutdown() error {
	h.Client.Close()
	return nil
}

// ProvideAudibleClient provides the Audible API client.
func ProvideAudibleClient(i do.Injector) (*AudibleClientHandle, error) {
	log := do.MustInvoke[*logger.Logger](i)

	client := audible.New(log.Logger)
	log.Info("Audible client initialized")

	return &AudibleClientHandle{Client: client}, nil
}

// MetadataServiceHandle wraps the metadata service with shutdown capability.
type MetadataServiceHandle struct {
	*service.MetadataService
}

// Shutdown implements do.Shutdownable.
func (h *MetadataServiceHandle) Shutdown() error {
	h.MetadataService.Close()
	return nil
}

// ProvideMetadataService provides the metadata service.
func ProvideMetadataService(i do.Injector) (*MetadataServiceHandle, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)
	clientHandle := do.MustInvoke[*AudibleClientHandle](i)
	storeHandle := do.MustInvoke[*StoreHandle](i)

	defaultRegion := audible.Region(cfg.Audible.DefaultRegion)
	if !defaultRegion.Valid() {
		defaultRegion = audible.RegionUS
		log.Warn("Invalid Audible default region, falling back to US",
			"configured", cfg.Audible.DefaultRegion,
		)
	}

	svc := service.NewMetadataService(
		clientHandle.Client,
		storeHandle.Store,
		defaultRegion,
		log.Logger,
	)

	log.Info("Metadata service initialized",
		"default_region", defaultRegion,
	)

	return &MetadataServiceHandle{MetadataService: svc}, nil
}
