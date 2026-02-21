package store

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// checkpointTTL controls how long the in-memory checkpoint cache is valid.
// Sync endpoints can hammer GetLibraryCheckpoint — a 10-second cache prevents
// repeated full table scans without noticeably delaying sync detection.
const checkpointTTL = 10 * time.Second

var (
	checkpointCacheMu  sync.Mutex
	checkpointCache    time.Time
	checkpointCachedAt time.Time
)

// GetLibraryCheckpoint returns the most recent UpdatedAt timestamp
// across ALL entities (books, contributors, series). This represents when the library was last changed.
//
// Results are cached for checkpointTTL to prevent repeated full table scans
// when the sync endpoint is polled frequently.
func (s *Store) GetLibraryCheckpoint(_ context.Context) (time.Time, error) {
	checkpointCacheMu.Lock()
	if !checkpointCachedAt.IsZero() && time.Since(checkpointCachedAt) < checkpointTTL {
		cached := checkpointCache
		checkpointCacheMu.Unlock()
		return cached, nil
	}
	checkpointCacheMu.Unlock()

	var latest time.Time

	err := s.db.View(func(txn *badger.Txn) error {
		// Check books
		if err := s.checkEntityTimestamp(txn, []byte(bookPrefix), &latest, func(val []byte) (time.Time, error) {
			var book domain.Book
			if err := json.Unmarshal(val, &book); err != nil {
				return time.Time{}, err
			}
			return book.UpdatedAt, nil
		}); err != nil {
			return err
		}

		// Check contributors
		if err := s.checkEntityTimestamp(txn, []byte(contributorPrefix), &latest, func(val []byte) (time.Time, error) {
			var contributor domain.Contributor
			if err := json.Unmarshal(val, &contributor); err != nil {
				return time.Time{}, err
			}
			return contributor.UpdatedAt, nil
		}); err != nil {
			return err
		}

		// Check series
		if err := s.checkEntityTimestamp(txn, []byte(seriesPrefix), &latest, func(val []byte) (time.Time, error) {
			var series domain.Series
			if err := json.Unmarshal(val, &series); err != nil {
				return time.Time{}, err
			}
			return series.UpdatedAt, nil
		}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("get library checkpoint: %w", err)
	}

	// Update cache
	checkpointCacheMu.Lock()
	checkpointCache = latest
	checkpointCachedAt = time.Now()
	checkpointCacheMu.Unlock()

	return latest, nil
}

// InvalidateCheckpointCache clears the checkpoint cache.
// Call this after any library mutation (book/contributor/series write) to ensure
// the next sync picks up the change within checkpointTTL.
// TODO: Replace with a dedicated sys:checkpoint key as part of the SQLite migration.
func (s *Store) InvalidateCheckpointCache() {
	checkpointCacheMu.Lock()
	checkpointCachedAt = time.Time{}
	checkpointCacheMu.Unlock()
}

// checkEntityTimestamp iterates entities with a given prefix and updates latest timestamp.
func (s *Store) checkEntityTimestamp(txn *badger.Txn, prefix []byte, latest *time.Time, extractTime func([]byte) (time.Time, error)) error {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	opts.PrefetchValues = true

	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()

		err := item.Value(func(val []byte) error {
			updatedAt, err := extractTime(val)
			if err != nil {
				return err
			}
			if updatedAt.After(*latest) {
				*latest = updatedAt
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}
