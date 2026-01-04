package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const (
	listeningEventPrefix  = "evt:"
	eventByUserPrefix     = "evt:idx:user:"
	eventByBookPrefix     = "evt:idx:book:"
	eventByUserBookPrefix = "evt:idx:userbook:"
	progressPrefix        = "progress:"
)

// Sentinel errors for listening operations.
var (
	ErrEventNotFound    = ErrNotFound.WithMessage("listening event not found")
	ErrProgressNotFound = ErrNotFound.WithMessage("playback progress not found")
)

// CreateListeningEvent stores an event and its indexes atomically.
// Events are immutable - no Update method exists.
func (s *Store) CreateListeningEvent(ctx context.Context, event *domain.ListeningEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Primary key
		if err := txn.Set([]byte(listeningEventPrefix+event.ID), data); err != nil {
			return fmt.Errorf("set event: %w", err)
		}

		// Index: by user
		userIdx := eventByUserPrefix + event.UserID + ":" + event.ID
		if err := txn.Set([]byte(userIdx), []byte(event.ID)); err != nil {
			return fmt.Errorf("set user index: %w", err)
		}

		// Index: by book
		bookIdx := eventByBookPrefix + event.BookID + ":" + event.ID
		if err := txn.Set([]byte(bookIdx), []byte(event.ID)); err != nil {
			return fmt.Errorf("set book index: %w", err)
		}

		// Index: by user+book
		userBookIdx := eventByUserBookPrefix + event.UserID + ":" + event.BookID + ":" + event.ID
		if err := txn.Set([]byte(userBookIdx), []byte(event.ID)); err != nil {
			return fmt.Errorf("set user-book index: %w", err)
		}

		return nil
	})
}

// GetListeningEvent retrieves an event by ID.
func (s *Store) GetListeningEvent(ctx context.Context, id string) (*domain.ListeningEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var event domain.ListeningEvent
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(listeningEventPrefix + id))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrEventNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &event)
		})
	})

	if err != nil {
		return nil, err
	}
	return &event, nil
}

// GetEventsForUser retrieves all events for a user.
func (s *Store) GetEventsForUser(ctx context.Context, userID string) ([]*domain.ListeningEvent, error) {
	return s.getEventsByPrefix(ctx, eventByUserPrefix+userID+":")
}

// GetEventsForBook retrieves all events for a book.
func (s *Store) GetEventsForBook(ctx context.Context, bookID string) ([]*domain.ListeningEvent, error) {
	return s.getEventsByPrefix(ctx, eventByBookPrefix+bookID+":")
}

// GetEventsForUserBook retrieves all events for a user+book combination.
func (s *Store) GetEventsForUserBook(ctx context.Context, userID, bookID string) ([]*domain.ListeningEvent, error) {
	return s.getEventsByPrefix(ctx, eventByUserBookPrefix+userID+":"+bookID+":")
}

// getEventsByPrefix retrieves events matching an index prefix.
// Uses a single transaction to collect IDs and fetch all events (no N+1).
func (s *Store) getEventsByPrefix(ctx context.Context, prefix string) ([]*domain.ListeningEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var events []*domain.ListeningEvent

	err := s.db.View(func(txn *badger.Txn) error {
		// First pass: collect event IDs from index
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		var eventIDs []string
		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			err := it.Item().Value(func(val []byte) error {
				eventIDs = append(eventIDs, string(val))
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Second pass: batch fetch all events in same transaction
		events = make([]*domain.ListeningEvent, 0, len(eventIDs))
		for _, id := range eventIDs {
			item, err := txn.Get([]byte(listeningEventPrefix + id))
			if err != nil {
				continue // Skip missing events
			}

			var event domain.ListeningEvent
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &event)
			}); err != nil {
				continue // Skip corrupt events
			}
			events = append(events, &event)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetProgress retrieves playback progress for a user+book.
func (s *Store) GetProgress(ctx context.Context, userID, bookID string) (*domain.PlaybackProgress, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := progressPrefix + domain.ProgressID(userID, bookID)
	var progress domain.PlaybackProgress

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrProgressNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &progress)
		})
	})

	if err != nil {
		return nil, err
	}
	return &progress, nil
}

// UpsertProgress creates or updates playback progress.
func (s *Store) UpsertProgress(ctx context.Context, progress *domain.PlaybackProgress) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := progressPrefix + domain.ProgressID(progress.UserID, progress.BookID)
	data, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

// DeleteProgress removes playback progress.
func (s *Store) DeleteProgress(ctx context.Context, userID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := progressPrefix + domain.ProgressID(userID, bookID)
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// GetProgressForUser retrieves all progress records for a user.
func (s *Store) GetProgressForUser(ctx context.Context, userID string) ([]*domain.PlaybackProgress, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := progressPrefix + userID + ":"
	var results []*domain.PlaybackProgress

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			var progress domain.PlaybackProgress
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &progress)
			})
			if err != nil {
				return err
			}
			results = append(results, &progress)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// GetEventsForUserInRange retrieves events for a user within a time range.
// Uses the event's EndedAt timestamp for filtering (when listening occurred).
// start is inclusive, end is exclusive. Zero start = beginning of time.
func (s *Store) GetEventsForUserInRange(
	ctx context.Context,
	userID string,
	start, end time.Time,
) ([]*domain.ListeningEvent, error) {
	allEvents, err := s.GetEventsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var filtered []*domain.ListeningEvent
	for _, e := range allEvents {
		// Use EndedAt for "when did this listening session complete"
		if (start.IsZero() || !e.EndedAt.Before(start)) && e.EndedAt.Before(end) {
			filtered = append(filtered, e)
		}
	}

	return filtered, nil
}

// GetProgressFinishedInRange retrieves books finished within a time range.
// start is inclusive, end is exclusive. Zero start = beginning of time.
func (s *Store) GetProgressFinishedInRange(
	ctx context.Context,
	userID string,
	start, end time.Time,
) ([]*domain.PlaybackProgress, error) {
	allProgress, err := s.GetProgressForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var finished []*domain.PlaybackProgress
	for _, p := range allProgress {
		if p.IsFinished && p.FinishedAt != nil {
			finishedAt := *p.FinishedAt
			if (start.IsZero() || !finishedAt.Before(start)) && finishedAt.Before(end) {
				finished = append(finished, p)
			}
		}
	}

	return finished, nil
}

// GetContinueListening returns in-progress books, excluding hidden ones.
// Uses batch lookup: 2 prefix scans instead of N+1 queries.
func (s *Store) GetContinueListening(ctx context.Context, userID string, limit int) ([]*domain.PlaybackProgress, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Scan 1: All progress for user
	allProgress, err := s.GetProgressForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Scan 2: All preferences for user
	allPrefs, err := s.GetAllBookPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Build hidden lookup
	hiddenBooks := make(map[string]bool, len(allPrefs))
	for _, p := range allPrefs {
		if p.HideFromContinueListening {
			hiddenBooks[p.BookID] = true
		}
	}

	// Filter: in-progress, not finished, not hidden
	var result []*domain.PlaybackProgress
	for _, p := range allProgress {
		if p.IsFinished || p.Progress == 0 || hiddenBooks[p.BookID] {
			continue
		}
		result = append(result, p)
	}

	// Sort by LastPlayedAt descending (most recent first)
	slices.SortFunc(result, func(a, b *domain.PlaybackProgress) int {
		return b.LastPlayedAt.Compare(a.LastPlayedAt)
	})

	// Apply limit
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}
