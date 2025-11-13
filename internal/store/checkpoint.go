package store

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const (
	checkpointKey = "sync:checkpoint"
)

// GetLibraryCheckpoint returns the most recent UpdatedAt time stamp
// across ALL books. this represents when the library was last changed.
func (s *Store) GetLibraryCheckpoint(ctx context.Context) (time.Time, error) {
	// Fow now iterate all books and find the latest UpdatedAt
	// TODO: Optimize by caching checkpoint in Library.SyncCheckpoint field
	// and updating it on every entity change
	var latest time.Time

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(bookPrefix)
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var book domain.Book
				if err := json.Unmarshal(val, &book); err != nil {
					return err
				}
				if book.UpdatedAt.After(latest) {
					latest = book.UpdatedAt
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("get library checkpoint: %w", err)
	}

	// if no books exist, return zero time
	return latest, nil
}

func (s *Store) SetLibraryCheckppint(ctx context.Context, t time.Time) error {
	// for now, this is a no op since we computer checkpoint dynamically

	return nil
}
