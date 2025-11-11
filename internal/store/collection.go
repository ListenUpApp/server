package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// Key prefixes for BadgerDB
const (
	libraryPrefix    = "library:"
	collectionPrefix = "collection:"

	// Index Key
	collectionsByLibraryPrefix = "idx:collections:library:"
	collectionsByTypePrefix    = "idx:collections:type:"
)

var (
	ErrLibraryNotFound     = errors.New("library not found")
	ErrCollectionNotFound  = errors.New("collection not found")
	ErrDuplicateLibrary    = errors.New("library already exists")
	ErrDuplicateCollection = errors.New("collection already exists")
)

func (s *Store) CreateLibrary(ctx context.Context, lib *domain.Library) error {
	key := []byte(libraryPrefix + lib.ID)

	// Check if already exists
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check library exists: %w", err)
	}
	if exists {
		return ErrDuplicateLibrary
	}

	if err := s.set(key, lib); err != nil {
		return fmt.Errorf("save library: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("library created", "id", lib.ID, "name", lib.Name, "scan_paths", len(lib.ScanPaths))
	}
	return nil
}

func (s *Store) GetLibrary(ctx context.Context, id string) (*domain.Library, error) {
	key := []byte(libraryPrefix + id)

	var lib domain.Library
	if err := s.get(key, &lib); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrLibraryNotFound
		}
		return nil, fmt.Errorf("get library: %w", err)
	}

	return &lib, nil
}

func (s *Store) UpdateLibrary(ctx context.Context, lib *domain.Library) error {
	key := []byte(libraryPrefix + lib.ID)

	// Verify exists
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check library exists: %w", err)
	}
	if !exists {
		return ErrLibraryNotFound
	}

	if err := s.set(key, lib); err != nil {
		return fmt.Errorf("update library: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("library updated", "id", lib.ID, "name", lib.Name)
	}

	return nil
}

func (s *Store) DeleteLibrary(ctx context.Context, id string) error {
	// Get all collections for this library
	collections, err := s.ListCollectionsByLibrary(ctx, id)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}

	// Delete all collections first
	for _, coll := range collections {
		if err := s.DeleteCollection(ctx, coll.ID); err != nil {
			return fmt.Errorf("delete collection %s: %w", coll.ID, err)
		}
	}
	key := []byte(libraryPrefix + id)
	if err := s.delete(key); err != nil {
		return fmt.Errorf("delete library: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("library deleted", "id", id)
	}

	return nil
}

func (s *Store) ListLibraries(ctx context.Context) ([]*domain.Library, error) {
	var libraries []*domain.Library

	prefix := []byte(libraryPrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var lib domain.Library
				if err := json.Unmarshal(val, &lib); err != nil {
					return err
				}
				libraries = append(libraries, &lib)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	return libraries, nil
}

func (s *Store) GetDefaultLibrary(ctx context.Context) (*domain.Library, error) {
	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}

	if len(libraries) == 0 {
		return nil, ErrLibraryNotFound
	}

	return libraries[0], nil
}

func (s *Store) CreateCollection(ctx context.Context, coll *domain.Collection) error {
	key := []byte(collectionPrefix + coll.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}
	if exists {
		return ErrDuplicateCollection
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(coll)
		if err != nil {
			return fmt.Errorf("marshal collection: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		indexKey := []byte(collectionsByLibraryPrefix + coll.LibraryID)
		var collectionIDs []string

		item, err := txn.Get(indexKey)
		if err == nil {
			err = item.Value(func(val []byte) error {
				return json.Unmarshal(val, &collectionIDs)
			})
			if err != nil {
				return err
			}
		}

		collectionIDs = append(collectionIDs, coll.ID)
		indexData, err := json.Marshal(collectionIDs)
		if err != nil {
			return err
		}

		if err := txn.Set(indexKey, indexData); err != nil {
			return err
		}

		typeKey := []byte(collectionsByTypePrefix + coll.LibraryID + ":" + coll.Type.String())
		if err := txn.Set(typeKey, []byte(coll.ID)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("collection created", "id", coll.ID, "name", coll.Name, "type", coll.Type.String(), "library_id", coll.LibraryID)
	}
	return nil
}

func (s *Store) GetCollection(ctx context.Context, id string) (*domain.Collection, error) {
	key := []byte(collectionPrefix + id)

	var coll domain.Collection
	if err := s.get(key, &coll); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrCollectionNotFound
		}
		return nil, fmt.Errorf("get collection: %w", err)
	}

	return &coll, nil
}

func (s *Store) UpdateCollection(ctx context.Context, coll *domain.Collection) error {
	key := []byte(collectionPrefix + coll.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}

	if !exists {
		return ErrCollectionNotFound
	}

	if err := s.set(key, coll); err != nil {
		return fmt.Errorf("update collection: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("collection updated", "id", coll.ID, "name", coll.Name)
	}
	return nil
}

func (s *Store) DeleteCollection(ctx context.Context, id string) error {
	coll, err := s.GetCollection(ctx, id)
	if err != nil {
		return err
	}

	if coll.Type.IsSystemCollection() {
		return errors.New("cannot delete system collection")
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		key := []byte(collectionPrefix + id)
		if err := txn.Delete(key); err != nil {
			return err
		}

		indexKey := []byte(collectionsByLibraryPrefix + coll.LibraryID)
		var collectionIDs []string

		item, err := txn.Get(indexKey)
		if err != nil {
			return err
		}

		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &collectionIDs)
		})
		if err != nil {
			return err
		}

		collectionIDs = slices.DeleteFunc(collectionIDs, func(cid string) bool {
			return cid == id
		})

		indexData, err := json.Marshal(collectionIDs)
		if err != nil {
			return err
		}

		return txn.Set(indexKey, indexData)
	})
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("collection deleted", "id", id)
	}

	return nil
}

