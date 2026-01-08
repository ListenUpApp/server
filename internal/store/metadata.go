package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	metadataSearchPrefix   = "metadata:search:"

	// Differentiated cache durations.
	searchCacheDuration  = 24 * time.Hour      // High volume, changes often
	bookCacheDuration    = 7 * 24 * time.Hour  // Moderate, stable
	chapterCacheDuration = 30 * 24 * time.Hour // Rarely changes
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

// CachedSearch wraps search results with cache info.
type CachedSearch struct {
	Results   []audible.SearchResult `json:"results"`
	FetchedAt time.Time              `json:"fetched_at"`
	Region    audible.Region         `json:"region"`
	Query     string                 `json:"query"`
}

// GetCachedBook retrieves cached book metadata.
// Returns nil, nil if not found or expired.
func (s *Store) GetCachedBook(ctx context.Context, region audible.Region, asin string) (*CachedBook, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := fmt.Appendf(nil, "%s%s:%s", metadataBookPrefix, region, asin)

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
	if time.Since(cached.FetchedAt) > bookCacheDuration {
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

	key := fmt.Appendf(nil, "%s%s:%s", metadataBookPrefix, region, asin)

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// DeleteCachedBook removes cached book metadata.
func (s *Store) DeleteCachedBook(ctx context.Context, region audible.Region, asin string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := fmt.Appendf(nil, "%s%s:%s", metadataBookPrefix, region, asin)

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

	key := fmt.Appendf(nil, "%s%s:%s", metadataChaptersPrefix, region, asin)

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
	if time.Since(cached.FetchedAt) > chapterCacheDuration {
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

	key := fmt.Appendf(nil, "%s%s:%s", metadataChaptersPrefix, region, asin)

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// DeleteCachedChapters removes cached chapter data.
func (s *Store) DeleteCachedChapters(ctx context.Context, region audible.Region, asin string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := fmt.Appendf(nil, "%s%s:%s", metadataChaptersPrefix, region, asin)

	return s.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	})
}

// searchCacheKey generates a cache key for search results.
// Uses hash to handle long query strings.
func searchCacheKey(region audible.Region, query string) []byte {
	hash := sha256.Sum256([]byte(query))
	hashStr := hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars
	return fmt.Appendf(nil, "%s%s:%s", metadataSearchPrefix, region, hashStr)
}

// GetCachedSearch retrieves cached search results.
// Returns nil, nil if not found or expired.
func (s *Store) GetCachedSearch(ctx context.Context, region audible.Region, query string) (*CachedSearch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := searchCacheKey(region, query)

	var cached CachedSearch
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
		return nil, fmt.Errorf("get cached search: %w", err)
	}

	// Check if expired
	if time.Since(cached.FetchedAt) > searchCacheDuration {
		return nil, nil
	}

	return &cached, nil
}

// SetCachedSearch stores search results in cache.
func (s *Store) SetCachedSearch(ctx context.Context, region audible.Region, query string, results []audible.SearchResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cached := CachedSearch{
		Results:   results,
		FetchedAt: time.Now(),
		Region:    region,
		Query:     query,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("marshal cached search: %w", err)
	}

	key := searchCacheKey(region, query)

	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

// DeleteCachedSearch removes cached search results.
func (s *Store) DeleteCachedSearch(ctx context.Context, region audible.Region, query string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := searchCacheKey(region, query)

	return s.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	})
}
