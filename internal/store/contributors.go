// Package store provides data storage and retrieval operations using BadgerDB.
//
//nolint:dupl // Similar CRUD patterns for different entity types are acceptable
package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
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
	contributorByAliasPrefix     = "idx:contributors:alias:"      // For alias lookup: idx:contributors:alias:{normalized}:{contributorID}
	contributorByASINPrefix      = "idx:contributors:asin:"       // For Audible deduplication
	contributorByUpdatedAtPrefix = "idx:contributors:updated_at:" // Format: idx:contributors:updated_at:{RFC3339Nano}:contributor:{uuid}
	contributorByDeletedAtPrefix = "idx:contributors:deleted_at:" // Format: idx:contributors:deleted_at:{RFC3339Nano}:contributor:{uuid}
)

var (
	// ErrContributorNotFound is returned when a contributor cannot be found.
	ErrContributorNotFound = errors.New("contributor not found")
	// ErrContributorExists is returned when attempting to create a contributor that already exists.
	ErrContributorExists = errors.New("contributor already exists")
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

	err = s.db.Update(func(txn *badger.Txn) error {
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

		// Create alias indexes for each alias
		for _, alias := range contributor.Aliases {
			aliasKey := []byte(contributorByAliasPrefix + normalizeContributorName(alias))
			if err := txn.Set(aliasKey, []byte(contributor.ID)); err != nil {
				return err
			}
		}

		// Create ASIN index if present
		if contributor.ASIN != "" {
			asinKey := []byte(contributorByASINPrefix + contributor.ASIN)
			if err := txn.Set(asinKey, []byte(contributor.ID)); err != nil {
				return err
			}
		}

		// Create updated_at index for sync support
		updatedAtKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, contributor.UpdatedAt, "contributor", contributor.ID)
		if err := txn.Set(updatedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Index for search asynchronously
	if s.searchIndexer != nil {
		go func() {
			if err := s.searchIndexer.IndexContributor(context.Background(), contributor); err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to index contributor for search", "contributor_id", contributor.ID, "error", err)
				}
			}
		}()
	}

	return nil
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

// GetContributorByASIN retrieves a contributor by their Audible ASIN.
func (s *Store) GetContributorByASIN(ctx context.Context, asin string) (*domain.Contributor, error) {
	if asin == "" {
		return nil, ErrContributorNotFound
	}

	indexKey := []byte(contributorByASINPrefix + asin)

	var contributorID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(indexKey)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			contributorID = string(val)
			return nil
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrContributorNotFound
		}
		return nil, fmt.Errorf("lookup contributor by ASIN: %w", err)
	}

	return s.GetContributor(ctx, contributorID)
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

		// Update alias indexes if aliases changed
		oldAliasSet := make(map[string]bool)
		for _, alias := range oldContributor.Aliases {
			oldAliasSet[normalizeContributorName(alias)] = true
		}
		newAliasSet := make(map[string]bool)
		for _, alias := range contributor.Aliases {
			newAliasSet[normalizeContributorName(alias)] = true
		}

		// Delete removed aliases
		for oldAlias := range oldAliasSet {
			if !newAliasSet[oldAlias] {
				aliasKey := []byte(contributorByAliasPrefix + oldAlias)
				if err := txn.Delete(aliasKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
					return err
				}
			}
		}

		// Add new aliases
		for newAlias := range newAliasSet {
			if !oldAliasSet[newAlias] {
				aliasKey := []byte(contributorByAliasPrefix + newAlias)
				if err := txn.Set(aliasKey, []byte(contributor.ID)); err != nil {
					return err
				}
			}
		}

		// Update ASIN index if changed
		if oldContributor.ASIN != contributor.ASIN {
			// Delete old ASIN index
			if oldContributor.ASIN != "" {
				oldASINKey := []byte(contributorByASINPrefix + oldContributor.ASIN)
				if err := txn.Delete(oldASINKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
					return err
				}
			}
			// Create new ASIN index
			if contributor.ASIN != "" {
				newASINKey := []byte(contributorByASINPrefix + contributor.ASIN)
				if err := txn.Set(newASINKey, []byte(contributor.ID)); err != nil {
					return err
				}
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

	// Reindex for search asynchronously
	if s.searchIndexer != nil {
		go func() {
			if err := s.searchIndexer.IndexContributor(context.Background(), contributor); err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to reindex contributor for search", "contributor_id", contributor.ID, "error", err)
				}
			}
		}()
	}

	return nil
}

