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
	contributorPrefix            = "contributor:"
	contributorByNamePrefix      = "idx:contributors:name:"       // For deduplication
	contributorByUpdatedAtPrefix = "idx:contributors:updated_at:" // Format: idx:contributors:updated_at:{RFC3339Nano}:contributor:{uuid}
)

var (
	ErrContributorNotFound = errors.New("contributor not found")
	ErrContributorExists   = errors.New("contributor already exists")
)

// CreateContributor creates a new contributor.
func (s *Store) CreateContributor(ctx context.Context, contributor *domain.Contributor) error {
	key := []byte(contributorPrefix + contributor.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check contributor exists: %w", err)
	}
	if exists {
		return ErrContributorExists
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Save contributor
		data, err := json.Marshal(contributor)
		if err != nil {
			return fmt.Errorf("marshal contributor: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create name index for deduplication
		nameKey := []byte(contributorByNamePrefix + normalizeContributorName(contributor.Name))
		if err := txn.Set(nameKey, []byte(contributor.ID)); err != nil {
			return err
		}

		// Create updated_at index for sync support
		updatedAtKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, contributor.UpdatedAt, "contributor", contributor.ID)
		if err := txn.Set(updatedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// GetContributor retrieves a contributor by ID.
func (s *Store) GetContributor(_ context.Context, id string) (*domain.Contributor, error) {
	key := []byte(contributorPrefix + id)

	var contributor domain.Contributor
	if err := s.get(key, &contributor); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrContributorNotFound
		}
		return nil, fmt.Errorf("get contributor: %w", err)
	}

	// Treat soft-deleted contributors as not found
	if contributor.IsDeleted() {
		return nil, ErrContributorNotFound
	}

	return &contributor, nil
}

// GetContributorsByIDs retrieves multiple contributors by their IDs.
func (s *Store) GetContributorsByIDs(ctx context.Context, ids []string) ([]*domain.Contributor, error) {
	contributors := make([]*domain.Contributor, 0, len(ids))

	for _, id := range ids {
		contributor, err := s.GetContributor(ctx, id)
		if err != nil {
			if errors.Is(err, ErrContributorNotFound) {
				continue // Skip missing contributors
			}
			return nil, err
		}
		contributors = append(contributors, contributor)
	}

	return contributors, nil
}

// UpdateContributor updates an existing contributor.
func (s *Store) UpdateContributor(ctx context.Context, contributor *domain.Contributor) error {
	key := []byte(contributorPrefix + contributor.ID)

	// Get old contributor for index updates
	oldContributor, err := s.GetContributor(ctx, contributor.ID)
	if err != nil {
		return err
	}

	contributor.Touch()

	err = s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(contributor)
		if err != nil {
			return fmt.Errorf("marshal contributor: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update name index if name changed
		if oldContributor.Name != contributor.Name {
			// Delete old name index
			oldNameKey := []byte(contributorByNamePrefix + normalizeContributorName(oldContributor.Name))
			if err := txn.Delete(oldNameKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			// Create new name index
			newNameKey := []byte(contributorByNamePrefix + normalizeContributorName(contributor.Name))
			if err := txn.Set(newNameKey, []byte(contributor.ID)); err != nil {
				return err
			}
		}

		// Update updated_at index
		oldUpdatedAtKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, oldContributor.UpdatedAt, "contributor", contributor.ID)
		if err := txn.Delete(oldUpdatedAtKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		newUpdatedAtKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, contributor.UpdatedAt, "contributor", contributor.ID)
		if err := txn.Set(newUpdatedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	s.eventEmitter.Emit(sse.NewContributorUpdatedEvent(contributor))
	return nil
}

// GetOrCreateContributorByName finds or creates a contributor by name.
// No power without control - one entity per person, no matter how metadata spells it.
func (s *Store) GetOrCreateContributorByName(ctx context.Context, name string) (*domain.Contributor, error) {
	// Ka is a wheel, and names are its spokes.
	normalized := normalizeContributorName(name)
	nameKey := []byte(contributorByNamePrefix + normalized)

	// Try to find existing contributor
	var contributorID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(nameKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			contributorID = string(val)
			return nil
		})
	})

	if err == nil {
		// Found existing contributor
		return s.GetContributor(ctx, contributorID)
	}

	if !errors.Is(err, badger.ErrKeyNotFound) {
		return nil, fmt.Errorf("lookup contributor by name: %w", err)
	}

	// Create new contributor
	contributorID, err = id.Generate("contributor")
	if err != nil {
		return nil, fmt.Errorf("generate contributor ID: %w", err)
	}

	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name: name,
	}
	contributor.InitTimestamps()

	if err := s.CreateContributor(ctx, contributor); err != nil {
		return nil, fmt.Errorf("create contributor: %w", err)
	}

	s.eventEmitter.Emit(sse.NewContributorCreatedEvent(contributor))

	return contributor, nil
}

// ListContributors returns paginated contributors.
func (s *Store) ListContributors(ctx context.Context, params PaginationParams) (*PaginatedResult[*domain.Contributor], error) {
	params.Validate()

	var contributors []*domain.Contributor
	var lastKey string
	var hasMore bool

	prefix := []byte(contributorPrefix)

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

		// Collect items up to limit (excluding deleted contributors)
		count := 0
		for it.ValidForPrefix(prefix) {
			item := it.Item()
			key := string(item.Key())

			// If we've collected enough items, check if there are more non-deleted contributors
			if count == params.Limit {
				// Check if there's at least one more non-deleted contributor
				for it.ValidForPrefix(prefix) {
					var checkContributor domain.Contributor
					err := it.Item().Value(func(val []byte) error {
						return json.Unmarshal(val, &checkContributor)
					})
					if err != nil {
						it.Next()
						continue
					}
					if !checkContributor.IsDeleted() {
						hasMore = true
						break
					}
					it.Next()
				}
				break
			}

			err := item.Value(func(val []byte) error {
				var contributor domain.Contributor
				if err := json.Unmarshal(val, &contributor); err != nil {
					return err
				}

				// Skip deleted contributors
				if contributor.IsDeleted() {
					return nil
				}

				contributors = append(contributors, &contributor)
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
		return nil, fmt.Errorf("list contributors: %w", err)
	}

	// Create result
	result := &PaginatedResult[*domain.Contributor]{
		Items:   contributors,
		HasMore: hasMore,
	}

	// Set next cursor if there are more results
	if hasMore && lastKey != "" {
		if len(contributors) > 0 {
			result.NextCursor = EncodeCursor(contributorPrefix + contributors[len(contributors)-1].ID)
		}
	}

	return result, nil
}

// GetContributorsUpdatedAfter efficiently queries contributors with UpdatedAt > timestamp.
// This is used for delta sync.
func (s *Store) GetContributorsUpdatedAfter(_ context.Context, timestamp time.Time) ([]*domain.Contributor, error) {
	var contributors []*domain.Contributor

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // We'll fetch values separately

		it := txn.NewIterator(opts)
		defer it.Close()

		// Seek to the timestamp
		seekKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, timestamp, "", "")
		prefix := []byte(contributorByUpdatedAtPrefix)

		it.Seek(seekKey)
		for it.ValidForPrefix(prefix) {
			key := it.Item().Key()

			entityType, entityID, err := parseTimestampIndexKey(key, contributorByUpdatedAtPrefix)
			if err != nil {
				it.Next()
				continue
			}

			if entityType == "contributor" {
				// Fetch the actual contributor
				contributorKey := []byte(contributorPrefix + entityID)
				item, err := txn.Get(contributorKey)
				if err != nil {
					it.Next()
					continue
				}

				var contributor domain.Contributor
				err = item.Value(func(val []byte) error {
					return json.Unmarshal(val, &contributor)
				})
				if err != nil {
					it.Next()
					continue
				}

				contributors = append(contributors, &contributor)
			}
			it.Next()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("query contributors by updated_at: %w", err)
	}

	return contributors, nil
}

// GetBooksByContributor returns all books with a specific contributor (any role).
// "You can't take the sky from me" - nor can you take the books from contributors.
func (s *Store) GetBooksByContributor(ctx context.Context, contributorID string) ([]*domain.Book, error) {
	var bookIDs []string

	// Use reverse index for O(1) lookup
	prefix := []byte(bookByContributorPrefix + contributorID + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		it.Seek(prefix)
		for it.ValidForPrefix(prefix) {
			key := it.Item().Key()
			// Extract book ID from key: idx:books:contributor:{contributorID}:{bookID}
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
		return nil, fmt.Errorf("lookup books by contributor: %w", err)
	}

	// Fetch actual books
	books := make([]*domain.Book, 0, len(bookIDs))
	for _, bookID := range bookIDs {
		book, err := s.GetBook(ctx, bookID)
		if err != nil {
			if errors.Is(err, ErrBookNotFound) {
				continue // Skip missing books
			}
			return nil, err
		}
		books = append(books, book)
	}

	return books, nil
}

// GetBooksByContributorRole returns all books with a contributor in a specific role.
func (s *Store) GetBooksByContributorRole(ctx context.Context, contributorID string, role domain.ContributorRole) ([]*domain.Book, error) {
	// Get all books for this contributor
	allBooks, err := s.GetBooksByContributor(ctx, contributorID)
	if err != nil {
		return nil, err
	}

	// Filter by role
	var books []*domain.Book
	for _, book := range allBooks {
		for _, bc := range book.Contributors {
			if bc.ContributorID == contributorID {
				for _, r := range bc.Roles {
					if r == role {
						books = append(books, book)
						break
					}
				}
			}
		}
	}

	return books, nil
}

// touchContributor updates just the UpdatedAt timestamp for a contributor without rewriting all data.
func (s *Store) touchContributor(_ context.Context, id string) error {
	key := []byte(contributorPrefix + id)

	return s.db.Update(func(txn *badger.Txn) error {
		// Get existing contributor
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrContributorNotFound
			}
			return err
		}

		var contributor domain.Contributor
		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &contributor)
		})
		if err != nil {
			return fmt.Errorf("unmarshal contributor: %w", err)
		}

		// Store old timestamp for index cleanup
		oldUpdatedAt := contributor.UpdatedAt

		// Update timestamp
		contributor.Touch()

		// Marshal and save
		data, err := json.Marshal(&contributor)
		if err != nil {
			return fmt.Errorf("marshal contributor: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update updated_at index
		oldUpdatedAtKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, oldUpdatedAt, "contributor", contributor.ID)
		if err := txn.Delete(oldUpdatedAtKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		newUpdatedAtKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, contributor.UpdatedAt, "contributor", contributor.ID)
		if err := txn.Set(newUpdatedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// normalizeContributorName normalizes a name for deduplication.
// Lowercase, trim whitespace, collapse multiple spaces.
func normalizeContributorName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Collapse multiple spaces to single space
	parts := strings.Fields(name)
	return strings.Join(parts, " ")
}
