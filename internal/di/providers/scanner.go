package providers

import (
	"context"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/processor"
	"github.com/listenupapp/listenup-server/internal/scanner"
)

// ProvideScanner provides the file scanner.
func ProvideScanner(i do.Injector) (*scanner.Scanner, error) {
	storeHandle := do.MustInvoke[*StoreHandle](i)
	sseHandle := do.MustInvoke[*SSEManagerHandle](i)
	imageProcessor := do.MustInvoke[*images.Processor](i)
	log := do.MustInvoke[*logger.Logger](i)

	return scanner.NewScanner(storeHandle.Store, sseHandle.Manager, imageProcessor, log.Logger), nil
}

// ProvideEventProcessor provides the file event processor.
func ProvideEventProcessor(i do.Injector) (*processor.EventProcessor, error) {
	fileScanner := do.MustInvoke[*scanner.Scanner](i)
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return processor.NewEventProcessor(fileScanner, storeHandle.Store, log.Logger), nil
}

// RunInitialScan starts an initial library scan in a goroutine.
// Should be called after all dependencies are wired.
func RunInitialScan(i do.Injector, bootstrap *Bootstrap) {
	if !bootstrap.IsNewLibrary {
		return
	}

	fileScanner := do.MustInvoke[*scanner.Scanner](i)
	log := do.MustInvoke[*logger.Logger](i)

	log.Info("New library detected, starting initial scan")

	ctx := context.Background()
	for _, scanPath := range bootstrap.Library.ScanPaths {
		log.Info("Running initial scan", "path", scanPath)
		if _, err := fileScanner.Scan(ctx, scanPath, scanner.ScanOptions{
			LibraryID: bootstrap.Library.ID,
		}); err != nil {
			log.Error("Initial scan failed", "path", scanPath, "error", err)
		}
	}
}
