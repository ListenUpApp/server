package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json/v2"
	"time"

	"github.com/listenupapp/listenup-server/internal/metadata/audible"
	"github.com/listenupapp/listenup-server/internal/store"
)

// Differentiated cache durations.
const (
	searchCacheDuration  = 24 * time.Hour      // High volume, changes often
	bookCacheDuration    = 7 * 24 * time.Hour  // Moderate, stable
	chapterCacheDuration = 30 * 24 * time.Hour // Rarely changes
)

// GetCachedBook retrieves cached book metadata.
// Returns nil, nil if not found or expired.
func (s *Store) GetCachedBook(ctx context.Context, region audible.Region, asin string) (*store.CachedBook, error) {
	var (
		data      string
		fetchedAt string
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT data, fetched_at FROM audible_cache_books WHERE region = ? AND asin = ?`,
		string(region), asin).Scan(&data, &fetchedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	fetchedTime, err := parseTime(fetchedAt)
	if err != nil {
		return nil, err
	}

	// Check if expired.
	if time.Since(fetchedTime) > bookCacheDuration {
		return nil, nil // Treat as cache miss
	}

	var book audible.Book
	if err := json.Unmarshal([]byte(data), &book); err != nil {
		return nil, err
	}

	return &store.CachedBook{
		Book:      &book,
		FetchedAt: fetchedTime,
		Region:    region,
	}, nil
}

// SetCachedBook stores book metadata in cache.
func (s *Store) SetCachedBook(ctx context.Context, region audible.Region, asin string, book *audible.Book) error {
	data, err := json.Marshal(book)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO audible_cache_books (region, asin, data, fetched_at) VALUES (?, ?, ?, ?)`,
		string(region), asin, string(data), formatTime(time.Now().UTC()))
	return err
}

// DeleteCachedBook removes cached book metadata.
// This operation is idempotent.
func (s *Store) DeleteCachedBook(ctx context.Context, region audible.Region, asin string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM audible_cache_books WHERE region = ? AND asin = ?`,
		string(region), asin)
	return err
}

// GetCachedChapters retrieves cached chapter data.
// Returns nil, nil if not found or expired.
func (s *Store) GetCachedChapters(ctx context.Context, region audible.Region, asin string) (*store.CachedChapters, error) {
	var (
		data      string
		fetchedAt string
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT data, fetched_at FROM audible_cache_chapters WHERE region = ? AND asin = ?`,
		string(region), asin).Scan(&data, &fetchedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	fetchedTime, err := parseTime(fetchedAt)
	if err != nil {
		return nil, err
	}

	// Check if expired.
	if time.Since(fetchedTime) > chapterCacheDuration {
		return nil, nil
	}

	var chapters []audible.Chapter
	if err := json.Unmarshal([]byte(data), &chapters); err != nil {
		return nil, err
	}

	return &store.CachedChapters{
		Chapters:  chapters,
		FetchedAt: fetchedTime,
		Region:    region,
	}, nil
}

// SetCachedChapters stores chapter data in cache.
func (s *Store) SetCachedChapters(ctx context.Context, region audible.Region, asin string, chapters []audible.Chapter) error {
	data, err := json.Marshal(chapters)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO audible_cache_chapters (region, asin, data, fetched_at) VALUES (?, ?, ?, ?)`,
		string(region), asin, string(data), formatTime(time.Now().UTC()))
	return err
}

// DeleteCachedChapters removes cached chapter data.
// This operation is idempotent.
func (s *Store) DeleteCachedChapters(ctx context.Context, region audible.Region, asin string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM audible_cache_chapters WHERE region = ? AND asin = ?`,
		string(region), asin)
	return err
}

// searchCacheQueryKey generates a stable key for search results.
// Uses hash to handle long query strings.
func searchCacheQueryKey(query string) string {
	hash := sha256.Sum256([]byte(query))
	return hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars
}

// GetCachedSearch retrieves cached search results.
// Returns nil, nil if not found or expired.
func (s *Store) GetCachedSearch(ctx context.Context, region audible.Region, query string) (*store.CachedSearch, error) {
	queryKey := searchCacheQueryKey(query)

	var (
		data      string
		fetchedAt string
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT data, fetched_at FROM audible_cache_search WHERE region = ? AND query = ?`,
		string(region), queryKey).Scan(&data, &fetchedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	fetchedTime, err := parseTime(fetchedAt)
	if err != nil {
		return nil, err
	}

	// Check if expired.
	if time.Since(fetchedTime) > searchCacheDuration {
		return nil, nil
	}

	var results []audible.SearchResult
	if err := json.Unmarshal([]byte(data), &results); err != nil {
		return nil, err
	}

	return &store.CachedSearch{
		Results:   results,
		FetchedAt: fetchedTime,
		Region:    region,
		Query:     query,
	}, nil
}

// SetCachedSearch stores search results in cache.
func (s *Store) SetCachedSearch(ctx context.Context, region audible.Region, query string, results []audible.SearchResult) error {
	queryKey := searchCacheQueryKey(query)

	data, err := json.Marshal(results)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO audible_cache_search (region, query, data, fetched_at) VALUES (?, ?, ?, ?)`,
		string(region), queryKey, string(data), formatTime(time.Now().UTC()))
	return err
}

// DeleteCachedSearch removes cached search results.
// This operation is idempotent.
func (s *Store) DeleteCachedSearch(ctx context.Context, region audible.Region, query string) error {
	queryKey := searchCacheQueryKey(query)

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM audible_cache_search WHERE region = ? AND query = ?`,
		string(region), queryKey)
	return err
}
