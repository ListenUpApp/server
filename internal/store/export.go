package store

import (
	"context"
	"iter"

	"encoding/json/v2"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// StreamBooks returns an iterator over all books for backup export.
func (s *Store) StreamBooks(ctx context.Context) iter.Seq2[*domain.Book, error] {
	return streamEntities[domain.Book](s.db, ctx, bookPrefix)
}

// StreamContributors returns an iterator over all contributors.
func (s *Store) StreamContributors(ctx context.Context) iter.Seq2[*domain.Contributor, error] {
	return streamEntities[domain.Contributor](s.db, ctx, contributorPrefix)
}

// StreamSeries returns an iterator over all series.
func (s *Store) StreamSeries(ctx context.Context) iter.Seq2[*domain.Series, error] {
	return streamEntities[domain.Series](s.db, ctx, seriesPrefix)
}

// StreamCollections returns an iterator over all collections.
func (s *Store) StreamCollections(ctx context.Context) iter.Seq2[*domain.Collection, error] {
	return streamEntities[domain.Collection](s.db, ctx, collectionPrefix)
}

// StreamLenses returns an iterator over all lenses.
func (s *Store) StreamLenses(ctx context.Context) iter.Seq2[*domain.Lens, error] {
	return streamEntities[domain.Lens](s.db, ctx, lensPrefix)
}

// StreamActivities returns an iterator over all activities.
func (s *Store) StreamActivities(ctx context.Context) iter.Seq2[*domain.Activity, error] {
	return streamEntities[domain.Activity](s.db, ctx, activityPrefix)
}

// StreamListeningEvents returns an iterator over all listening events.
func (s *Store) StreamListeningEvents(ctx context.Context) iter.Seq2[*domain.ListeningEvent, error] {
	return streamEntities[domain.ListeningEvent](s.db, ctx, listeningEventPrefix)
}

// StreamProfiles returns an iterator over all user profiles.
func (s *Store) StreamProfiles(ctx context.Context) iter.Seq2[*domain.UserProfile, error] {
	return streamEntities[domain.UserProfile](s.db, ctx, profilePrefix)
}

// streamEntities is a generic streaming iterator for any entity type.
func streamEntities[T any](db *badger.DB, ctx context.Context, prefix string) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		_ = db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.Prefix = []byte(prefix)
			opts.PrefetchValues = true

			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
				if ctx.Err() != nil {
					yield(nil, ctx.Err())
					return ctx.Err()
				}

				// Skip index keys (they have patterns like "prefix:idx:")
				key := string(it.Item().Key())
				keyRemainder := key[len(prefix):]
				if len(keyRemainder) >= 4 && keyRemainder[:4] == "idx:" {
					continue
				}

				var entity T
				err := it.Item().Value(func(val []byte) error {
					return json.Unmarshal(val, &entity)
				})

				if err != nil {
					if !yield(nil, err) {
						return nil
					}
					continue
				}

				if !yield(&entity, nil) {
					return nil
				}
			}

			return nil
		})
	}
}

// ClearAllProgress removes all playback progress records.
func (s *Store) ClearAllProgress(ctx context.Context) error {
	return s.deleteByPrefix(ctx, progressPrefix)
}

// ClearAllData removes all data from the store. Used for full restore.
func (s *Store) ClearAllData(ctx context.Context) error {
	prefixes := []string{
		"user:",
		libraryPrefix,
		bookPrefix,
		contributorPrefix,
		seriesPrefix,
		genrePrefix,
		tagPrefix,
		collectionPrefix,
		"share:",
		lensPrefix,
		activityPrefix,
		listeningEventPrefix,
		"session:",
		progressPrefix,
		"instance:",
		"settings:",
		profilePrefix,
		bookPreferencesPrefix,
		userSettingsPrefix,
	}

	for _, prefix := range prefixes {
		if err := s.deleteByPrefix(ctx, prefix); err != nil {
			return err
		}
	}

	// Clear genre cache
	s.InvalidateGenreCache()

	return nil
}

func (s *Store) deleteByPrefix(ctx context.Context, prefix string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			key := it.Item().KeyCopy(nil)
			if err := txn.Delete(key); err != nil {
				return err
			}
		}

		return nil
	})
}

// SaveProgress saves or updates playback progress.
func (s *Store) SaveProgress(ctx context.Context, progress *domain.PlaybackProgress) error {
	key := progressPrefix + domain.ProgressID(progress.UserID, progress.BookID)
	return s.set([]byte(key), progress)
}

// GetCollectionNoAccessCheck retrieves a collection without access control.
// For backup/restore operations only.
func (s *Store) GetCollectionNoAccessCheck(ctx context.Context, id string) (*domain.Collection, error) {
	key := []byte(collectionPrefix + id)

	var coll domain.Collection
	if err := s.get(key, &coll); err != nil {
		return nil, err
	}

	return &coll, nil
}

// UpdateCollectionNoAccessCheck updates a collection without access control.
// For backup/restore operations only.
func (s *Store) UpdateCollectionNoAccessCheck(ctx context.Context, coll *domain.Collection) error {
	key := []byte(collectionPrefix + coll.ID)

	coll.UpdatedAt = coll.UpdatedAt // preserve original UpdatedAt
	return s.set(key, coll)
}

// GetTagByIDForRestore retrieves a tag by ID for restore operations.
func (s *Store) GetTagByIDForRestore(ctx context.Context, tagID string) (*domain.Tag, error) {
	key := []byte(tagPrefix + tagID)

	var tag domain.Tag
	if err := s.get(key, &tag); err != nil {
		return nil, err
	}

	return &tag, nil
}

// UpdateTagForRestore updates a tag for restore operations.
func (s *Store) UpdateTagForRestore(ctx context.Context, t *domain.Tag) error {
	key := []byte(tagPrefix + t.ID)
	return s.set(key, t)
}
