package providers

import (
	"context"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/service"
)

// BackfillUserStatsIfNeeded runs the user stats backfill at startup.
func BackfillUserStatsIfNeeded(i do.Injector) {
	log := do.MustInvoke[*logger.Logger](i)
	storeHandle := do.MustInvoke[*StoreHandle](i)

	ctx := context.Background()
	if err := service.BackfillUserStats(ctx, storeHandle.Store, log.Logger); err != nil {
		log.Error("Failed to backfill user stats", "error", err)
	}
}
