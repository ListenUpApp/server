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
