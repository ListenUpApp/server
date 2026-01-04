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
	"github.com/listenupapp/listenup-server/internal/genre"
	"github.com/listenupapp/listenup-server/internal/id"
)

// Key prefixes for genre storage.
const (
	genrePrefix         = "genre:"
	genreBySlugPrefix   = "idx:genre:slug:"   // slug -> genre ID
	genreByParentPrefix = "idx:genre:parent:" // parentID:genreID -> empty
	genreAliasPrefix    = "genre_alias:"
	genreAliasByRaw     = "idx:genre_alias:raw:" // raw -> alias ID
	unmappedGenrePrefix = "unmapped_genre:"
	bookGenrePrefix     = "idx:book:genre:" // bookID:genreID -> empty
	genreBookPrefix     = "idx:genre:book:" // genreID:bookID -> empty
)

// Genre errors.
var (
	ErrGenreNotFound      = errors.New("genre not found")
	ErrGenreExists        = errors.New("genre already exists")
	ErrGenreHasChildren   = errors.New("genre has children")
	ErrCannotDeleteSystem = errors.New("cannot delete system genre")
)

// CreateGenre creates a new genre.
func (s *Store) CreateGenre(ctx context.Context, g *domain.Genre) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := []byte(genrePrefix + g.ID)

	err := s.db.Update(func(txn *badger.Txn) error {
		// Check if already exists.
		if _, err := txn.Get(key); err == nil {
			return ErrGenreExists
		}

		// Store genre.
		data, err := json.Marshal(g)
		if err != nil {
			return fmt.Errorf("marshal genre: %w", err)
		}
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create slug index.
		slugKey := []byte(genreBySlugPrefix + g.Slug)
		if err := txn.Set(slugKey, []byte(g.ID)); err != nil {
			return err
		}

		// Create parent index (for tree queries).
		if g.ParentID != "" {
			parentKey := []byte(genreByParentPrefix + g.ParentID + ":" + g.ID)
			if err := txn.Set(parentKey, []byte{}); err != nil {
				return err
			}
		} else {
			// Root genres indexed under empty parent.
			parentKey := []byte(genreByParentPrefix + "root:" + g.ID)
			if err := txn.Set(parentKey, []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}
	s.InvalidateGenreCache()
	return nil
}

// GetGenre retrieves a genre by ID.
// Uses in-memory cache for fast lookups when available.
func (s *Store) GetGenre(ctx context.Context, id string) (*domain.Genre, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Check cache first
	if cached := s.getGenreFromCache(id); cached != nil {
		if cached.IsDeleted() {
			return nil, ErrGenreNotFound
		}
		return cached, nil
	}

	// Cache miss - fetch from DB
	var g domain.Genre
	key := []byte(genrePrefix + id)

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrGenreNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &g)
		})
	})

	if err != nil {
		return nil, err
	}

	// Treat soft-deleted as not found.
	if g.IsDeleted() {
		return nil, ErrGenreNotFound
	}

	return &g, nil
}

// GetGenresByIDs retrieves multiple genres by their IDs.
// Missing or deleted genres are silently skipped (no error returned).
// This is used by the enricher for batch denormalization.
func (s *Store) GetGenresByIDs(ctx context.Context, ids []string) ([]*domain.Genre, error) {
	genres := make([]*domain.Genre, 0, len(ids))

	for _, id := range ids {
		genre, err := s.GetGenre(ctx, id)
		if err != nil {
			if errors.Is(err, ErrGenreNotFound) {
				continue // Skip missing genres
			}
			return nil, err
		}
		genres = append(genres, genre)
	}

	return genres, nil
}

// GetGenreBySlug retrieves a genre by its slug.
func (s *Store) GetGenreBySlug(ctx context.Context, slug string) (*domain.Genre, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var genreID string
	slugKey := []byte(genreBySlugPrefix + slug)

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(slugKey)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrGenreNotFound
		}
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			genreID = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return s.GetGenre(ctx, genreID)
}