// GetOrCreateContributorByName finds or creates a contributor by name.
// No power without control - one entity per person, no matter how metadata spells it.
//
// Lookup order:
//  1. Check name index (exact match on normalized name)
//  2. Check alias index (pen names merged into canonical contributor)
//  3. Create new contributor if not found
//
// Returns the contributor and a boolean indicating if it was found by alias.
// When found by alias, the caller should set CreditedAs on the BookContributor.
func (s *Store) GetOrCreateContributorByName(ctx context.Context, name string) (*domain.Contributor, error) {
	contributor, _, err := s.GetOrCreateContributorByNameWithAlias(ctx, name)
	return contributor, err
}

// GetOrCreateContributorByNameWithAlias is like GetOrCreateContributorByName but also
// returns whether the contributor was found via an alias lookup.
// When foundByAlias is true, the original name should be preserved as CreditedAs.
func (s *Store) GetOrCreateContributorByNameWithAlias(ctx context.Context, name string) (contributor *domain.Contributor, foundByAlias bool, err error) {
	// Ka is a wheel, and names are its spokes.
	normalized := normalizeContributorName(name)
	nameKey := []byte(contributorByNamePrefix + normalized)
	aliasKey := []byte(contributorByAliasPrefix + normalized)

	// Try to find existing contributor by name first
	var contributorID string
	err = s.db.View(func(txn *badger.Txn) error {
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
		// Found by exact name match
		c, err := s.GetContributor(ctx, contributorID)
		return c, false, err
	}

	if !errors.Is(err, badger.ErrKeyNotFound) {
		return nil, false, fmt.Errorf("lookup contributor by name: %w", err)
	}

	// Not found by name - check alias index
	err = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(aliasKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			contributorID = string(val)
			return nil
		})
	})

	if err == nil {
		// Found by alias - this is a pen name
		c, err := s.GetContributor(ctx, contributorID)
		return c, true, err
	}

	if !errors.Is(err, badger.ErrKeyNotFound) {
		return nil, false, fmt.Errorf("lookup contributor by alias: %w", err)
	}

	// Create new contributor
	contributorID, err = id.Generate("contributor")
	if err != nil {
		return nil, false, fmt.Errorf("generate contributor ID: %w", err)
	}

	contributor = &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name: name,
	}
	contributor.InitTimestamps()

	if err := s.CreateContributor(ctx, contributor); err != nil {
		return nil, false, fmt.Errorf("create contributor: %w", err)
	}

	s.eventEmitter.Emit(sse.NewContributorCreatedEvent(contributor))

	return contributor, false, nil
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
		book, err := s.getBookInternal(ctx, bookID)
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
				if slices.Contains(bc.Roles, role) {
					books = append(books, book)
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

// DeleteContributor soft-deletes a contributor by setting DeletedAt.
// The contributor will no longer appear in normal queries but can be queried
// via GetContributorsDeletedAfter for sync purposes.
func (s *Store) DeleteContributor(ctx context.Context, id string) error {
	key := []byte(contributorPrefix + id)

	// Get existing contributor
	contributor, err := s.GetContributor(ctx, id)
	if err != nil {
		return err
	}

	// Mark as deleted
	contributor.MarkDeleted()

	err = s.db.Update(func(txn *badger.Txn) error {
		// Save updated contributor
		data, err := json.Marshal(contributor)
		if err != nil {
			return fmt.Errorf("marshal contributor: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Remove name index entry so lookups don't find deleted contributor
		normalizedName := normalizeContributorName(contributor.Name)
		nameKey := []byte(contributorByNamePrefix + normalizedName)
		if err := txn.Delete(nameKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			// Best effort cleanup
		}

		// Remove alias index entries
		for _, alias := range contributor.Aliases {
			aliasKey := []byte(contributorByAliasPrefix + normalizeContributorName(alias))
			if err := txn.Delete(aliasKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				// Best effort cleanup
			}
		}

		// Remove ASIN index entry
		if contributor.ASIN != "" {
			asinKey := []byte(contributorByASINPrefix + contributor.ASIN)
			if err := txn.Delete(asinKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				// Best effort cleanup
			}
		}

		// Update updated_at index (MarkDeleted also updates UpdatedAt)
		oldUpdatedAtKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, contributor.CreatedAt, "contributor", contributor.ID)
		if err := txn.Delete(oldUpdatedAtKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			// Best effort cleanup of old index
		}
		newUpdatedAtKey := formatTimestampIndexKey(contributorByUpdatedAtPrefix, contributor.UpdatedAt, "contributor", contributor.ID)
		if err := txn.Set(newUpdatedAtKey, []byte{}); err != nil {
			return err
		}

		// Create deleted_at index for sync
		deletedAtKey := formatTimestampIndexKey(contributorByDeletedAtPrefix, *contributor.DeletedAt, "contributor", contributor.ID)
		if err := txn.Set(deletedAtKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Remove from search index asynchronously
	if s.searchIndexer != nil {
		go func() {
			if err := s.searchIndexer.DeleteContributor(context.Background(), id); err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to remove contributor from search index", "contributor_id", id, "error", err)
				}
			}
		}()
	}

	return nil
}

// ListAllContributors returns all non-deleted contributors without pagination.
// WARNING: This can be expensive for large libraries. Use for bulk operations like reindexing.
func (s *Store) ListAllContributors(_ context.Context) ([]*domain.Contributor, error) {
	var contributors []*domain.Contributor

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(contributorPrefix)
		it.Seek(prefix)
		for it.ValidForPrefix(prefix) {
			var contributor domain.Contributor
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &contributor)
			})
			if err != nil {
				it.Next()
				continue
			}

			// Skip soft-deleted
			if !contributor.IsDeleted() {
				contributors = append(contributors, &contributor)
			}
			it.Next()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list all contributors: %w", err)
	}

	return contributors, nil
}

