package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
)

const (
	metadataBookPrefix     = "metadata:book:"
	metadataChaptersPrefix = "metadata:chapters:"

	// Cache duration for metadata.
	metadataCacheDuration = 30 * 24 * time.Hour // 30 days
)

// CachedBook wraps fetched book metadata with cache info.
type CachedBook struct {
	Book      *audible.Book  `json:"book"`
	FetchedAt time.Time      `json:"fetched_at"`
	Region    audible.Region `json:"region"`
}

// CachedChapters wraps fetched chapter data with cache info.
type CachedChapters struct {
	Chapters  []audible.Chapter `json:"chapters"`
	FetchedAt time.Time         `json:"fetched_at"`
	Region    audible.Region    `json:"region"`
}

// GetCachedBook retrieves cached book metadata.
// Returns nil, nil if not found or expired.
func (s *Store) GetCachedBook(ctx context.Context, region audible.Region, asin string) (*CachedBook, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := []byte(fmt.Sprintf("%s%s:%s", metadataBookPrefix, region, asin))

	var cached CachedBook
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &cached)
		})
	})

	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cached book: %w", err)
	}

	// Check if expired
	if time.Since(cached.FetchedAt) > metadataCacheDuration {
		return nil, nil // Treat as cache miss
	}

	return &cached, nil
}

// SetCachedBook stores book metadata in cache.
func (s *Store) SetCachedBook(ctx context.Context, region audible.Region, asin string, book *audible.Book) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cached := CachedBook{
		Book:      book,
		FetchedAt: time.Now(),
		Region:    region,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("marshal cached book: %w", err)
	}

	key := []byte(fmt.Sprintf("%s%s:%s", metadataBookPrefix, region, asin))

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// DeleteCachedBook removes cached book metadata.
func (s *Store) DeleteCachedBook(ctx context.Context, region audible.Region, asin string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := []byte(fmt.Sprintf("%s%s:%s", metadataBookPrefix, region, asin))

	return s.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil // Idempotent
		}
		return err
	})
}

// GetCachedChapters retrieves cached chapter data.
// Returns nil, nil if not found or expired.
func (s *Store) GetCachedChapters(ctx context.Context, region audible.Region, asin string) (*CachedChapters, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := []byte(fmt.Sprintf("%s%s:%s", metadataChaptersPrefix, region, asin))

	var cached CachedChapters
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &cached)
		})
	})

	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cached chapters: %w", err)
	}

	// Check if expired
	if time.Since(cached.FetchedAt) > metadataCacheDuration {
		return nil, nil
	}

	return &cached, nil
}

// SetCachedChapters stores chapter data in cache.
func (s *Store) SetCachedChapters(ctx context.Context, region audible.Region, asin string, chapters []audible.Chapter) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cached := CachedChapters{
		Chapters:  chapters,
		FetchedAt: time.Now(),
		Region:    region,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("marshal cached chapters: %w", err)
	}

	key := []byte(fmt.Sprintf("%s%s:%s", metadataChaptersPrefix, region, asin))

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// DeleteCachedChapters removes cached chapter data.
func (s *Store) DeleteCachedChapters(ctx context.Context, region audible.Region, asin string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := []byte(fmt.Sprintf("%s%s:%s", metadataChaptersPrefix, region, asin))

	return s.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	})
}