// GetOrCreateGenreBySlug finds or creates a genre.
func (s *Store) GetOrCreateGenreBySlug(ctx context.Context, slug, name, parentID string) (*domain.Genre, error) {
	// Try to find existing.
	existing, err := s.GetGenreBySlug(ctx, slug)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrGenreNotFound) {
		return nil, err
	}

	// Create new genre.
	genreID, err := id.Generate("genre")
	if err != nil {
		return nil, fmt.Errorf("generate genre ID: %w", err)
	}

	// Build path.
	var path string
	var depth int
	if parentID != "" {
		parent, err := s.GetGenre(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("get parent genre: %w", err)
		}
		path = parent.Path + "/" + slug
		depth = parent.Depth + 1
	} else {
		path = "/" + slug
		depth = 0
	}

	g := &domain.Genre{
		Syncable: domain.Syncable{ID: genreID},
		Name:     name,
		Slug:     slug,
		ParentID: parentID,
		Path:     path,
		Depth:    depth,
	}
	g.InitTimestamps()

	if err := s.CreateGenre(ctx, g); err != nil {
		return nil, err
	}

	return g, nil
}

// ListGenres returns all genres.
// Uses in-memory cache for fast lookups; populates cache on first call.
func (s *Store) ListGenres(ctx context.Context) ([]*domain.Genre, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Check cache first
	if cached := s.getGenreListFromCache(); cached != nil {
		return cached, nil
	}

	// Cache miss - load from DB
	var genres []*domain.Genre
	prefix := []byte(genrePrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var g domain.Genre
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &g)
			})
			if err != nil {
				continue
			}
			if !g.IsDeleted() {
				genres = append(genres, &g)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by path for consistent tree order.
	slices.SortFunc(genres, func(a, b *domain.Genre) int {
		return strings.Compare(a.Path, b.Path)
	})

	// Populate cache for future calls
	s.populateGenreCache(genres)

	return genres, nil
}

// GetGenreChildren returns direct children of a genre.
func (s *Store) GetGenreChildren(ctx context.Context, parentID string) ([]*domain.Genre, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := genreByParentPrefix + parentID + ":"
	if parentID == "" {
		prefix = genreByParentPrefix + "root:"
	}

	var childIDs []string

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			key := string(it.Item().Key())
			childID := strings.TrimPrefix(key, prefix)
			childIDs = append(childIDs, childID)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	children := make([]*domain.Genre, 0, len(childIDs))
	for _, childID := range childIDs {
		g, err := s.GetGenre(ctx, childID)
		if err != nil {
			continue
		}
		children = append(children, g)
	}

	// Sort by sort_order, then name.
	slices.SortFunc(children, func(a, b *domain.Genre) int {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder - b.SortOrder
		}
		return strings.Compare(a.Name, b.Name)
	})

	return children, nil
}