// CountBooksForContributor returns the number of books associated with a contributor.
// This is more efficient than GetBooksByContributor when only the count is needed.
func (s *Store) CountBooksForContributor(_ context.Context, contributorID string) (int, error) {
	var count int

	prefix := []byte(bookByContributorPrefix + contributorID + ":")

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		it.Seek(prefix)
		for it.ValidForPrefix(prefix) {
			count++
			it.Next()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("count books for contributor: %w", err)
	}

	return count, nil
}

// CountBooksForAllContributors returns book counts for all contributors in a single scan.
// Much more efficient than calling CountBooksForContributor N times during reindexing.
// Returns map[contributorID]bookCount.
func (s *Store) CountBooksForAllContributors(_ context.Context) (map[string]int, error) {
	counts := make(map[string]int)

	prefix := []byte(bookByContributorPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		// Key format: idx:books:contributor:{contributorID}:{bookID}
		prefixLen := len(bookByContributorPrefix)

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := string(it.Item().Key())
			// Extract contributorID from key
			rest := key[prefixLen:] // {contributorID}:{bookID}
			colonIdx := strings.Index(rest, ":")
			if colonIdx > 0 {
				contributorID := rest[:colonIdx]
				counts[contributorID]++
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("count books for all contributors: %w", err)
	}

	return counts, nil
}

// SearchContributorsByName performs a case-insensitive search for contributors by name.
// Returns contributors whose names contain the query string, limited to `limit` results.
// This is used for autocomplete in the contributor editing UI.
func (s *Store) SearchContributorsByName(_ context.Context, query string, limit int) ([]*domain.Contributor, error) {
	if limit <= 0 {
		limit = 10 // Default limit
	}

	// Normalize query for case-insensitive matching
	normalizedQuery := normalizeContributorName(query)
	if normalizedQuery == "" {
		return []*domain.Contributor{}, nil
	}

	var contributors []*domain.Contributor

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(contributorPrefix)
		it.Seek(prefix)
		for it.ValidForPrefix(prefix) {
			if len(contributors) >= limit {
				break
			}

			var contributor domain.Contributor
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &contributor)
			})
			if err != nil {
				it.Next()
				continue
			}

			// Skip soft-deleted contributors
			if contributor.IsDeleted() {
				it.Next()
				continue
			}

			// Check if normalized name contains the query
			normalizedName := normalizeContributorName(contributor.Name)
			if strings.Contains(normalizedName, normalizedQuery) {
				contributors = append(contributors, &contributor)
			}

			it.Next()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("search contributors by name: %w", err)
	}

	return contributors, nil
}

