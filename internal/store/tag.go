package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
)

// Key prefixes for global tag storage.
// Tags are community-wide — no user ownership.
const (
	tagPrefix       = "tag:"            // tag:{id} → Tag JSON
	tagBySlugPrefix = "idx:tags:slug:"  // idx:tags:slug:{slug} → tagID
	tagBooksPrefix  = "idx:tags:books:" // idx:tags:books:{tagID}:{bookID} → empty
	bookTagsPrefix  = "idx:books:tags:" // idx:books:tags:{bookID}:{tagID} → empty
)

// Tag errors.
var (
	ErrTagNotFound = errors.New("tag not found")
	ErrTagExists   = errors.New("tag already exists")
)

// CreateTag creates a new global tag.
func (s *Store) CreateTag(ctx context.Context, t *domain.Tag) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Check if slug already exists globally.
		slugKey := []byte(tagBySlugPrefix + t.Slug)
		if _, err := txn.Get(slugKey); err == nil {
			return ErrTagExists
		}

		// Store tag.
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		key := []byte(tagPrefix + t.ID)
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Slug index (global).
		return txn.Set(slugKey, []byte(t.ID))
	})
}

// GetTagByID retrieves a tag by ID.
func (s *Store) GetTagByID(ctx context.Context, tagID string) (*domain.Tag, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var t domain.Tag
	key := []byte(tagPrefix + tagID)

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrTagNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &t)
		})
	})

	if err != nil {
		return nil, err
	}

	return &t, nil
}

// GetTagBySlug retrieves a tag by its normalized slug.
func (s *Store) GetTagBySlug(ctx context.Context, slug string) (*domain.Tag, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var tagID string
	slugKey := []byte(tagBySlugPrefix + slug)

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(slugKey)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrTagNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			tagID = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return s.GetTagByID(ctx, tagID)
}

// ListTags returns all tags ordered by book count (descending).
func (s *Store) ListTags(ctx context.Context) ([]*domain.Tag, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := []byte(tagPrefix)
	var tags []*domain.Tag

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchSize = 100

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var t domain.Tag
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &t)
			})
			if err != nil {
				continue
			}
			tags = append(tags, &t)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by book count descending, then by slug for stability.
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].BookCount != tags[j].BookCount {
			return tags[i].BookCount > tags[j].BookCount
		}
		return tags[i].Slug < tags[j].Slug
	})

	return tags, nil
}

// DeleteTag hard-deletes a tag.
// Future: This is for admin cleanup. Normal operations keep orphaned tags.
func (s *Store) DeleteTag(ctx context.Context, tagID string) error {
	t, err := s.GetTagByID(ctx, tagID)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Remove main record.
		key := []byte(tagPrefix + tagID)
		if err := txn.Delete(key); err != nil {
			return err
		}

		// Remove slug index.
		slugKey := []byte(tagBySlugPrefix + t.Slug)
		if err := txn.Delete(slugKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		// Remove all book associations.
		prefix := []byte(fmt.Sprintf("%s%s:", tagBooksPrefix, tagID))
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		var keysToDelete [][]byte
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			keyCopy := make([]byte, len(it.Item().Key()))
			copy(keyCopy, it.Item().Key())
			keysToDelete = append(keysToDelete, keyCopy)

			// Extract bookID for reverse index cleanup.
			parts := string(keyCopy)
			lastColon := strings.LastIndex(parts, ":")
			if lastColon != -1 {
				bookID := parts[lastColon+1:]
				reverseKey := []byte(fmt.Sprintf("%s%s:%s", bookTagsPrefix, bookID, tagID))
				keysToDelete = append(keysToDelete, reverseKey)
			}
		}

		for _, k := range keysToDelete {
			if err := txn.Delete(k); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}

		return nil
	})
}

// FindOrCreateTagBySlug atomically finds an existing tag by slug or creates a new one.
// Returns (tag, created, error) where created is true if a new tag was made.
func (s *Store) FindOrCreateTagBySlug(ctx context.Context, slug string) (*domain.Tag, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	// Try to find existing tag first (optimistic read).
	existing, err := s.GetTagBySlug(ctx, slug)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, ErrTagNotFound) {
		return nil, false, err
	}

	// Tag doesn't exist, create it.
	tagID, err := id.Generate("tag")
	if err != nil {
		return nil, false, err
	}

	now := time.Now()
	t := &domain.Tag{
		ID:        tagID,
		Slug:      slug,
		BookCount: 0,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.CreateTag(ctx, t); err != nil {
		if errors.Is(err, ErrTagExists) {
			// Race condition: another goroutine created it.
			existing, err := s.GetTagBySlug(ctx, slug)
			if err != nil {
				return nil, false, err
			}
			return existing, false, nil
		}
		return nil, false, err
	}

	return t, true, nil
}

// AddTagToBook adds a tag to a book. Idempotent.
// Increments the tag's book count within the same transaction.
func (s *Store) AddTagToBook(ctx context.Context, bookID, tagID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Check if relationship already exists.
		btKey := []byte(fmt.Sprintf("%s%s:%s", tagBooksPrefix, tagID, bookID))
		_, err := txn.Get(btKey)
		if err == nil {
			// Already exists, idempotent success.
			return nil
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		// Create forward index: tag -> book.
		if err := txn.Set(btKey, []byte{}); err != nil {
			return err
		}

		// Create reverse index: book -> tag.
		tbKey := []byte(fmt.Sprintf("%s%s:%s", bookTagsPrefix, bookID, tagID))
		if err := txn.Set(tbKey, []byte{}); err != nil {
			return err
		}

		// Increment book count on tag.
		return s.updateTagBookCountInTxn(txn, tagID, 1)
	})
}