// UpdateGenre updates a genre.
func (s *Store) UpdateGenre(ctx context.Context, g *domain.Genre) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get old genre for index updates.
	old, err := s.GetGenre(ctx, g.ID)
	if err != nil {
		return err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Update main record.
		data, err := json.Marshal(g)
		if err != nil {
			return err
		}
		key := []byte(genrePrefix + g.ID)
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update slug index if changed.
		if old.Slug != g.Slug {
			// Remove old.
			oldSlugKey := []byte(genreBySlugPrefix + old.Slug)
			if err := txn.Delete(oldSlugKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			// Add new.
			newSlugKey := []byte(genreBySlugPrefix + g.Slug)
			if err := txn.Set(newSlugKey, []byte(g.ID)); err != nil {
				return err
			}
		}

		// Update parent index if changed.
		if old.ParentID != g.ParentID {
			// Remove from old parent.
			oldParentKey := genreByParentPrefix + old.ParentID + ":" + g.ID
			if old.ParentID == "" {
				oldParentKey = genreByParentPrefix + "root:" + g.ID
			}
			if err := txn.Delete([]byte(oldParentKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			// Add to new parent.
			newParentKey := genreByParentPrefix + g.ParentID + ":" + g.ID
			if g.ParentID == "" {
				newParentKey = genreByParentPrefix + "root:" + g.ID
			}
			if err := txn.Set([]byte(newParentKey), []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}
	s.InvalidateGenreCache()
	return nil
}

// DeleteGenre deletes a genre (must have no children and not be system).
func (s *Store) DeleteGenre(ctx context.Context, id string) error {
	g, err := s.GetGenre(ctx, id)
	if err != nil {
		return err
	}

	if g.IsSystem {
		return ErrCannotDeleteSystem
	}

	// Check for children.
	children, err := s.GetGenreChildren(ctx, id)
	if err != nil {
		return err
	}
	if len(children) > 0 {
		return ErrGenreHasChildren
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Soft delete.
		g.MarkDeleted()
		data, err := json.Marshal(g)
		if err != nil {
			return err
		}

		key := []byte(genrePrefix + id)
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Remove from indexes.
		slugKey := []byte(genreBySlugPrefix + g.Slug)
		if err := txn.Delete(slugKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		parentKey := genreByParentPrefix + g.ParentID + ":" + id
		if g.ParentID == "" {
			parentKey = genreByParentPrefix + "root:" + id
		}
		if err := txn.Delete([]byte(parentKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	s.InvalidateGenreCache()
	return nil
}

// MoveGenre changes a genre's parent (re-parents in tree).
func (s *Store) MoveGenre(ctx context.Context, genreID, newParentID string) error {
	g, err := s.GetGenre(ctx, genreID)
	if err != nil {
		return err
	}

	// Calculate new path and depth.
	var newPath string
	var newDepth int
	if newParentID != "" {
		newParent, err := s.GetGenre(ctx, newParentID)
		if err != nil {
			return err
		}
		newPath = newParent.Path + "/" + g.Slug
		newDepth = newParent.Depth + 1
	} else {
		newPath = "/" + g.Slug
		newDepth = 0
	}

	oldPath := g.Path

	g.ParentID = newParentID
	g.Path = newPath
	g.Depth = newDepth
	g.Touch()

	if err := s.UpdateGenre(ctx, g); err != nil {
		return err
	}

	// Recursively update all descendants' paths.
	return s.updateDescendantPaths(ctx, genreID, oldPath, newPath)
}

// updateDescendantPaths updates paths for all descendants after a move.
func (s *Store) updateDescendantPaths(ctx context.Context, parentID, oldPrefix, newPrefix string) error {
	children, err := s.GetGenreChildren(ctx, parentID)
	if err != nil {
		return err
	}

	for _, child := range children {
		child.Path = strings.Replace(child.Path, oldPrefix, newPrefix, 1)
		child.Depth = strings.Count(child.Path, "/") - 1
		child.Touch()

		if err := s.UpdateGenre(ctx, child); err != nil {
			return err
		}

		// Recurse.
		if err := s.updateDescendantPaths(ctx, child.ID, oldPrefix, newPrefix); err != nil {
			return err
		}
	}

	return nil
}

// MergeGenres merges source genre into target, reassigning all books.
func (s *Store) MergeGenres(ctx context.Context, sourceID, targetID string) error {
	source, err := s.GetGenre(ctx, sourceID)
	if err != nil {
		return err
	}

	_, err = s.GetGenre(ctx, targetID)
	if err != nil {
		return err
	}

	// Get all books in source genre.
	bookIDs, err := s.GetBookIDsForGenre(ctx, sourceID)
	if err != nil {
		return err
	}

	// Move books to target.
	for _, bookID := range bookIDs {
		if err := s.RemoveBookGenre(ctx, bookID, sourceID); err != nil {
			return err
		}
		if err := s.AddBookGenre(ctx, bookID, targetID); err != nil {
			return err
		}
	}

	// Move children to target.
	children, err := s.GetGenreChildren(ctx, sourceID)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := s.MoveGenre(ctx, child.ID, targetID); err != nil {
			return err
		}
	}

	// Delete source.
	source.MarkDeleted()
	return s.UpdateGenre(ctx, source)
}

// --- Book-Genre Association ---

// AddBookGenre adds a genre to a book.
func (s *Store) AddBookGenre(ctx context.Context, bookID, genreID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// book -> genre index.
		bgKey := []byte(fmt.Sprintf("%s%s:%s", bookGenrePrefix, bookID, genreID))
		if err := txn.Set(bgKey, []byte{}); err != nil {
			return err
		}

		// genre -> book index (for listing books by genre).
		gbKey := []byte(fmt.Sprintf("%s%s:%s", genreBookPrefix, genreID, bookID))
		return txn.Set(gbKey, []byte{})
	})
}

// RemoveBookGenre removes a genre from a book.
func (s *Store) RemoveBookGenre(ctx context.Context, bookID, genreID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		bgKey := []byte(fmt.Sprintf("%s%s:%s", bookGenrePrefix, bookID, genreID))
		if err := txn.Delete(bgKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		gbKey := []byte(fmt.Sprintf("%s%s:%s", genreBookPrefix, genreID, bookID))
		if err := txn.Delete(gbKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		return nil
	})
}

// SetBookGenres sets all genres for a book (replaces existing).
func (s *Store) SetBookGenres(ctx context.Context, bookID string, genreIDs []string) error {
	// Get current genres.
	currentIDs, err := s.GetGenreIDsForBook(ctx, bookID)
	if err != nil {
		return err
	}

	// Remove genres not in new list.
	currentSet := make(map[string]bool)
	for _, gid := range currentIDs {
		currentSet[gid] = true
	}

	newSet := make(map[string]bool)
	for _, gid := range genreIDs {
		newSet[gid] = true
	}

	// Remove old.
	for _, gid := range currentIDs {
		if !newSet[gid] {
			if err := s.RemoveBookGenre(ctx, bookID, gid); err != nil {
				return err
			}
		}
	}

	// Add new.
	for _, gid := range genreIDs {
		if !currentSet[gid] {
			if err := s.AddBookGenre(ctx, bookID, gid); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetGenreIDsForBook returns all genre IDs for a book.
func (s *Store) GetGenreIDsForBook(ctx context.Context, bookID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s%s:", bookGenrePrefix, bookID)
	var genreIDs []string

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(prefix)
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(prefix)); it.ValidForPrefix([]byte(prefix)); it.Next() {
			key := string(it.Item().Key())
			genreID := strings.TrimPrefix(key, prefix)
			genreIDs = append(genreIDs, genreID)
		}
		return nil
	})

	return genreIDs, err
}

// GetBookIDsForGenre returns all book IDs in a genre (direct, not inherited).
func (s *Store) GetBookIDsForGenre(ctx context.Context, genreID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("%s%s:", genreBookPrefix, genreID)
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

// GetBookIDsForGenreTree returns book IDs in a genre AND all its descendants.
func (s *Store) GetBookIDsForGenreTree(ctx context.Context, genreID string) ([]string, error) {
	g, err := s.GetGenre(ctx, genreID)
	if err != nil {
		return nil, err
	}

	// Get all genres with matching path prefix.
	allGenres, err := s.ListGenres(ctx)
	if err != nil {
		return nil, err
	}

	var matchingGenreIDs []string
	for _, gen := range allGenres {
		if strings.HasPrefix(gen.Path, g.Path) {
			matchingGenreIDs = append(matchingGenreIDs, gen.ID)
		}
	}

	// Collect unique book IDs.
	bookSet := make(map[string]bool)
	for _, gid := range matchingGenreIDs {
		bookIDs, err := s.GetBookIDsForGenre(ctx, gid)
		if err != nil {
			return nil, err
		}
		for _, bid := range bookIDs {
			bookSet[bid] = true
		}
	}

	result := make([]string, 0, len(bookSet))
	for bid := range bookSet {
		result = append(result, bid)
	}

	return result, nil
}

// --- Genre Aliases ---

// CreateGenreAlias creates a mapping from raw string to genre(s).
func (s *Store) CreateGenreAlias(ctx context.Context, alias *domain.GenreAlias) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(alias)
		if err != nil {
			return err
		}

		key := []byte(genreAliasPrefix + alias.ID)
		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Index by raw value (lowercase for lookup).
		rawKey := []byte(genreAliasByRaw + strings.ToLower(alias.RawValue))
		return txn.Set(rawKey, []byte(alias.ID))
	})
}

// GetGenreAliasByRaw looks up an alias by raw metadata string.
func (s *Store) GetGenreAliasByRaw(ctx context.Context, raw string) (*domain.GenreAlias, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rawKey := []byte(genreAliasByRaw + strings.ToLower(raw))

	var aliasID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(rawKey)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrGenreNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			aliasID = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Fetch full alias.
	var alias domain.GenreAlias
	key := []byte(genreAliasPrefix + aliasID)
	err = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &alias)
		})
	})

	return &alias, err
}