// GetContributorsDeletedAfter queries all contributors with DeletedAt > timestamp.
// This is used for delta sync to inform clients which contributors were deleted.
// Returns a list of contributor IDs that were soft-deleted after the given timestamp.
func (s *Store) GetContributorsDeletedAfter(_ context.Context, timestamp time.Time) ([]string, error) {
	var contributorIDs []string

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		// Seek to the timestamp
		seekKey := formatTimestampIndexKey(contributorByDeletedAtPrefix, timestamp, "", "")
		prefix := []byte(contributorByDeletedAtPrefix)

		it.Seek(seekKey)
		for it.ValidForPrefix(prefix) {
			key := it.Item().Key()

			entityType, entityID, err := parseTimestampIndexKey(key, contributorByDeletedAtPrefix)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to parse deleted_at key", "key", string(key), "error", err)
				}
				it.Next()
				continue
			}

			if entityType == "contributor" {
				contributorIDs = append(contributorIDs, entityID)
			}
			it.Next()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan deleted_at index: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("deleted contributors query completed",
			"timestamp", timestamp.Format(time.RFC3339),
			"contributors_deleted", len(contributorIDs),
		)
	}

	return contributorIDs, nil
}

// MergeContributors merges a source contributor into a target contributor.
// This is used when a user identifies that two contributors are the same person
// (e.g., "Richard Bachman" is actually "Stephen King").
//
// The merge operation:
//  1. Re-links all books from source to target, preserving original attribution via CreditedAs
//  2. Adds source's name to target's Aliases field
//  3. Soft-deletes the source contributor
//
// After merge:
//   - Books originally by "Richard Bachman" now link to "Stephen King"
//   - Those books have CreditedAs = "Richard Bachman" to preserve original credit
//   - Stephen King's Aliases include "Richard Bachman"
//   - Future book scans for "Richard Bachman" automatically link to Stephen King
//
// Returns the updated target contributor.
func (s *Store) MergeContributors(ctx context.Context, sourceID, targetID string) (*domain.Contributor, error) {
	// Validate: can't merge contributor into itself
	if sourceID == targetID {
		return nil, errors.New("cannot merge contributor into itself")
	}

	// Get both contributors
	source, err := s.GetContributor(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("get source contributor: %w", err)
	}

	target, err := s.GetContributor(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("get target contributor: %w", err)
	}

	// Get all books linked to the source contributor
	books, err := s.GetBooksByContributor(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("get books for source contributor: %w", err)
	}

	// Re-link each book from source to target
	for _, book := range books {
		updated := false
		newContributors := make([]domain.BookContributor, 0, len(book.Contributors))

		// First pass: collect all non-source contributors, noting target's roles if present
		var existingTargetIdx = -1
		var sourceRoles []domain.ContributorRole
		var sourceCreditedAs string

		for _, bc := range book.Contributors {
			switch bc.ContributorID {
			case sourceID:
				// Remember source's roles and creditedAs for merging
				sourceRoles = bc.Roles
				if bc.CreditedAs != "" {
					sourceCreditedAs = bc.CreditedAs
				} else {
					sourceCreditedAs = source.Name
				}
				updated = true
			case targetID:
				// Target already exists in this book
				existingTargetIdx = len(newContributors)
				newContributors = append(newContributors, bc)
			default:
				// Other contributors pass through unchanged
				newContributors = append(newContributors, bc)
			}
		}

		// Second pass: merge source into target
		if updated {
			if existingTargetIdx >= 0 {
				// Target already exists - merge roles
				newContributors[existingTargetIdx].Roles = mergeRoles(newContributors[existingTargetIdx].Roles, sourceRoles)
				// If target didn't have creditedAs but source did, preserve source's attribution
				if newContributors[existingTargetIdx].CreditedAs == "" && sourceCreditedAs != "" {
					newContributors[existingTargetIdx].CreditedAs = sourceCreditedAs
				}
			} else {
				// Target doesn't exist in this book - add it with source's roles
				newContributors = append(newContributors, domain.BookContributor{
					ContributorID: targetID,
					Roles:         sourceRoles,
					CreditedAs:    sourceCreditedAs,
				})
			}

			book.Contributors = newContributors
			if err := s.UpdateBook(ctx, book); err != nil {
				return nil, fmt.Errorf("update book %s contributors: %w", book.ID, err)
			}
		}
	}

	// Add source's name to target's aliases (if not already present)
	aliasExists := false
	normalizedSourceName := normalizeContributorName(source.Name)
	for _, alias := range target.Aliases {
		if normalizeContributorName(alias) == normalizedSourceName {
			aliasExists = true
			break
		}
	}
	if !aliasExists {
		target.Aliases = append(target.Aliases, source.Name)
	}

	// Also add any aliases that source had
	for _, sourceAlias := range source.Aliases {
		normalizedAlias := normalizeContributorName(sourceAlias)
		exists := false
		for _, targetAlias := range target.Aliases {
			if normalizeContributorName(targetAlias) == normalizedAlias {
				exists = true
				break
			}
		}
		if !exists {
			target.Aliases = append(target.Aliases, sourceAlias)
		}
	}

	// Update target with new aliases
	if err := s.UpdateContributor(ctx, target); err != nil {
		return nil, fmt.Errorf("update target contributor aliases: %w", err)
	}

	// Soft-delete the source contributor
	if err := s.DeleteContributor(ctx, sourceID); err != nil {
		return nil, fmt.Errorf("delete source contributor: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("merged contributors",
			"source_id", sourceID,
			"source_name", source.Name,
			"target_id", targetID,
			"target_name", target.Name,
			"books_relinked", len(books),
		)
	}

	return target, nil
}

