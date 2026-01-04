package providers

import (
	"fmt"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/media/images"
)

// ImageStorages groups all image storage services.
type ImageStorages struct {
	Covers            *images.Storage
	ContributorImages *images.Storage
	SeriesCovers      *images.Storage
	Avatars           *images.Storage
}

// ProvideImageStorages provides all image storage services.
func ProvideImageStorages(i do.Injector) (*ImageStorages, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)

	covers, err := images.NewStorage(cfg.Metadata.BasePath)
	if err != nil {
		return nil, fmt.Errorf("cover storage: %w", err)
	}

	contributors, err := images.NewStorageWithSubdir(cfg.Metadata.BasePath, "contributors")
	if err != nil {
		return nil, fmt.Errorf("contributor storage: %w", err)
	}

	series, err := images.NewStorageWithSubdir(cfg.Metadata.BasePath, "covers/series")
	if err != nil {
		return nil, fmt.Errorf("series storage: %w", err)
	}

	avatars, err := images.NewStorageWithSubdir(cfg.Metadata.BasePath, "avatars")
	if err != nil {
		return nil, fmt.Errorf("avatar storage: %w", err)
	}

	log.Info("Image storages initialized")

	return &ImageStorages{
		Covers:            covers,
		ContributorImages: contributors,
		SeriesCovers:      series,
		Avatars:           avatars,
	}, nil
}

// ProvideImageProcessor provides the image processor for cover art.
func ProvideImageProcessor(i do.Injector) (*images.Processor, error) {
	storages := do.MustInvoke[*ImageStorages](i)
	log := do.MustInvoke[*logger.Logger](i)

	return images.NewProcessor(storages.Covers, log.Logger), nil
}
