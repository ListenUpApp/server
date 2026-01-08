package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// Activity storage key prefixes.
// Uses inverted timestamps for descending order (newest first) during forward iteration.
const (
	activityPrefix        = "activity:"
	activityIdxTimePrefix = "activity:idx:time:"
	activityIdxUserPrefix = "activity:idx:user:"
	activityIdxBookPrefix = "activity:idx:book:"
)

// invertedTimestamp returns a string that sorts in descending order.
// Uses MaxInt64 - UnixNano to ensure newest timestamps come first during forward iteration.
func invertedTimestamp(t time.Time) string {
	inverted := math.MaxInt64 - t.UnixNano()
	return fmt.Sprintf("%019d", inverted)
}

// CreateActivity stores a new activity with all indexes in a single transaction.
func (s *Store) CreateActivity(ctx context.Context, activity *domain.Activity) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("marshaling activity: %w", err)
	}

	invertedTS := invertedTimestamp(activity.CreatedAt)

	return s.db.Update(func(txn *badger.Txn) error {
		// Primary key: activity:{id} → Activity JSON
		primaryKey := []byte(activityPrefix + activity.ID)
		if err := txn.Set(primaryKey, data); err != nil {
			return fmt.Errorf("setting primary key: %w", err)
		}

		// Time index: activity:idx:time:{inverted_timestamp}:{id} → "" (key-only)
		// This allows scanning newest-first without reverse iteration
		timeKey := []byte(activityIdxTimePrefix + invertedTS + ":" + activity.ID)
		if err := txn.Set(timeKey, []byte{}); err != nil {
			return fmt.Errorf("setting time index: %w", err)
		}

		// User index: activity:idx:user:{userId}:{inverted_timestamp}:{id} → ""
		userKey := []byte(activityIdxUserPrefix + activity.UserID + ":" + invertedTS + ":" + activity.ID)
		if err := txn.Set(userKey, []byte{}); err != nil {
			return fmt.Errorf("setting user index: %w", err)
		}

		// Book index (only for book-related activities)
		if activity.BookID != "" {
			bookKey := []byte(activityIdxBookPrefix + activity.BookID + ":" + invertedTS + ":" + activity.ID)
			if err := txn.Set(bookKey, []byte{}); err != nil {
				return fmt.Errorf("setting book index: %w", err)
			}
		}

		return nil
	})
}

// GetActivity retrieves a single activity by ID.
func (s *Store) GetActivity(ctx context.Context, id string) (*domain.Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var activity domain.Activity
	err := s.get([]byte(activityPrefix+id), &activity)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, fmt.Errorf("activity %s: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("getting activity %s: %w", id, err)
	}

	return &activity, nil
}

