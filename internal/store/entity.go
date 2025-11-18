package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

// Entity provides generic CRUD operations for any domain type.
type Entity[T any] struct {
	store   *Store
	prefix  string
	indexes []Index[T]
}

// Index defines a secondary index on an entity.
type Index[T any] struct {
	name            string
	keyGen          func(*T) []string
	lookupTransform func(string) string // Optional transformation for lookups
}

// NewEntity creates a new Entity instance for type T.
func NewEntity[T any](s *Store, prefix string) *Entity[T] {
	return &Entity[T]{
		store:   s,
		prefix:  prefix,
		indexes: make([]Index[T], 0),
	}
}

// WithIndex adds a secondary index to the entity.
// This will be used in Task 5 for index-based queries.
func (e *Entity[T]) WithIndex(name string, keyGen func(*T) []string) *Entity[T] {
	e.indexes = append(e.indexes, Index[T]{
		name:   name,
		keyGen: keyGen,
	})
	return e
}

// WithIndexTransform adds a secondary index with lookup transformation.
// The lookupTransform function is applied to search values before index lookup,
// enabling case-insensitive searches, normalization, etc.
func (e *Entity[T]) WithIndexTransform(name string, keyGen func(*T) []string, lookupTransform func(string) string) *Entity[T] {
	e.indexes = append(e.indexes, Index[T]{
		name:            name,
		keyGen:          keyGen,
		lookupTransform: lookupTransform,
	})
	return e
}

