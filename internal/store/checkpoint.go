package store

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// GetLibraryCheckpoint returns the most recent UpdatedAt timestamp
// across ALL entities (books, contributors, series). This represents when the library was last changed.
func (s *Store) GetLibraryCheckpoint(_ context.Context) (time.Time, error) {
	// TODO: Optimize by caching checkpoint in Library.SyncCheckpoint field
	// and updating it on every entity change.
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

	return latest, nil
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

// SetLibraryCheckppint sets the library checkpoint timestamp.
func (s *Store) SetLibraryCheckppint(_ context.Context, _ time.Time) error {
	// for now, this is a no op since we computer checkpoint dynamically.

	return nil
}