// GetActivitiesFeed retrieves the global activity feed sorted by CreatedAt descending.
// Use 'before' for cursor-based pagination (pass the CreatedAt of the last item from previous page).
// Returns up to 'limit' activities.
func (s *Store) GetActivitiesFeed(ctx context.Context, limit int, before *time.Time) ([]*domain.Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var activities []*domain.Activity

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Key-only index, no values to fetch
		opts.Prefix = []byte(activityIdxTimePrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		// Determine seek position
		seekKey := []byte(activityIdxTimePrefix)
		if before != nil {
			// Start after the 'before' timestamp
			// We use inverted timestamp, so "after" means a larger inverted value
			seekKey = []byte(activityIdxTimePrefix + invertedTimestamp(*before))
		}

		for it.Seek(seekKey); it.ValidForPrefix([]byte(activityIdxTimePrefix)); it.Next() {
			if len(activities) >= limit {
				break
			}

			// Extract activity ID from key: activity:idx:time:{inverted_ts}:{id}
			key := string(it.Item().Key())
			activityID := extractActivityIDFromTimeKey(key)
			if activityID == "" {
				continue
			}

			// Skip the exact 'before' item if we're paginating
			if before != nil {
				// The first item might be the cursor item itself if seek lands on it
				// Since we want items strictly before, we need to check
				activity, err := s.getActivityInTxn(txn, activityID)
				if err != nil {
					continue
				}
				if activity.CreatedAt.Equal(*before) || activity.CreatedAt.After(*before) {
					continue
				}
				activities = append(activities, activity)
			} else {
				activity, err := s.getActivityInTxn(txn, activityID)
				if err != nil {
					continue
				}
				activities = append(activities, activity)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("fetching activity feed: %w", err)
	}

	return activities, nil
}

// GetUserActivities retrieves activities for a specific user sorted by CreatedAt descending.
func (s *Store) GetUserActivities(ctx context.Context, userID string, limit int) ([]*domain.Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var activities []*domain.Activity
	indexPrefix := []byte(activityIdxUserPrefix + userID + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = indexPrefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(indexPrefix); it.ValidForPrefix(indexPrefix); it.Next() {
			if len(activities) >= limit {
				break
			}

			// Extract activity ID from key
			key := string(it.Item().Key())
			activityID := extractActivityIDFromUserKey(key, userID)
			if activityID == "" {
				continue
			}

			activity, err := s.getActivityInTxn(txn, activityID)
			if err != nil {
				continue
			}
			activities = append(activities, activity)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("fetching user activities: %w", err)
	}

	return activities, nil
}

// GetBookActivities retrieves activities for a specific book sorted by CreatedAt descending.
func (s *Store) GetBookActivities(ctx context.Context, bookID string, limit int) ([]*domain.Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var activities []*domain.Activity
	indexPrefix := []byte(activityIdxBookPrefix + bookID + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = indexPrefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(indexPrefix); it.ValidForPrefix(indexPrefix); it.Next() {
			if len(activities) >= limit {
				break
			}

			// Extract activity ID from key
			key := string(it.Item().Key())
			activityID := extractActivityIDFromBookKey(key, bookID)
			if activityID == "" {
				continue
			}

			activity, err := s.getActivityInTxn(txn, activityID)
			if err != nil {
				continue
			}
			activities = append(activities, activity)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("fetching book activities: %w", err)
	}

	return activities, nil
}

// getActivityInTxn retrieves an activity within an existing transaction.
func (s *Store) getActivityInTxn(txn *badger.Txn, id string) (*domain.Activity, error) {
	item, err := txn.Get([]byte(activityPrefix + id))
	if err != nil {
		return nil, err
	}

	var activity domain.Activity
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &activity)
	})
	if err != nil {
		return nil, err
	}

	return &activity, nil
}

// extractActivityIDFromTimeKey extracts activity ID from time index key.
// Key format: activity:idx:time:{inverted_ts}:{id}.
func extractActivityIDFromTimeKey(key string) string {
	const prefix = activityIdxTimePrefix
	if len(key) <= len(prefix)+20 { // 19 digits + colon
		return ""
	}
	// Skip prefix and inverted timestamp (19 digits) and colon
	remainder := key[len(prefix)+20:]
	return remainder
}

// extractActivityIDFromUserKey extracts activity ID from user index key.
// Key format: activity:idx:user:{userId}:{inverted_ts}:{id}.
func extractActivityIDFromUserKey(key, userID string) string {
	prefix := activityIdxUserPrefix + userID + ":"
	if len(key) <= len(prefix)+20 {
		return ""
	}
	remainder := key[len(prefix)+20:]
	return remainder
}

// extractActivityIDFromBookKey extracts activity ID from book index key.
// Key format: activity:idx:book:{bookId}:{inverted_ts}:{id}.
func extractActivityIDFromBookKey(key, bookID string) string {
	prefix := activityIdxBookPrefix + bookID + ":"
	if len(key) <= len(prefix)+20 {
		return ""
	}
	remainder := key[len(prefix)+20:]
	return remainder
}

// Milestone state storage

const milestoneStatePrefix = "milestone_state:"

// GetUserMilestoneState retrieves the milestone tracking state for a user.
// Returns nil if no state exists (user hasn't had any milestones tracked yet).
func (s *Store) GetUserMilestoneState(ctx context.Context, userID string) (*domain.UserMilestoneState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var state domain.UserMilestoneState
	err := s.get([]byte(milestoneStatePrefix+userID), &state)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, nil // No state yet, not an error
		}
		return nil, fmt.Errorf("getting milestone state for %s: %w", userID, err)
	}

	return &state, nil
}

// UpdateUserMilestoneState updates or creates the milestone tracking state for a user.
func (s *Store) UpdateUserMilestoneState(ctx context.Context, userID string, streakDays, listenHours int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	state := &domain.UserMilestoneState{
		UserID:               userID,
		LastStreakDays:       streakDays,
		LastListenHoursTotal: listenHours,
		UpdatedAt:            time.Now(),
	}

	return s.set([]byte(milestoneStatePrefix+userID), state)
}
