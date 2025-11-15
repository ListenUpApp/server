package domain

import (
	"context"
	"time"
)

// CascadeUpdater defines the interface for updating entity timestamps.
// Right now this basically just addes complexity, but once we add more.
// domain types, making sure they all update (and sync) is going to be.
// paramount to success, this file lays the groundwork and evolves over time.
type CascadeUpdater interface {
	// TouchEntity updates the UpdatedAt timestamp for an entity by ID.
	TouchEntity(ctx context.Context, entityType string, id string) error
	// GetBookIDsBySeries retrieves book IDs in a series without loading full book data.
	GetBookIDsBySeries(ctx context.Context, seriesID string) ([]string, error)
}

// CascadeBookUpdate updates timestamps for a book and all releated entities.
// When a book changes, we need to update:
// - The book itself.
func CascadeBookUpdate(ctx context.Context, updater CascadeUpdater, bookID string) error {
	// For now we just touch the book itself but when we add more entities.
	// These will get included here too.

	if err := updater.TouchEntity(ctx, "book", bookID); err != nil {
		return err
	}

	// TODO: add more entities here when the time comes.

	return nil
}

// CascadeSeriesUpdate updates timestamps for all books in a series.
// When a series changes, all its books need to be touched for sync.
func CascadeSeriesUpdate(ctx context.Context, updater CascadeUpdater, seriesID string) error {
	// Touch the series itself
	if err := updater.TouchEntity(ctx, "series", seriesID); err != nil {
		return err
	}

	// Get book IDs in this series (optimized - doesn't load full book data)
	bookIDs, err := updater.GetBookIDsBySeries(ctx, seriesID)
	if err != nil {
		return err
	}

	// Touch each book to trigger sync for clients
	for _, bookID := range bookIDs {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := updater.TouchEntity(ctx, "book", bookID); err != nil {
			return err
		}
	}

	return nil
}

// GetCurrentCheckpoint returns the most recent UpdatedAt timestamp.
// across all entities, this is used for sync delta queries and operations.
func GetCurrentCheckpoint(books []*Book) time.Time {
	var latest time.Time

	for _, book := range books {
		if book.UpdatedAt.After(latest) {
			latest = book.UpdatedAt
		}
	}

	// TODO: when other entities are added add them here. (thats most of this file right now guys)

	return latest
}