// mergeRoles combines two role slices, removing duplicates.
func mergeRoles(a, b []domain.ContributorRole) []domain.ContributorRole {
	roleSet := make(map[domain.ContributorRole]bool)
	for _, r := range a {
		roleSet[r] = true
	}
	for _, r := range b {
		roleSet[r] = true
	}

	result := make([]domain.ContributorRole, 0, len(roleSet))
	for r := range roleSet {
		result = append(result, r)
	}
	return result
}

// UnmergeContributor splits an alias back into a separate contributor.
// This is the reverse of MergeContributors - when a user decides that
// "Richard Bachman" should be a separate contributor from "Stephen King".
//
// The unmerge operation:
//  1. Creates a new contributor with the alias name
//  2. Finds all books where the source contributor has CreditedAs matching the alias
//  3. Re-links those books to the new contributor (clears CreditedAs since now linked correctly)
//  4. Removes the alias from the source contributor's Aliases field
//
// Returns the newly created contributor.
func (s *Store) UnmergeContributor(ctx context.Context, sourceID, aliasName string) (*domain.Contributor, error) {
	// Get source contributor
	source, err := s.GetContributor(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("get source contributor: %w", err)
	}

	// Verify the alias exists
	normalizedAlias := normalizeContributorName(aliasName)
	aliasFound := false
	aliasIndex := -1
	for i, alias := range source.Aliases {
		if normalizeContributorName(alias) == normalizedAlias {
			aliasFound = true
			aliasIndex = i
			break
		}
	}
	if !aliasFound {
		return nil, fmt.Errorf("alias %q not found on contributor %s", aliasName, sourceID)
	}

	// Create a new contributor with the alias name
	newContributorID, err := id.Generate("contributor")
	if err != nil {
		return nil, fmt.Errorf("generate contributor ID: %w", err)
	}

	newContributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: newContributorID,
		},
		Name: aliasName, // Use the original alias name (preserves casing)
	}
	newContributor.InitTimestamps()

	if err := s.CreateContributor(ctx, newContributor); err != nil {
		return nil, fmt.Errorf("create new contributor: %w", err)
	}

	// Get all books linked to the source contributor
	books, err := s.GetBooksByContributor(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("get books for source contributor: %w", err)
	}

	// Re-link books that were credited to the alias
	booksRelinked := 0
	for _, book := range books {
		updated := false
		newContributors := make([]domain.BookContributor, 0, len(book.Contributors))

		for _, bc := range book.Contributors {
			if bc.ContributorID == sourceID && bc.CreditedAs != "" {
				// Check if this book was credited to the alias we're unmerging
				if normalizeContributorName(bc.CreditedAs) == normalizedAlias {
					// Re-link to new contributor, clear creditedAs
					newContributors = append(newContributors, domain.BookContributor{
						ContributorID: newContributorID,
						Roles:         bc.Roles,
						CreditedAs:    "", // Clear - now linked to correct contributor
					})
					updated = true
					booksRelinked++
				} else {
					// Keep with source (different alias or no match)
					newContributors = append(newContributors, bc)
				}
			} else {
				// Keep unchanged
				newContributors = append(newContributors, bc)
			}
		}

		if updated {
			book.Contributors = newContributors
			if err := s.UpdateBook(ctx, book); err != nil {
				return nil, fmt.Errorf("update book %s contributors: %w", book.ID, err)
			}
		}
	}

	// Remove the alias from source contributor
	newAliases := make([]string, 0, len(source.Aliases)-1)
	for i, alias := range source.Aliases {
		if i != aliasIndex {
			newAliases = append(newAliases, alias)
		}
	}
	source.Aliases = newAliases

	if err := s.UpdateContributor(ctx, source); err != nil {
		return nil, fmt.Errorf("update source contributor aliases: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("unmerged contributor",
			"source_id", sourceID,
			"source_name", source.Name,
			"new_id", newContributorID,
			"alias_name", aliasName,
			"books_relinked", booksRelinked,
		)
	}

	s.eventEmitter.Emit(sse.NewContributorCreatedEvent(newContributor))

	return newContributor, nil
}

// CountContributors returns the total number of non-deleted contributors.
// This is more efficient than ListContributors when only the count is needed.
func (s *Store) CountContributors(_ context.Context) (int, error) {
	var count int
	prefix := []byte(contributorPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys for counting

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// We need to check if the contributor is deleted, so we must fetch the value
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var contributor domain.Contributor
				if err := json.Unmarshal(val, &contributor); err != nil {
					return nil // Skip malformed entries
				}
				if contributor.DeletedAt == nil {
					count++
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("count contributors: %w", err)
	}

	return count, nil
}
