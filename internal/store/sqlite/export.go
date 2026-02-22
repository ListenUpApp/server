package sqlite

import (
	"context"
	"fmt"
	"iter"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// --- Stream methods for export/backup ---

// StreamBooks returns an iterator over all non-deleted books.
// Audio files and chapters are loaded for each book.
func (s *Store) StreamBooks(ctx context.Context) iter.Seq2[*domain.Book, error] {
	return func(yield func(*domain.Book, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+bookColumns+` FROM books WHERE deleted_at IS NULL ORDER BY id`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			b, err := scanBook(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// Load audio files and chapters.
			b.AudioFiles, err = s.loadBookAudioFiles(ctx, s.db, b.ID)
			if err != nil {
				if !yield(nil, fmt.Errorf("load audio files for %s: %w", b.ID, err)) {
					return
				}
				continue
			}
			b.Chapters, err = s.loadBookChapters(ctx, s.db, b.ID)
			if err != nil {
				if !yield(nil, fmt.Errorf("load chapters for %s: %w", b.ID, err)) {
					return
				}
				continue
			}

			if !yield(b, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// StreamContributors returns an iterator over all non-deleted contributors.
func (s *Store) StreamContributors(ctx context.Context) iter.Seq2[*domain.Contributor, error] {
	return func(yield func(*domain.Contributor, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+contributorColumns+` FROM contributors WHERE deleted_at IS NULL ORDER BY id`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			c, err := scanContributor(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(c, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// StreamSeries returns an iterator over all non-deleted series.
func (s *Store) StreamSeries(ctx context.Context) iter.Seq2[*domain.Series, error] {
	return func(yield func(*domain.Series, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+seriesColumns+` FROM series WHERE deleted_at IS NULL ORDER BY id`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			sr, err := scanSeries(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(sr, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// StreamCollections returns an iterator over all collections.
// BookIDs are loaded for each collection.
func (s *Store) StreamCollections(ctx context.Context) iter.Seq2[*domain.Collection, error] {
	return func(yield func(*domain.Collection, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+collectionColumns+` FROM collections ORDER BY id`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			coll, err := scanCollection(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// Load BookIDs for this collection.
			coll.BookIDs, err = s.loadBookIDs(ctx, coll.ID)
			if err != nil {
				if !yield(nil, fmt.Errorf("load book IDs for %s: %w", coll.ID, err)) {
					return
				}
				continue
			}

			if !yield(coll, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// StreamLenses is an alias for StreamShelves for backward compatibility.
func (s *Store) StreamLenses(ctx context.Context) iter.Seq2[*domain.Shelf, error] {
	return s.StreamShelves(ctx)
}

// StreamShelves returns an iterator over all shelves.
// BookIDs are loaded for each shelf.
func (s *Store) StreamShelves(ctx context.Context) iter.Seq2[*domain.Shelf, error] {
	return func(yield func(*domain.Shelf, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+shelfColumns+` FROM shelves ORDER BY id`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			l, err := scanShelf(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// Load BookIDs for this lens.
			l.BookIDs, err = s.loadShelfBookIDs(ctx, l.ID)
			if err != nil {
				if !yield(nil, fmt.Errorf("load shelf book IDs for %s: %w", l.ID, err)) {
					return
				}
				continue
			}

			if !yield(l, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// StreamActivities returns an iterator over all activities.
func (s *Store) StreamActivities(ctx context.Context) iter.Seq2[*domain.Activity, error] {
	return func(yield func(*domain.Activity, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+activityColumns+` FROM activities ORDER BY created_at DESC`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			a, err := scanActivity(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(a, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// StreamListeningEvents returns an iterator over all listening events.
func (s *Store) StreamListeningEvents(ctx context.Context) iter.Seq2[*domain.ListeningEvent, error] {
	return func(yield func(*domain.ListeningEvent, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+listeningEventColumns+` FROM listening_events ORDER BY created_at ASC`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			e, err := scanListeningEvent(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(e, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// StreamProfiles returns an iterator over all user profiles.
func (s *Store) StreamProfiles(ctx context.Context) iter.Seq2[*domain.UserProfile, error] {
	return func(yield func(*domain.UserProfile, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+profileColumns+` FROM user_profiles ORDER BY user_id`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			p, err := scanProfile(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(p, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// StreamCollectionShares returns an iterator over all collection shares.
func (s *Store) StreamCollectionShares(ctx context.Context) iter.Seq2[*domain.CollectionShare, error] {
	return func(yield func(*domain.CollectionShare, error) bool) {
		rows, err := s.db.QueryContext(ctx,
			`SELECT `+shareColumns+` FROM collection_shares ORDER BY id`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}

			share, err := scanShare(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !yield(share, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// --- Clear/restore methods ---

// ClearAllData deletes all data from all tables.
// Uses a transaction to ensure atomicity. Tables are deleted in order
// to respect foreign key constraints.
func (s *Store) ClearAllData(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete in dependency order (children first, then parents).
	tables := []string{
		"abs_import_progress",
		"abs_import_sessions",
		"abs_import_books",
		"abs_import_users",
		"abs_imports",
		"transcode_jobs",
		"user_stats",
		"user_milestone_states",
		"activities",
		"book_reading_sessions",
		"book_preferences",
		"playback_state",
		"listening_events",
		"shelf_books",
		"shelves",
		"collection_shares",
		"collection_books",
		"collections",
		"book_genres",
		"book_tags",
		"book_chapters",
		"book_audio_files",
		"book_contributors",
		"book_series",
		"books",
		"genre_aliases",
		"unmapped_genres",
		"genres",
		"tags",
		"contributors",
		"series",
		"invites",
		"user_profiles",
		"user_settings",
		"sessions",
		"libraries",
		"server_settings",
		"instance",
		"users",
		"audible_cache_books",
		"audible_cache_chapters",
		"audible_cache_search",
	}

	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return fmt.Errorf("clear table %s: %w", table, err)
		}
	}

	return tx.Commit()
}

// ClearAllProgress deletes all playback state records.
func (s *Store) ClearAllProgress(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM playback_state`)
	return err
}

// SaveProgress upserts a playback state (alias for UpsertState, used by restore).
func (s *Store) SaveProgress(ctx context.Context, progress *domain.PlaybackState) error {
	return s.UpsertState(ctx, progress)
}

// GetCollectionNoAccessCheck delegates to GetCollection with empty userID.
func (s *Store) GetCollectionNoAccessCheck(ctx context.Context, id string) (*domain.Collection, error) {
	return s.GetCollection(ctx, id, "")
}

// UpdateCollectionNoAccessCheck delegates to UpdateCollection with empty userID.
func (s *Store) UpdateCollectionNoAccessCheck(ctx context.Context, coll *domain.Collection) error {
	return s.UpdateCollection(ctx, coll, "")
}

// GetTagByIDForRestore retrieves a tag by its ID (alias for GetTagByID, used by restore).
func (s *Store) GetTagByIDForRestore(ctx context.Context, tagID string) (*domain.Tag, error) {
	return s.GetTagByID(ctx, tagID)
}

// UpdateTagForRestore updates a tag's slug and timestamps.
// Returns store.ErrNotFound if the tag does not exist.
func (s *Store) UpdateTagForRestore(ctx context.Context, t *domain.Tag) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE tags SET slug = ?, created_at = ?, updated_at = ?
		WHERE id = ?`,
		t.Slug,
		formatTime(t.CreatedAt),
		formatTime(t.UpdatedAt),
		t.ID,
	)
	if err != nil {
		return err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
