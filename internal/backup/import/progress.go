package backupimport

import (
	"context"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// rebuildProgress rebuilds all PlaybackProgress from ListeningEvents.
func (i *Importer) rebuildProgress(ctx context.Context) error {
	i.logger.Info("rebuilding playback progress from events")

	// Clear existing progress
	if err := i.store.ClearAllProgress(ctx); err != nil {
		return err
	}

	// Stream all events, build progress by user+book
	progressMap := make(map[string]*domain.PlaybackProgress)
	orphanedCount := 0

	for event, err := range i.store.StreamListeningEvents(ctx) {
		if err != nil {
			i.logger.Warn("error reading event", "error", err)
			continue
		}

		key := domain.ProgressID(event.UserID, event.BookID)

		// Get book duration for progress calculation
		book, err := i.store.GetBookNoAccessCheck(ctx, event.BookID)
		if err != nil {
			// Orphaned event - book doesn't exist
			orphanedCount++
			continue
		}

		progress, exists := progressMap[key]
		if !exists {
			progress = domain.NewPlaybackProgress(event, book.TotalDuration)
			progressMap[key] = progress
		} else {
			progress.UpdateFromEvent(event, book.TotalDuration)
		}
	}

	if orphanedCount > 0 {
		i.logger.Warn("events reference missing books",
			"orphaned_count", orphanedCount)
	}

	// Persist all progress
	saved := 0
	for _, progress := range progressMap {
		if err := i.store.SaveProgress(ctx, progress); err != nil {
			i.logger.Warn("failed to save progress",
				"user_id", progress.UserID,
				"book_id", progress.BookID,
				"error", err)
			continue
		}
		saved++
	}

	i.logger.Info("progress rebuild complete",
		"total", len(progressMap),
		"saved", saved,
		"orphaned_events", orphanedCount)

	return nil
}

// RebuildAllProgress is the public admin function for progress rebuild.
func RebuildAllProgress(ctx context.Context, s *store.Store, logger *slog.Logger) error {
	importer := New(s, "", logger)
	return importer.rebuildProgress(ctx)
}
