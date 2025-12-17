package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const bookPreferencesPrefix = "bookprefs:"

// ErrBookPreferencesNotFound is returned when book preferences are not found.
var ErrBookPreferencesNotFound = ErrNotFound.WithMessage("book preferences not found")

// GetBookPreferences retrieves preferences for a user+book.
func (s *Store) GetBookPreferences(ctx context.Context, userID, bookID string) (*domain.BookPreferences, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := bookPreferencesPrefix + domain.BookPreferencesID(userID, bookID)
	var prefs domain.BookPreferences

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrBookPreferencesNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &prefs)
		})
	})

	if err != nil {
		return nil, err
	}
	return &prefs, nil
}

// UpsertBookPreferences creates or updates book preferences.
func (s *Store) UpsertBookPreferences(ctx context.Context, prefs *domain.BookPreferences) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := bookPreferencesPrefix + domain.BookPreferencesID(prefs.UserID, prefs.BookID)
	data, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("marshal preferences: %w", err)
	}

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
}

// DeleteBookPreferences removes book preferences.
func (s *Store) DeleteBookPreferences(ctx context.Context, userID, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := bookPreferencesPrefix + domain.BookPreferencesID(userID, bookID)
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// GetAllBookPreferences retrieves all book preferences for a user.
// Used by GetContinueListening for batch lookup of hidden books.
func (s *Store) GetAllBookPreferences(ctx context.Context, userID string) ([]*domain.BookPreferences, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Prefix: bookprefs:userID:prefs: (matching BookPreferencesID format)
	prefix := bookPreferencesPrefix + userID + ":prefs:"
	var results []*domain.BookPreferences

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			var prefs domain.BookPreferences
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &prefs)
			})
			if err != nil {
				return err
			}
			results = append(results, &prefs)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return results, nil
}