func (s *Store) ListCollectionsByLibrary(ctx context.Context, libraryID string) ([]*domain.Collection, error) {
	indexKey := []byte(collectionsByLibraryPrefix + libraryID)

	var collectionIDs []string

	err := s.get(indexKey, &collectionIDs)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return []*domain.Collection{}, nil
		}
		return nil, fmt.Errorf("get collection index: %w", err)
	}
	collections := make([]*domain.Collection, 0, len(collectionIDs))
	for _, id := range collectionIDs {
		coll, err := s.GetCollection(ctx, id)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("failed to get collection", "id", id, "error", err)
			}
			continue
		}
		collections = append(collections, coll)
	}
	return collections, nil
}

func (s *Store) GetCollectionByType(ctx context.Context, libraryID string, collType domain.CollectionType) (*domain.Collection, error) {
	// Use type index for fast lookup
	typeKey := []byte(collectionsByTypePrefix + libraryID + ":" + collType.String())

	var collectionID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(typeKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			collectionID = string(val)
			return nil
		})
	})
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrCollectionNotFound
		}
		return nil, fmt.Errorf("get collection type index: %w", err)
	}
	return s.GetCollection(ctx, collectionID)
}

// GetDefaultCollection returns the default collection for a library
func (s *Store) GetDefaultCollection(ctx context.Context, libraryID string) (*domain.Collection, error) {
	return s.GetCollectionByType(ctx, libraryID, domain.CollectionTypeDefault)
}

func (s *Store) GetInboxCollection(ctx context.Context, libraryID string) (*domain.Collection, error) {
	return s.GetCollectionByType(ctx, libraryID, domain.CollectionTypeInbox)
}

func (s *Store) AddBookToCollection(ctx context.Context, bookID, collectionID string) error {
	coll, err := s.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}

	if slices.Contains(coll.BookIDs, bookID) {
		// Collection already contains book, return nil
		return nil
	}

	coll.BookIDs = append(coll.BookIDs, bookID)

	return s.UpdateCollection(ctx, coll)
}

// RemoveBookFromCollection removes a book ID from a collection
func (s *Store) RemoveBookFromCollection(ctx context.Context, bookID, collectionID string) error {
	coll, err := s.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}

	coll.BookIDs = slices.DeleteFunc(coll.BookIDs, func(id string) bool {
		return id == bookID
	})

	return s.UpdateCollection(ctx, coll)
}

func (s *Store) GetCollectionsForBook(ctx context.Context, bookID string) ([]*domain.Collection, error) {
	var matchingCollections []*domain.Collection

	// We need to scan all collections (no reverse index yet)
	// For now, iterate through all libraries and their collections
	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}

	for _, lib := range libraries {
		collections, err := s.ListCollectionsByLibrary(ctx, lib.ID)
		if err != nil {
			continue
		}

		for _, coll := range collections {
			if slices.Contains(coll.BookIDs, bookID) {
				matchingCollections = append(matchingCollections, coll)
			}
		}
	}
	return matchingCollections, nil
}