// Create creates a new entity with the given ID.
// Returns ErrAlreadyExists if an entity with this ID already exists.
func (e *Entity[T]) Create(ctx context.Context, id string, entity *T) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := e.prefix + id

	// Marshal the entity
	data, err := json.Marshal(entity)
	if err != nil {
		return fmt.Errorf("failed to marshal entity: %w", err)
	}

	// Write to database
	err = e.store.db.Update(func(txn *badger.Txn) error {
		// Check if key already exists
		_, err := txn.Get([]byte(key))
		if err == nil {
			return ErrAlreadyExists
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("failed to check existing key: %w", err)
		}

		// Check for index conflicts
		for _, idx := range e.indexes {
			indexKeys := idx.keyGen(entity)
			for _, indexKey := range indexKeys {
				idxKey := e.prefix + "idx:" + idx.name + ":" + indexKey
				_, err := txn.Get([]byte(idxKey))
				if err == nil {
					return fmt.Errorf("index %s conflict on key %s: %w", idx.name, indexKey, ErrAlreadyExists)
				}
				if !errors.Is(err, badger.ErrKeyNotFound) {
					return fmt.Errorf("failed to check index key: %w", err)
				}
			}
		}

		// Set the primary key
		if err := txn.Set([]byte(key), data); err != nil {
			return fmt.Errorf("failed to set key: %w", err)
		}

		// Set index keys
		for _, idx := range e.indexes {
			indexKeys := idx.keyGen(entity)
			for _, indexKey := range indexKeys {
				idxKey := e.prefix + "idx:" + idx.name + ":" + indexKey
				if err := txn.Set([]byte(idxKey), []byte(id)); err != nil {
					return fmt.Errorf("failed to set index key: %w", err)
				}
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// Get retrieves an entity by ID.
// Returns ErrNotFound if the entity does not exist.
func (e *Entity[T]) Get(ctx context.Context, id string) (*T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := e.prefix + id
	var entity T

	err := e.store.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to get key: %w", err)
		}

		return item.Value(func(val []byte) error {
			if err := json.Unmarshal(val, &entity); err != nil {
				return fmt.Errorf("failed to unmarshal entity: %w", err)
			}
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return &entity, nil
}

// GetByIndex retrieves an entity by secondary index.
// If the index has a lookup transform, it will be applied to the value before lookup.
func (e *Entity[T]) GetByIndex(ctx context.Context, indexName, value string) (*T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Find the index and apply transformation if available
	transformedValue := value
	for _, idx := range e.indexes {
		if idx.name == indexName && idx.lookupTransform != nil {
			transformedValue = idx.lookupTransform(value)
			break
		}
	}

	indexKey := []byte(e.prefix + "idx:" + indexName + ":" + transformedValue)

	var id string
	err := e.store.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(indexKey)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			return err
		}

		return item.Value(func(val []byte) error {
			id = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return e.Get(ctx, id)
}

// Update updates an existing entity.
// Returns ErrNotFound if the entity does not exist.
func (e *Entity[T]) Update(ctx context.Context, id string, entity *T) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := e.prefix + id

	// Marshal the entity
	data, err := json.Marshal(entity)
	if err != nil {
		return fmt.Errorf("failed to marshal entity: %w", err)
	}

	err = e.store.db.Update(func(txn *badger.Txn) error {
		// Get the old entity to clean up old indexes
		var oldEntity T
		item, err := txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to get existing key: %w", err)
		}

		err = item.Value(func(val []byte) error {
			if err := json.Unmarshal(val, &oldEntity); err != nil {
				return fmt.Errorf("failed to unmarshal old entity: %w", err)
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Delete old index keys
		for _, idx := range e.indexes {
			oldIndexKeys := idx.keyGen(&oldEntity)
			for _, indexKey := range oldIndexKeys {
				idxKey := e.prefix + "idx:" + idx.name + ":" + indexKey
				if err := txn.Delete([]byte(idxKey)); err != nil {
					return fmt.Errorf("failed to delete old index key: %w", err)
				}
			}
		}

		// Check for new index conflicts (excluding old keys)
		for _, idx := range e.indexes {
			newIndexKeys := idx.keyGen(entity)
			oldIndexKeys := idx.keyGen(&oldEntity)

			// Build map of old keys for quick lookup
			oldKeys := make(map[string]bool)
			for _, k := range oldIndexKeys {
				oldKeys[k] = true
			}

			for _, indexKey := range newIndexKeys {
				// Skip if this is an old key being reused
				if oldKeys[indexKey] {
					continue
				}

				idxKey := e.prefix + "idx:" + idx.name + ":" + indexKey
				_, err := txn.Get([]byte(idxKey))
				if err == nil {
					return fmt.Errorf("index %s conflict on key %s: %w", idx.name, indexKey, ErrAlreadyExists)
				}
				if !errors.Is(err, badger.ErrKeyNotFound) {
					return fmt.Errorf("failed to check index key: %w", err)
				}
			}
		}

		// Set the primary key
		if err := txn.Set([]byte(key), data); err != nil {
			return fmt.Errorf("failed to set key: %w", err)
		}

		// Set new index keys
		for _, idx := range e.indexes {
			indexKeys := idx.keyGen(entity)
			for _, indexKey := range indexKeys {
				idxKey := e.prefix + "idx:" + idx.name + ":" + indexKey
				if err := txn.Set([]byte(idxKey), []byte(id)); err != nil {
					return fmt.Errorf("failed to set index key: %w", err)
				}
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// Delete deletes an entity by ID.
// This operation is idempotent - it does not return an error if the entity does not exist.
func (e *Entity[T]) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := e.prefix + id

	err := e.store.db.Update(func(txn *badger.Txn) error {
		// Get the entity to clean up indexes
		var entity T
		item, err := txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			// Idempotent - no error if doesn't exist
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get key: %w", err)
		}

		err = item.Value(func(val []byte) error {
			if err := json.Unmarshal(val, &entity); err != nil {
				return fmt.Errorf("failed to unmarshal entity: %w", err)
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Delete index keys
		for _, idx := range e.indexes {
			indexKeys := idx.keyGen(&entity)
			for _, indexKey := range indexKeys {
				idxKey := e.prefix + "idx:" + idx.name + ":" + indexKey
				if err := txn.Delete([]byte(idxKey)); err != nil {
					return fmt.Errorf("failed to delete index key: %w", err)
				}
			}
		}

		// Delete the primary key
		if err := txn.Delete([]byte(key)); err != nil {
			return fmt.Errorf("failed to delete key: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// List returns an iterator over all entities.
func (e *Entity[T]) List(ctx context.Context) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		e.store.db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.Prefix = []byte(e.prefix)
			opts.PrefetchValues = true

			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Seek([]byte(e.prefix)); it.ValidForPrefix([]byte(e.prefix)); it.Next() {
				// Check context cancellation
				if ctx.Err() != nil {
					yield(nil, ctx.Err())
					return ctx.Err()
				}

				// Skip index keys
				key := string(it.Item().Key())
				if len(key) > len(e.prefix) {
					remainder := key[len(e.prefix):]
					if strings.HasPrefix(remainder, "idx:") {
						continue
					}
				}

				var entity T
				err := it.Item().Value(func(val []byte) error {
					return json.Unmarshal(val, &entity)
				})

				if err != nil {
					yield(nil, err)
					return err
				}

				if !yield(&entity, nil) {
					return nil // Consumer stopped early
				}
			}

			return nil
		})
	}
}
