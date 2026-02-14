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
	eventByUserTimePrefix = "evt:idx:user:time:" // Format: evt:idx:user:time:{userID}:{endedAtMs}:{eventID}
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

		// Index: by user+time (for efficient range queries)
		// Format: evt:idx:user:time:{userID}:{endedAtMs:020d}:{eventID}
		// Zero-padded to 20 digits for lexicographic sorting (supports dates until year 2286)
		userTimeIdx := fmt.Sprintf("%s%s:%020d:%s", eventByUserTimePrefix, event.UserID, event.EndedAt.UnixMilli(), event.ID)
		if err := txn.Set([]byte(userTimeIdx), []byte(event.ID)); err != nil {
			return fmt.Errorf("set user-time index: %w", err)
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

// DeleteEventsForUserBook deletes all listening events for a user and book.
// This is used for GDPR-style purges when keepHistory=false.
func (s *Store) DeleteEventsForUserBook(ctx context.Context, userID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// First, get all events for this user+book to collect their IDs
	events, err := s.GetEventsForUserBook(ctx, userID, bookID)
	if err != nil {
		return fmt.Errorf("get events for deletion: %w", err)
	}

	if len(events) == 0 {
		return nil // Nothing to delete
	}

	// Delete primary records and all indexes in a single transaction
	return s.db.Update(func(txn *badger.Txn) error {
		for _, event := range events {
			// Delete primary key
			if err := txn.Delete([]byte(listeningEventPrefix + event.ID)); err != nil {
				return fmt.Errorf("delete event %s: %w", event.ID, err)
			}

			// Delete index: by user
			userIdx := eventByUserPrefix + event.UserID + ":" + event.ID
			if err := txn.Delete([]byte(userIdx)); err != nil {
				return fmt.Errorf("delete user index: %w", err)
			}

			// Delete index: by user+time
			userTimeIdx := fmt.Sprintf("%s%s:%020d:%s", eventByUserTimePrefix, event.UserID, event.EndedAt.UnixMilli(), event.ID)
			if err := txn.Delete([]byte(userTimeIdx)); err != nil {
				return fmt.Errorf("delete user-time index: %w", err)
			}

			// Delete index: by book
			bookIdx := eventByBookPrefix + event.BookID + ":" + event.ID
			if err := txn.Delete([]byte(bookIdx)); err != nil {
				return fmt.Errorf("delete book index: %w", err)
			}

			// Delete index: by user+book
			userBookIdx := eventByUserBookPrefix + event.UserID + ":" + event.BookID + ":" + event.ID
			if err := txn.Delete([]byte(userBookIdx)); err != nil {
				return fmt.Errorf("delete user-book index: %w", err)
			}
		}

		return nil
	})
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

			// IMPORTANT: Allocate on heap to avoid loop variable pointer bug.
			// Using `var event` inside loop and taking &event would cause all
			// pointers to reference the same memory location (the last value).
			event := new(domain.ListeningEvent)
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, event)
			}); err != nil {
				continue // Skip corrupt events
			}
			events = append(events, event)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetState retrieves playback state for a user+book.
