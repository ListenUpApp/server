package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const userStatsPrefix = "user_stats:"

// GetUserStats retrieves pre-aggregated stats for a user.
// Returns nil, nil if no stats exist yet.
func (s *Store) GetUserStats(ctx context.Context, userID string) (*domain.UserStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var stats domain.UserStats
	err := s.get([]byte(userStatsPrefix+userID), &stats)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting user stats for %s: %w", userID, err)
	}
	return &stats, nil
}

// GetAllUserStats retrieves stats for all users.
func (s *Store) GetAllUserStats(ctx context.Context) ([]*domain.UserStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var results []*domain.UserStats
	prefix := []byte(userStatsPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			stats := new(domain.UserStats)
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, stats)
			})
			if err != nil {
				continue
			}
			results = append(results, stats)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// EnsureUserStats creates a user_stats row if it doesn't exist.
func (s *Store) EnsureUserStats(ctx context.Context, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	existing, err := s.GetUserStats(ctx, userID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil // Already exists
	}

	stats := &domain.UserStats{
		UserID:    userID,
		UpdatedAt: time.Now(),
	}
	return s.set([]byte(userStatsPrefix+userID), stats)
}

// IncrementListenTime atomically increments the total listen time for a user.
func (s *Store) IncrementListenTime(ctx context.Context, userID string, deltaMs int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(userStatsPrefix + userID)
		stats := &domain.UserStats{UserID: userID}

		item, err := txn.Get(key)
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err == nil {
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, stats)
			}); err != nil {
				return err
			}
		}

		stats.TotalListenTimeMs += deltaMs
		stats.UpdatedAt = time.Now()

		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
}

// IncrementBooksFinished atomically increments the books finished count.
func (s *Store) IncrementBooksFinished(ctx context.Context, userID string, delta int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(userStatsPrefix + userID)
		stats := &domain.UserStats{UserID: userID}

		item, err := txn.Get(key)
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err == nil {
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, stats)
			}); err != nil {
				return err
			}
		}

		stats.TotalBooksFinished += delta
		if stats.TotalBooksFinished < 0 {
			stats.TotalBooksFinished = 0
		}
		stats.UpdatedAt = time.Now()

		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
}

// UpdateUserStreak updates the streak fields and last listened date for a user.
func (s *Store) UpdateUserStreak(ctx context.Context, userID string, currentStreak, longestStreak int, lastListenedDate string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(userStatsPrefix + userID)
		stats := &domain.UserStats{UserID: userID}

		item, err := txn.Get(key)
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err == nil {
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, stats)
			}); err != nil {
				return err
			}
		}

		stats.CurrentStreakDays = currentStreak
		stats.LongestStreakDays = longestStreak
		stats.LastListenedDate = lastListenedDate
		stats.UpdatedAt = time.Now()

		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
}

// SetUserStats saves a complete UserStats (used for backfill).
func (s *Store) SetUserStats(ctx context.Context, stats *domain.UserStats) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.set([]byte(userStatsPrefix+stats.UserID), stats)
}

// UpdateUserStatsLastListened updates the last listened date for a user.
func (s *Store) UpdateUserStatsLastListened(ctx context.Context, userID string, date string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(userStatsPrefix + userID)
		stats := &domain.UserStats{UserID: userID}

		item, err := txn.Get(key)
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err == nil {
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, stats)
			}); err != nil {
				return err
			}
		}

		stats.LastListenedDate = date
		stats.UpdatedAt = time.Now()

		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
}

// UpdateUserStatsFromEvent atomically ensures, increments listen time, and updates last listened date
// in a single BadgerDB transaction to prevent race conditions.
func (s *Store) UpdateUserStatsFromEvent(ctx context.Context, userID string, deltaMs int64, lastListenedDate string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(userStatsPrefix + userID)
		stats := &domain.UserStats{UserID: userID}

		item, err := txn.Get(key)
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err == nil {
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, stats)
			}); err != nil {
				return err
			}
		}

		stats.TotalListenTimeMs += deltaMs
		stats.LastListenedDate = lastListenedDate
		stats.UpdatedAt = time.Now()

		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
}

// IncrementBooksFinishedAtomic reads and increments books finished in one transaction.
func (s *Store) IncrementBooksFinishedAtomic(ctx context.Context, userID string, delta int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(userStatsPrefix + userID)
		stats := &domain.UserStats{UserID: userID}

		item, err := txn.Get(key)
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err == nil {
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, stats)
			}); err != nil {
				return err
			}
		}

		stats.TotalBooksFinished += delta
		if stats.TotalBooksFinished < 0 {
			stats.TotalBooksFinished = 0
		}
		stats.UpdatedAt = time.Now()

		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
}

// ClearAllUserStats deletes all user_stats keys. Used after backup restore.
func (s *Store) ClearAllUserStats(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		prefix := []byte(userStatsPrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		var keys [][]byte
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().KeyCopy(nil)
			keys = append(keys, key)
		}

		for _, key := range keys {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}
