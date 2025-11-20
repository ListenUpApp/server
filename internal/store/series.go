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
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/sse"
)

const (
	seriesPrefix            = "series:"
	seriesByNamePrefix      = "idx:series:name:"       // For deduplication
	seriesByUpdatedAtPrefix = "idx:series:updated_at:" // Format: idx:series:updated_at:{RFC3339Nano}:series:{uuid}
)

var (
	// ErrSeriesNotFound is returned when a series is not found in the store.
	ErrSeriesNotFound = errors.New("series not found")
	// ErrSeriesExists is returned when attempting to create a series that already exists.
	ErrSeriesExists = errors.New("series already exists")
)

// CreateSeries creates a new series.
func (s *Store) CreateSeries(ctx context.Context, series *domain.Series) error {
	key := []byte(seriesPrefix + series.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check series exists: %w", err)
	}
	if exists {
		return ErrSeriesExists
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Save series
		data, err := json.Marshal(series)
		if err != nil {
			return fmt.Errorf("marshal series: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create name index for deduplication
		nameKey := []byte(seriesByNamePrefix + normalizeSeriesName(series.Name))
		if err := txn.Set(nameKey, []byte(series.ID)); err != nil {
			return err
		}

		// Create updated_at index for sync support
		updatedAtKey := formatTimestampIndexKey(seriesByUpdatedAtPrefix, series.UpdatedAt, "series", series.ID)
		if err := txn.Set(updatedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// GetSeries retrieves a series by ID.
func (s *Store) GetSeries(ctx context.Context, id string) (*domain.Series, error) {
	key := []byte(seriesPrefix + id)

	var series domain.Series
	if err := s.get(key, &series); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrSeriesNotFound
		}
		return nil, fmt.Errorf("get series: %w", err)
	}

	// Treat soft-deleted series as not found
	if series.IsDeleted() {
		return nil, ErrSeriesNotFound
	}

	// Populate total books count from reverse index
	count, err := s.CountBooksInSeries(ctx, id)
	if err != nil {
		// Log but don't fail - TotalBooks will be 0
		if s.logger != nil {
			s.logger.Warn("failed to count books in series", "series_id", id, "error", err)
		}
	}
	series.TotalBooks = count

	return &series, nil
}

// UpdateSeries updates an existing series.
func (s *Store) UpdateSeries(ctx context.Context, series *domain.Series) error {
	key := []byte(seriesPrefix + series.ID)

	// Get old series for index updates
	oldSeries, err := s.GetSeries(ctx, series.ID)
	if err != nil {
		return err
	}

	series.Touch()

	err = s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(series)
		if err != nil {
			return fmt.Errorf("marshal series: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update name index if name changed
		if oldSeries.Name != series.Name {
			// Delete old name index
			oldNameKey := []byte(seriesByNamePrefix + normalizeSeriesName(oldSeries.Name))
			if err := txn.Delete(oldNameKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			// Create new name index
			newNameKey := []byte(seriesByNamePrefix + normalizeSeriesName(series.Name))
			if err := txn.Set(newNameKey, []byte(series.ID)); err != nil {
				return err
			}
		}

		// Update updated_at index
		oldUpdatedAtKey := formatTimestampIndexKey(seriesByUpdatedAtPrefix, oldSeries.UpdatedAt, "series", series.ID)
		if err := txn.Delete(oldUpdatedAtKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		newUpdatedAtKey := formatTimestampIndexKey(seriesByUpdatedAtPrefix, series.UpdatedAt, "series", series.ID)
		if err := txn.Set(newUpdatedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	s.eventEmitter.Emit(sse.NewSeriesUpdatedEvent(series))

	// Cascade update to all books in this series
	if err := domain.CascadeSeriesUpdate(ctx, s, series.ID); err != nil {
		// Log but don't fail the update
		if s.logger != nil {
			s.logger.Error("cascade update failed", "series_id", series.ID, "error", err)
		}
	}

	return nil
}

// GetOrCreateSeriesByName finds or creates a series by name.
func (s *Store) GetOrCreateSeriesByName(ctx context.Context, name string) (*domain.Series, error) {
	normalized := normalizeSeriesName(name)
	nameKey := []byte(seriesByNamePrefix + normalized)

	// Try to find existing series
	var seriesID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(nameKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			seriesID = string(val)
			return nil
		})
	})

	if err == nil {
		// Found existing series
		return s.GetSeries(ctx, seriesID)
	}

	if !errors.Is(err, badger.ErrKeyNotFound) {
		return nil, fmt.Errorf("lookup series by name: %w", err)
	}

	// Create new series
	seriesID, err = id.Generate("series")
	if err != nil {
		return nil, fmt.Errorf("generate series ID: %w", err)
	}

	series := &domain.Series{
		Syncable: domain.Syncable{
			ID: seriesID,
		},
		Name:       name,
		TotalBooks: 0, // Unknown total by default
	}
	series.InitTimestamps()

	if err := s.CreateSeries(ctx, series); err != nil {
		return nil, fmt.Errorf("create series: %w", err)
	}

	s.eventEmitter.Emit(sse.NewSeriesCreatedEvent(series))

	return series, nil
}

// ListSeries returns paginated series.
func (s *Store) ListSeries(ctx context.Context, params PaginationParams) (*PaginatedResult[*domain.Series], error) {
	params.Validate()

	var seriesList []*domain.Series
	var lastKey string
	var hasMore bool

	prefix := []byte(seriesPrefix)

	// Decode cursor to get starting key
	startKey, err := DecodeCursor(params.Cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchSize = params.Limit + 1

		it := txn.NewIterator(opts)
		defer it.Close()

		// Start from cursor or beginning
		var seekKey []byte
		if startKey != "" {
			seekKey = []byte(startKey)
			it.Seek(seekKey)
			// Skip the cursor key itself
			if it.Valid() && string(it.Item().Key()) == startKey {
				it.Next()
			}
		} else {
			seekKey = prefix
			it.Seek(seekKey)
		}

		// Collect items up to limit (excluding deleted series)
		count := 0
		for it.ValidForPrefix(prefix) {
			item := it.Item()
			key := string(item.Key())

			// If we've collected enough items, check if there are more non-deleted series
			if count == params.Limit {
				// Check if there's at least one more non-deleted series
				for it.ValidForPrefix(prefix) {
					var checkSeries domain.Series
					err := it.Item().Value(func(val []byte) error {
						return json.Unmarshal(val, &checkSeries)
					})
					if err != nil {
						it.Next()
						continue
					}
					if !checkSeries.IsDeleted() {
						hasMore = true
						break
					}
					it.Next()
				}
				break
			}

			err := item.Value(func(val []byte) error {
				var series domain.Series
				if err := json.Unmarshal(val, &series); err != nil {
					return err
				}

				// Skip deleted series
				if series.IsDeleted() {
					return nil
				}

				seriesList = append(seriesList, &series)
				lastKey = key
				count++
				return nil
			})
			if err != nil {
				return err
			}
			it.Next()
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}

	// Populate total books count for all series in a single transaction
	if len(seriesList) > 0 {
		seriesIDs := make([]string, len(seriesList))
		for i, series := range seriesList {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			seriesIDs[i] = series.ID
		}

		counts, err := s.CountBooksForMultipleSeries(ctx, seriesIDs)
		if err != nil {
			// Log but don't fail - TotalBooks will be 0 for all
			if s.logger != nil {
				s.logger.Warn("failed to batch count books for series", "error", err)
			}
		} else {
			// Populate counts from batch result
			for _, series := range seriesList {
				// Check for context cancellation
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}
				series.TotalBooks = counts[series.ID]
			}
		}
	}

	// Create result
	result := &PaginatedResult[*domain.Series]{
		Items:   seriesList,
		HasMore: hasMore,
	}

	// Set next cursor if there are more results
	if hasMore && lastKey != "" {
		if len(seriesList) > 0 {
			result.NextCursor = EncodeCursor(seriesPrefix + seriesList[len(seriesList)-1].ID)
		}
	}

	return result, nil
}

// GetSeriesUpdatedAfter efficiently queries series with UpdatedAt > timestamp.
// This is used for delta sync.
func (s *Store) GetSeriesUpdatedAfter(ctx context.Context, timestamp time.Time) ([]*domain.Series, error) {
	var seriesList []*domain.Series

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We'll fetch values separately

		it := txn.NewIterator(opts)
		defer it.Close()

		// Seek to the timestamp
		seekKey := formatTimestampIndexKey(seriesByUpdatedAtPrefix, timestamp, "", "")
		prefix := []byte(seriesByUpdatedAtPrefix)

		it.Seek(seekKey)
		for it.ValidForPrefix(prefix) {
			key := it.Item().Key()

			entityType, entityID, err := parseTimestampIndexKey(key, seriesByUpdatedAtPrefix)
			if err != nil {
				it.Next()
				continue
			}

			if entityType == "series" {
				// Fetch the actual series
				seriesKey := []byte(seriesPrefix + entityID)
				item, err := txn.Get(seriesKey)
				if err != nil {
					it.Next()
					continue
				}

				var series domain.Series
				err = item.Value(func(val []byte) error {
					return json.Unmarshal(val, &series)
				})
				if err != nil {
					it.Next()
					continue
				}

				seriesList = append(seriesList, &series)
			}
			it.Next()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("query series by updated_at: %w", err)
	}

	// Populate total books count for all series in a single transaction
	if len(seriesList) > 0 {
		seriesIDs := make([]string, len(seriesList))
		for i, series := range seriesList {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			seriesIDs[i] = series.ID
		}

		counts, err := s.CountBooksForMultipleSeries(ctx, seriesIDs)
		if err != nil {
			// Log but don't fail - TotalBooks will be 0 for all
			if s.logger != nil {
				s.logger.Warn("failed to batch count books for series", "error", err)
			}
		} else {
			// Populate counts from batch result
			for _, series := range seriesList {
				// Check for context cancellation
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}
				series.TotalBooks = counts[series.ID]
			}
		}
	}

	return seriesList, nil
}

// GetBooksBySeries returns all books in a specific series, sorted by sequence.
func (s *Store) GetBooksBySeries(ctx context.Context, seriesID string) ([]*domain.Book, error) {
	var bookIDs []string

	// Use reverse index for O(1) lookup
	prefix := []byte(bookBySeriesPrefix + seriesID + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		it.Seek(prefix)
		for it.ValidForPrefix(prefix) {
			key := it.Item().Key()
			// Extract book ID from key: idx:books:series:{seriesID}:{bookID}
			parts := strings.Split(string(key), ":")
			if len(parts) >= 5 {
				bookID := parts[4]
				bookIDs = append(bookIDs, bookID)
			}
			it.Next()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lookup books by series: %w", err)
	}

	// Fetch actual books
	books := make([]*domain.Book, 0, len(bookIDs))
	for _, bookID := range bookIDs {
		book, err := s.getBookInternal(ctx, bookID)
		if err != nil {
			if errors.Is(err, ErrBookNotFound) {
				continue // Skip missing books
			}
			return nil, err
		}
		books = append(books, book)
	}

	// TODO: Sort books by sequence (needs natural sort for "1", "1.5", "2", etc.)
	// For now, return in database order

	return books, nil
}

// GetBookIDsBySeries returns just the book IDs for a series without loading full book data.
// This is optimized for cascade operations that only need IDs.
func (s *Store) GetBookIDsBySeries(_ context.Context, seriesID string) ([]string, error) {
	var bookIDs []string
	prefix := []byte(bookBySeriesPrefix + seriesID + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys, not values

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			// Extract book ID from key: idx:books:series:{seriesID}:{bookID}
			parts := strings.Split(string(key), ":")
			if len(parts) >= 5 {
				bookIDs = append(bookIDs, parts[4])
			}
		}
		return nil
	})

	return bookIDs, err
}

// CountBooksInSeries efficiently counts books in a series using the reverse index.
func (s *Store) CountBooksInSeries(_ context.Context, seriesID string) (int, error) {
	count := 0
	prefix := []byte(bookBySeriesPrefix + seriesID + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We only need to count keys, not load values

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	})

	return count, err
}

// CountBooksForMultipleSeries counts books for multiple series in a single database transaction.
// This avoids the N+1 query problem when listing many series.
func (s *Store) CountBooksForMultipleSeries(_ context.Context, seriesIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(seriesIDs))

	// Create a set for O(1) lookup and initialize counts
	seriesSet := make(map[string]struct{}, len(seriesIDs))
	for _, id := range seriesIDs {
		seriesSet[id] = struct{}{}
		counts[id] = 0 // Initialize to 0
	}

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys, not values
		opts.Prefix = []byte(bookBySeriesPrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(opts.Prefix); it.ValidForPrefix(opts.Prefix); it.Next() {
			key := it.Item().Key()
			// Extract series ID from key: idx:books:series:{seriesID}:{bookID}
			parts := strings.Split(string(key), ":")
			if len(parts) >= 4 {
				seriesID := parts[3]
				if _, exists := seriesSet[seriesID]; exists {
					counts[seriesID]++
				}
			}
		}
		return nil
	})

	return counts, err
}