func (s *Store) GetState(ctx context.Context, userID, bookID string) (*domain.PlaybackState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := progressPrefix + domain.StateID(userID, bookID)
	var progress domain.PlaybackState

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

// UpsertState creates or updates playback state.
func (s *Store) UpsertState(ctx context.Context, progress *domain.PlaybackState) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := progressPrefix + domain.StateID(progress.UserID, progress.BookID)
	data, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

// DeleteState removes playback state.
func (s *Store) DeleteState(ctx context.Context, userID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := progressPrefix + domain.StateID(userID, bookID)
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// DeleteEventsForBook deletes all listening events for a book across all users.
// Used when a book is deleted to clean up orphaned data.
func (s *Store) DeleteEventsForBook(ctx context.Context, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get all events for this book
	events, err := s.GetEventsForBook(ctx, bookID)
	if err != nil {
		return fmt.Errorf("get events for book: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	return s.db.Update(func(txn *badger.Txn) error {
		for _, event := range events {
			// Delete event
			if err := txn.Delete([]byte(listeningEventPrefix + event.ID)); err != nil {
				return err
			}
			// Delete indexes
			userKey := eventByUserPrefix + event.UserID + ":" + event.ID
			if err := txn.Delete([]byte(userKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			bookIdx := eventByBookPrefix + event.BookID + ":" + event.ID
			if err := txn.Delete([]byte(bookIdx)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			userBookKey := eventByUserBookPrefix + event.UserID + ":" + event.BookID + ":" + event.ID
			if err := txn.Delete([]byte(userBookKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			// Delete user time index
			endedAtMs := event.EndedAt.UnixMilli()
			userTimeKey := fmt.Sprintf("%s%s:%013d:%s", eventByUserTimePrefix, event.UserID, endedAtMs, event.ID)
			if err := txn.Delete([]byte(userTimeKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		return nil
	})
}

// DeleteStateForBook deletes all playback state entries for a book across all users.
// Used when a book is deleted to clean up orphaned data.
func (s *Store) DeleteStateForBook(ctx context.Context, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Scan all progress entries and delete those matching the bookID
	prefix := []byte(progressPrefix)
	var keysToDelete [][]byte

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var state domain.PlaybackState
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &state)
			}); err != nil {
				continue
			}
			if state.BookID == bookID {
				keysToDelete = append(keysToDelete, append([]byte{}, item.Key()...))
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan state for book: %w", err)
	}

	if len(keysToDelete) == 0 {
		return nil
	}

	return s.db.Update(func(txn *badger.Txn) error {
		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetStateForUser retrieves all state records for a user.
func (s *Store) GetStateForUser(ctx context.Context, userID string) ([]*domain.PlaybackState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := progressPrefix + userID + ":"
	var results []*domain.PlaybackState

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			// IMPORTANT: Allocate on heap to avoid pointer aliasing.
			// Using `var progress` and taking &progress would cause all
			// pointers to potentially reference the same memory location.
			progress := new(domain.PlaybackState)
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, progress)
			})
			if err != nil {
				return err
			}
			results = append(results, progress)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}

// GetEventsForUserInRange retrieves events for a user within a time range.
// Uses the time-based index for efficient range scans (no full table scan).
// start is inclusive, end is exclusive. Zero start = beginning of time.
func (s *Store) GetEventsForUserInRange(
	ctx context.Context,
	userID string,
	start, end time.Time,
) ([]*domain.ListeningEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Build seek and end prefixes for the time index
	// Format: evt:idx:user:time:{userID}:{endedAtMs:020d}:{eventID}
	userPrefix := eventByUserTimePrefix + userID + ":"

	var startKey string
	if start.IsZero() {
		startKey = userPrefix // Start from beginning
	} else {
		startKey = fmt.Sprintf("%s%020d:", userPrefix, start.UnixMilli())
	}

	endMs := end.UnixMilli()

	var events []*domain.ListeningEvent

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(userPrefix)
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		// Collect event IDs within range
		var eventIDs []string
		for it.Seek([]byte(startKey)); it.ValidForPrefix([]byte(userPrefix)); it.Next() {
			key := string(it.Item().Key())

			// Extract timestamp from key to check end bound
			// Key format: evt:idx:user:time:{userID}:{endedAtMs:020d}:{eventID}
			rest := key[len(userPrefix):] // {endedAtMs:020d}:{eventID}
			if len(rest) < 21 {           // 20 digits + colon
				continue
			}

			var tsMs int64
			if _, err := fmt.Sscanf(rest[:20], "%d", &tsMs); err != nil {
				continue
			}

			// Stop if we've passed the end time
			if tsMs >= endMs {
				break
			}

			// Get event ID from index value
			err := it.Item().Value(func(val []byte) error {
				eventIDs = append(eventIDs, string(val))
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Batch fetch events (same transaction)
		events = make([]*domain.ListeningEvent, 0, len(eventIDs))
		for _, id := range eventIDs {
			item, err := txn.Get([]byte(listeningEventPrefix + id))
			if err != nil {
				continue // Skip missing events
			}

			// IMPORTANT: Allocate on heap to avoid loop variable pointer bug.
			event := new(domain.ListeningEvent)
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, event)
			}); err != nil {
				continue // Skip corrupt events
			}
			events = append(events, event)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return events, nil
}

// GetStateFinishedInRange retrieves books finished within a time range.
// start is inclusive, end is exclusive. Zero start = beginning of time.
func (s *Store) GetStateFinishedInRange(
	ctx context.Context,
	userID string,
	start, end time.Time,
) ([]*domain.PlaybackState, error) {
	allProgress, err := s.GetStateForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	var finished []*domain.PlaybackState
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
func (s *Store) GetContinueListening(ctx context.Context, userID string, limit int) ([]*domain.PlaybackState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Scan 1: All state for user
	allProgress, err := s.GetStateForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// DEBUG: Log all progress entries and their finished status
	if s.logger != nil {
		s.logger.Info("GetContinueListening: fetched all progress",
			"user_id", userID,
			"total_count", len(allProgress))
	}
	finishedCount := 0
	for _, p := range allProgress {
		if p.IsFinished {
			finishedCount++
			if s.logger != nil {
				s.logger.Info("GetContinueListening: FINISHED book will be excluded",
					"book_id", p.BookID,
					"current_position_ms", p.CurrentPositionMs,
					"is_finished", p.IsFinished)
			}
		}
	}
	if s.logger != nil {
		s.logger.Info("GetContinueListening: finished book count",
			"user_id", userID,
			"finished_count", finishedCount,
			"in_progress_count", len(allProgress)-finishedCount)
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
	// A book has progress if CurrentPositionMs > 0 (position has been tracked)
	var result []*domain.PlaybackState
	for _, p := range allProgress {
		hasProgress := p.CurrentPositionMs > 0
		if p.IsFinished || !hasProgress || hiddenBooks[p.BookID] {
			if p.IsFinished && s.logger != nil {
				s.logger.Debug("GetContinueListening: excluding finished book",
					"book_id", p.BookID)
			}
			continue
		}
		if s.logger != nil {
			s.logger.Debug("GetContinueListening: including in-progress book",
				"book_id", p.BookID,
				"current_position_ms", p.CurrentPositionMs,
				"is_finished", p.IsFinished)
		}
		result = append(result, p)
	}
	if s.logger != nil {
		s.logger.Info("GetContinueListening: final result count",
			"user_id", userID,
			"result_count", len(result))
	}

	// Sort by LastPlayedAt descending (most recent first)
	slices.SortFunc(result, func(a, b *domain.PlaybackState) int {
		return b.LastPlayedAt.Compare(a.LastPlayedAt)
	})

	// Apply limit
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}
