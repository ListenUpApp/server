package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/genre"
	"github.com/listenupapp/listenup-server/internal/id"
)

// Key prefixes for tag storage.
const (
	tagPrefix        = "tag:"
	tagByOwnerPrefix = "idx:tag:owner:" // ownerID:tagID -> empty
	tagBySlugPrefix  = "idx:tag:slug:"  // ownerID:slug -> tagID
	bookTagPrefix    = "idx:book:tag:"  // bookID:userID:tagID -> empty
	tagBookPrefix    = "idx:tag:book:"  // tagID:bookID -> empty
)

// Tag errors.
var (
	ErrTagNotFound = errors.New("tag not found")
	ErrTagExists   = errors.New("tag already exists")
)

// CreateTag creates a new tag.
func (s *Store) CreateTag(ctx context.Context, t *domain.Tag) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Check if slug already exists for this user.
		slugKey := []byte(fmt.Sprintf("%s%s:%s", tagBySlugPrefix, t.OwnerID, t.Slug))
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

		// Owner index.
		ownerKey := []byte(fmt.Sprintf("%s%s:%s", tagByOwnerPrefix, t.OwnerID, t.ID))
		if err := txn.Set(ownerKey, []byte{}); err != nil {
			return err
		}

		// Slug index (per owner).
		return txn.Set(slugKey, []byte(t.ID))
	})
}

// GetTag retrieves a tag by ID.
func (s *Store) GetTag(ctx context.Context, tagID string) (*domain.Tag, error) {
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

	// Treat soft-deleted as not found.
	if t.IsDeleted() {
		return nil, ErrTagNotFound
	}

	return &t, nil
}

// GetOrCreateTagByName finds or creates a tag for a user.
func (s *Store) GetOrCreateTagByName(ctx context.Context, userID, name string) (*domain.Tag, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slug := genre.Slugify(name)
	slugKey := []byte(fmt.Sprintf("%s%s:%s", tagBySlugPrefix, userID, slug))

	var tagID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(slugKey)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			tagID = string(val)
			return nil
		})
	})

	if err == nil {
		return s.GetTag(ctx, tagID)
	}
	if !errors.Is(err, badger.ErrKeyNotFound) {
		return nil, err
	}

	// Create new tag.
	tagID, err = id.Generate("tag")
	if err != nil {
		return nil, err
	}

	t := &domain.Tag{
		Syncable: domain.Syncable{ID: tagID},
		Name:     name,
		Slug:     slug,
		OwnerID:  userID,
	}
	t.InitTimestamps()

	if err := s.CreateTag(ctx, t); err != nil {
		return nil, err
	}

	return t, nil
}

// UpdateTag updates a tag.
func (s *Store) UpdateTag(ctx context.Context, t *domain.Tag) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get old tag for index updates.
	old, err := s.GetTag(ctx, t.ID)
	if err != nil {
		return err
	}

	t.Touch()

	return s.db.Update(func(txn *badger.Txn) error {
		// Update main record.
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		key := []byte(tagPrefix + t.ID)
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update slug index if changed.
		if old.Slug != t.Slug {
			// Remove old.
			oldSlugKey := []byte(fmt.Sprintf("%s%s:%s", tagBySlugPrefix, old.OwnerID, old.Slug))
			if err := txn.Delete(oldSlugKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			// Add new.
			newSlugKey := []byte(fmt.Sprintf("%s%s:%s", tagBySlugPrefix, t.OwnerID, t.Slug))
			if err := txn.Set(newSlugKey, []byte(t.ID)); err != nil {
				return err
			}
		}

		return nil
	})
}

// ListTagsForUser returns all tags owned by a user.
func (s *Store) ListTagsForUser(ctx context.Context, userID string) ([]*domain.Tag, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s%s:", tagByOwnerPrefix, userID)
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
		t, err := s.GetTag(ctx, tagID)
		if err != nil {
			continue
		}
		tags = append(tags, t)
	}

	return tags, nil
}

// DeleteTag soft-deletes a tag.
func (s *Store) DeleteTag(ctx context.Context, tagID string) error {
	t, err := s.GetTag(ctx, tagID)
	if err != nil {
		return err
	}

	t.MarkDeleted()

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		key := []byte(tagPrefix + tagID)
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Remove from indexes.
		ownerKey := []byte(fmt.Sprintf("%s%s:%s", tagByOwnerPrefix, t.OwnerID, t.ID))
		if err := txn.Delete(ownerKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		slugKey := []byte(fmt.Sprintf("%s%s:%s", tagBySlugPrefix, t.OwnerID, t.Slug))
		return txn.Delete(slugKey)
	})
}

// --- Book-Tag Association ---

// AddBookTag tags a book for a user.
func (s *Store) AddBookTag(ctx context.Context, bt *domain.BookTag) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// book -> tag index (user-scoped).
		btKey := []byte(fmt.Sprintf("%s%s:%s:%s", bookTagPrefix, bt.BookID, bt.UserID, bt.TagID))
		if err := txn.Set(btKey, []byte{}); err != nil {
			return err
		}

		// tag -> book index.
		tbKey := []byte(fmt.Sprintf("%s%s:%s", tagBookPrefix, bt.TagID, bt.BookID))
		return txn.Set(tbKey, []byte{})
	})
}

// RemoveBookTag removes a tag from a book for a user.
func (s *Store) RemoveBookTag(ctx context.Context, bookID, userID, tagID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		btKey := []byte(fmt.Sprintf("%s%s:%s:%s", bookTagPrefix, bookID, userID, tagID))
		if err := txn.Delete(btKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		tbKey := []byte(fmt.Sprintf("%s%s:%s", tagBookPrefix, tagID, bookID))
		if err := txn.Delete(tbKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		return nil
	})
}

// GetTagIDsForBook returns tag IDs for a book (for a specific user).
func (s *Store) GetTagIDsForBook(ctx context.Context, bookID, userID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s%s:%s:", bookTagPrefix, bookID, userID)
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

// GetBookIDsForTag returns book IDs with a specific tag.
func (s *Store) GetBookIDsForTag(ctx context.Context, tagID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s%s:", tagBookPrefix, tagID)
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

// SetBookTags sets all tags for a book by a user (replaces existing).
func (s *Store) SetBookTags(ctx context.Context, bookID, userID string, tagIDs []string) error {
	// Get current tags.
	currentIDs, err := s.GetTagIDsForBook(ctx, bookID, userID)
	if err != nil {
		return err
	}

	// Build sets.
	currentSet := make(map[string]bool)
	for _, tid := range currentIDs {
		currentSet[tid] = true
	}

	newSet := make(map[string]bool)
	for _, tid := range tagIDs {
		newSet[tid] = true
	}

	// Remove old.
	for _, tid := range currentIDs {
		if !newSet[tid] {
			if err := s.RemoveBookTag(ctx, bookID, userID, tid); err != nil {
				return err
			}
		}
	}

	// Add new.
	for _, tid := range tagIDs {
		if !currentSet[tid] {
			bt := &domain.BookTag{
				BookID:    bookID,
				TagID:     tid,
				UserID:    userID,
				CreatedAt: time.Now().UnixMilli(),
			}
			if err := s.AddBookTag(ctx, bt); err != nil {
				return err
			}
		}
	}

	return nil
}