// RemoveTagFromBook removes a tag from a book. Idempotent.
// Decrements the tag's book count within the same transaction.
func (s *Store) RemoveTagFromBook(ctx context.Context, bookID, tagID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Check if relationship exists.
		btKey := []byte(fmt.Sprintf("%s%s:%s", tagBooksPrefix, tagID, bookID))
		_, err := txn.Get(btKey)
		if errors.Is(err, badger.ErrKeyNotFound) {
			// Doesn't exist, idempotent success.
			return nil
		}
		if err != nil {
			return err
		}

		// Delete forward index: tag -> book.
		if err := txn.Delete(btKey); err != nil {
			return err
		}

		// Delete reverse index: book -> tag.
		tbKey := []byte(fmt.Sprintf("%s%s:%s", bookTagsPrefix, bookID, tagID))
		if err := txn.Delete(tbKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		// Decrement book count on tag.
		return s.updateTagBookCountInTxn(txn, tagID, -1)
	})
}

// updateTagBookCountInTxn updates the tag's book count within an existing transaction.
func (s *Store) updateTagBookCountInTxn(txn *badger.Txn, tagID string, delta int) error {
	key := []byte(tagPrefix + tagID)

	item, err := txn.Get(key)
	if err != nil {
		return err
	}

	var t domain.Tag
	if err := item.Value(func(val []byte) error {
		return json.Unmarshal(val, &t)
	}); err != nil {
		return err
	}

	t.BookCount += delta
	if t.BookCount < 0 {
		t.BookCount = 0 // Safety guard.
	}
	t.Touch()

	data, err := json.Marshal(t)
	if err != nil {
		return err
	}

	return txn.Set(key, data)
}

// GetTagsForBook returns all tags on a book.
func (s *Store) GetTagsForBook(ctx context.Context, bookID string) ([]*domain.Tag, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s%s:", bookTagsPrefix, bookID)
	var tagIDs []string

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			key := string(it.Item().Key())
			tagID := strings.TrimPrefix(key, prefix)
			tagIDs = append(tagIDs, tagID)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	tags := make([]*domain.Tag, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		t, err := s.GetTagByID(ctx, tagID)
		if err != nil {
			continue // Skip missing tags.
		}
		tags = append(tags, t)
	}

	// Sort alphabetically by slug.
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Slug < tags[j].Slug
	})

	return tags, nil
}

// GetTagIDsForBook returns tag IDs for a book (for search indexing).
func (s *Store) GetTagIDsForBook(ctx context.Context, bookID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s%s:", bookTagsPrefix, bookID)
	var tagIDs []string

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			key := string(it.Item().Key())
			tagID := strings.TrimPrefix(key, prefix)
			tagIDs = append(tagIDs, tagID)
		}
		return nil
	})

	return tagIDs, err
}

// GetBookIDsForTag returns all book IDs with a specific tag.
func (s *Store) GetBookIDsForTag(ctx context.Context, tagID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s%s:", tagBooksPrefix, tagID)
	var bookIDs []string

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			key := string(it.Item().Key())
			bookID := strings.TrimPrefix(key, prefix)
			bookIDs = append(bookIDs, bookID)
		}
		return nil
	})

	return bookIDs, err
}

// CleanupTagsForDeletedBook removes all tag associations for a deleted book.
// Call this when deleting a book to maintain tag book counts.
func (s *Store) CleanupTagsForDeletedBook(ctx context.Context, bookID string) error {
	tagIDs, err := s.GetTagIDsForBook(ctx, bookID)
	if err != nil {
		return err
	}

	for _, tagID := range tagIDs {
		if err := s.RemoveTagFromBook(ctx, bookID, tagID); err != nil {
			// Log but continue — best effort cleanup.
			if s.logger != nil {
				s.logger.Warn("failed to remove tag from deleted book", "book_id", bookID, "tag_id", tagID, "error", err)
			}
		}
	}

	return nil
}

// RecalculateTagBookCount recalculates the book count for a tag from the index.
// Use for data repair or verification.
func (s *Store) RecalculateTagBookCount(ctx context.Context, tagID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	prefix := fmt.Sprintf("%s%s:", tagBooksPrefix, tagID)
	count := 0

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			count++
		}
		return nil
	})

	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(tagPrefix + tagID)

		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		var t domain.Tag
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &t)
		}); err != nil {
			return err
		}

		t.BookCount = count
		t.Touch()

		data, err := json.Marshal(t)
		if err != nil {
			return err
		}

		return txn.Set(key, data)
	})
}

// GetTagSlugsForBook returns tag slugs for a book (for search indexing).
func (s *Store) GetTagSlugsForBook(ctx context.Context, bookID string) ([]string, error) {
	tags, err := s.GetTagsForBook(ctx, bookID)
	if err != nil {
		return nil, err
	}

	slugs := make([]string, len(tags))
	for i, t := range tags {
		slugs[i] = t.Slug
	}

	return slugs, nil
}