// --- Unmapped Genres ---

// TrackUnmappedGenre records a genre string that couldn't be normalized.
func (s *Store) TrackUnmappedGenre(ctx context.Context, raw string, bookID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key := []byte(unmappedGenrePrefix + genre.Slugify(raw))

	return s.db.Update(func(txn *badger.Txn) error {
		var unmapped domain.UnmappedGenre

		item, err := txn.Get(key)
		if err == nil {
			// Exists - update.
			err = item.Value(func(val []byte) error {
				return json.Unmarshal(val, &unmapped)
			})
			if err != nil {
				return err
			}

			unmapped.BookCount++
			if len(unmapped.BookIDs) < 10 { // Keep sample of up to 10.
				unmapped.BookIDs = append(unmapped.BookIDs, bookID)
			}
		} else if errors.Is(err, badger.ErrKeyNotFound) {
			// New unmapped genre.
			unmapped = domain.UnmappedGenre{
				RawValue:  raw,
				BookCount: 1,
				FirstSeen: time.Now(),
				BookIDs:   []string{bookID},
			}
		} else {
			return err
		}

		data, err := json.Marshal(unmapped)
		if err != nil {
			return err
		}
		return txn.Set(key, data)
	})
}

// ListUnmappedGenres returns all unmapped genre strings.
func (s *Store) ListUnmappedGenres(ctx context.Context) ([]*domain.UnmappedGenre, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var unmapped []*domain.UnmappedGenre
	prefix := []byte(unmappedGenrePrefix)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var u domain.UnmappedGenre
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &u)
			})
			if err != nil {
				continue
			}
			unmapped = append(unmapped, &u)
		}
		return nil
	})

	// Sort by book count descending (most common first).
	slices.SortFunc(unmapped, func(a, b *domain.UnmappedGenre) int {
		return b.BookCount - a.BookCount
	})

	return unmapped, err
}