// touchSeries updates just the UpdatedAt timestamp for a series without rewriting all data.
func (s *Store) touchSeries(_ context.Context, id string) error {
	key := []byte(seriesPrefix + id)

	return s.db.Update(func(txn *badger.Txn) error {
		// Get existing series
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrSeriesNotFound
			}
			return err
		}

		var series domain.Series
		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &series)
		})
		if err != nil {
			return fmt.Errorf("unmarshal series: %w", err)
		}

		// Store old timestamp for index cleanup
		oldUpdatedAt := series.UpdatedAt

		// Update timestamp
		series.Touch()

		// Marshal and save
		data, err := json.Marshal(&series)
		if err != nil {
			return fmt.Errorf("marshal series: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update updated_at index
		oldUpdatedAtKey := formatTimestampIndexKey(seriesByUpdatedAtPrefix, oldUpdatedAt, "series", series.ID)
		if err := txn.Delete(oldUpdatedAtKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		newUpdatedAtKey := formatTimestampIndexKey(seriesByUpdatedAtPrefix, series.UpdatedAt, "series", series.ID)
		if err := txn.Set(newUpdatedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// normalizeSeriesName normalizes a series name for deduplication.
// Lowercase, trim whitespace, collapse multiple spaces.
func normalizeSeriesName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Collapse multiple spaces to single space
	parts := strings.Fields(name)
	return strings.Join(parts, " ")
}