// ResolveUnmappedGenre creates an alias and removes from unmapped.
func (s *Store) ResolveUnmappedGenre(ctx context.Context, raw string, genreIDs []string, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Create alias.
	aliasID, err := id.Generate("alias")
	if err != nil {
		return err
	}

	alias := &domain.GenreAlias{
		ID:        aliasID,
		RawValue:  raw,
		GenreIDs:  genreIDs,
		CreatedBy: userID,
		CreatedAt: time.Now(),
	}

	if err := s.CreateGenreAlias(ctx, alias); err != nil {
		return err
	}

	// Remove from unmapped.
	key := []byte(unmappedGenrePrefix + genre.Slugify(raw))
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

// --- Seeding ---

// SeedDefaultGenres creates the default genre hierarchy.
func (s *Store) SeedDefaultGenres(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Check if already seeded.
	genres, err := s.ListGenres(ctx)
	if err != nil {
		return err
	}
	if len(genres) > 0 {
		return nil // Already seeded.
	}

	return s.seedGenreTree(ctx, genre.DefaultGenres, "", "")
}

func (s *Store) seedGenreTree(ctx context.Context, seeds []genre.GenreSeed, parentID, parentPath string) error {
	for i, seed := range seeds {
		genreID, err := id.Generate("genre")
		if err != nil {
			return err
		}

		path := "/" + seed.Slug
		depth := 0
		if parentPath != "" {
			path = parentPath + "/" + seed.Slug
			depth = strings.Count(path, "/") - 1
		}

		g := &domain.Genre{
			Syncable:  domain.Syncable{ID: genreID},
			Name:      seed.Name,
			Slug:      seed.Slug,
			ParentID:  parentID,
			Path:      path,
			Depth:     depth,
			SortOrder: i,
			IsSystem:  true, // Default genres are system genres.
		}
		g.InitTimestamps()

		if err := s.CreateGenre(ctx, g); err != nil {
			return fmt.Errorf("create genre %s: %w", seed.Name, err)
		}

		// Recurse for children.
		if len(seed.Children) > 0 {
			if err := s.seedGenreTree(ctx, seed.Children, genreID, path); err != nil {
				return err
			}
		}
	}

	return nil
}
