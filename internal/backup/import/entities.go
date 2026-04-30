package backupimport

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"time"

	"encoding/json/v2"

	"github.com/listenupapp/listenup-server/internal/backup/export"
	"github.com/listenupapp/listenup-server/internal/backup/stream"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// progressLogEvery controls how often importEntity emits progress logs.
const progressLogEvery = 1000

// mergeVerdict describes what to do when an entity already exists in the
// destination store during a merge restore.
type mergeVerdict int

const (
	mergeVerdictUpdate mergeVerdict = iota // overwrite existing with backup version
	mergeVerdictSkip                       // keep existing, skip backup version
)

// applyMergeStrategy returns whether a backup entity should overwrite an
// existing local entity given the active merge strategy. It is only called
// when both versions exist; full-mode restores wipe data first so existing
// will always be nil.
func applyMergeStrategy(opts RestoreOptions, backupUpdated, existingUpdated time.Time) mergeVerdict {
	if opts.Mode != RestoreModeMerge {
		return mergeVerdictUpdate
	}
	switch opts.MergeStrategy {
	case MergeKeepLocal:
		return mergeVerdictSkip
	case MergeNewest:
		if !backupUpdated.After(existingUpdated) {
			return mergeVerdictSkip
		}
		return mergeVerdictUpdate
	case MergeKeepBackup:
		return mergeVerdictUpdate
	default:
		return mergeVerdictUpdate
	}
}

// persistOutcome describes what happened when persist was called for one
// streamed entity. Returning skipped=true means the entity was intentionally
// skipped (already up-to-date, soft-deleted in merge mode, etc.) and should
// be counted toward "skipped" rather than "imported" or "errors".
type persistOutcome struct {
	skipped bool
	err     error
}

// importEntity is the generic streaming importer used for every entity type
// stored in the backup zip as a JSONL file. It centralizes file lookup, parse
// error accumulation, dry-run handling, soft-delete short-circuiting, and
// progress logging so each per-entity importer collapses to a thin wrapper
// that only declares its persist closure.
//
// fileName is the zip-relative path (e.g. "entities/users.jsonl"). entityName
// is used as RestoreError.EntityType and in progress logs. idOf returns the
// stable ID used in RestoreError.EntityID for failures (may be nil for
// append-only types whose IDs aren't useful in error reporting). isDeleted
// is consulted in merge mode to honor soft-deletes from the source; pass
// nil for entity types that have no soft-delete semantics. persistFn is
// invoked in non-dry-run mode to actually write the entity, and returns
// whether it was skipped (e.g. by merge strategy) or errored.
//
// A missing zip entry is not an error: importEntity returns zero counts so
// optional sections (older backups, partial archives) restore cleanly.
//
// Note: Go does not permit type parameters on methods, so importEntity is a
// free function that takes the Importer explicitly.
func importEntity[T any](
	ctx context.Context,
	i *Importer,
	zr *zip.ReadCloser,
	opts RestoreOptions,
	fileName string,
	entityName string,
	idOf func(*T) string,
	isDeleted func(*T) bool,
	persistFn func(ctx context.Context, item *T) persistOutcome,
) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, fileName)
	if err != nil {
		if errors.Is(err, stream.ErrFileNotFound) {
			return 0, 0, nil
		}
		return 0, 0, []RestoreError{{EntityType: entityName, Error: err.Error()}}
	}

	reader := stream.NewReader[T](rc)

	for entity, perr := range reader.All() {
		if perr != nil {
			errs = append(errs, RestoreError{
				EntityType: entityName,
				Error:      fmt.Sprintf("parse error: %v", perr),
			})
			continue
		}

		// Soft-delete short circuit (merge mode only). Full restores keep
		// soft-deleted rows so the destination matches the source exactly.
		if isDeleted != nil && opts.Mode == RestoreModeMerge && isDeleted(&entity) {
			skipped++
			continue
		}

		if opts.DryRun {
			imported++
			continue
		}

		outcome := persistFn(ctx, &entity)
		switch {
		case outcome.err != nil:
			rerr := RestoreError{EntityType: entityName, Error: outcome.err.Error()}
			if idOf != nil {
				rerr.EntityID = idOf(&entity)
			}
			errs = append(errs, rerr)
		case outcome.skipped:
			skipped++
		default:
			imported++
		}

		if total := imported + skipped; total > 0 && total%progressLogEvery == 0 {
			i.logger.Debug("import progress",
				"type", entityName,
				"imported", imported,
				"skipped", skipped,
				"errors", len(errs))
		}
	}

	return imported, skipped, errs
}

// upsertWithMerge is the common "look up existing, decide via merge strategy,
// then update or create" recipe used by most importers. It is parameterised
// over the small set of store callbacks that vary between entity types.
//
// existing must be a pointer to the existing entity or nil if not found
// (errors from get are intentionally swallowed to mirror the prior code; the
// store returns nil + ErrNotFound rather than a hard failure for missing rows).
func upsertWithMerge[T any](
	ctx context.Context,
	opts RestoreOptions,
	item *T,
	get func(ctx context.Context) (*T, error),
	updatedAt func(*T) time.Time,
	update func(ctx context.Context, item *T) error,
	create func(ctx context.Context, item *T) error,
) persistOutcome {
	existing, _ := get(ctx)
	if existing != nil {
		switch applyMergeStrategy(opts, updatedAt(item), updatedAt(existing)) {
		case mergeVerdictSkip:
			return persistOutcome{skipped: true}
		case mergeVerdictUpdate:
			if err := update(ctx, item); err != nil {
				return persistOutcome{err: err}
			}
			return persistOutcome{}
		}
	}
	if err := create(ctx, item); err != nil {
		return persistOutcome{err: err}
	}
	return persistOutcome{}
}

// --- Per-entity importers (thin wrappers over importEntity) ----------------

func (i *Importer) importUsers(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/users.jsonl",
		"users",
		func(u *export.UserExport) string { return u.ID },
		func(u *export.UserExport) bool { return u.IsDeleted() },
		func(ctx context.Context, u *export.UserExport) persistOutcome {
			user := &domain.User{
				Syncable:     u.Syncable,
				Email:        u.Email,
				PasswordHash: u.PasswordHash,
				IsRoot:       u.IsRoot,
				Role:         u.Role,
				Status:       u.Status,
				DisplayName:  u.DisplayName,
				FirstName:    u.FirstName,
				LastName:     u.LastName,
				Permissions:  u.Permissions,
				InvitedBy:    u.InvitedBy,
				ApprovedBy:   u.ApprovedBy,
			}
			return upsertWithMerge(ctx, opts, user,
				func(ctx context.Context) (*domain.User, error) { return i.store.GetUser(ctx, user.ID) },
				func(x *domain.User) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.User) error { return i.store.UpdateUser(ctx, x) },
				func(ctx context.Context, x *domain.User) error { return i.store.CreateUser(ctx, x) },
			)
		},
	)
}

func (i *Importer) importProfiles(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/profiles.jsonl",
		"profiles",
		func(p *domain.UserProfile) string { return p.UserID },
		nil, // no soft-delete on profiles
		func(ctx context.Context, p *domain.UserProfile) persistOutcome {
			// Profiles use upsert semantics — store handles existing rows.
			if err := i.store.SaveUserProfile(ctx, p); err != nil {
				return persistOutcome{err: err}
			}
			return persistOutcome{}
		},
	)
}

func (i *Importer) importLibraries(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/libraries.jsonl",
		"libraries",
		func(l *domain.Library) string { return l.ID },
		nil, // libraries have no soft-delete
		func(ctx context.Context, l *domain.Library) persistOutcome {
			return upsertWithMerge(ctx, opts, l,
				func(ctx context.Context) (*domain.Library, error) { return i.store.GetLibrary(ctx, l.ID) },
				func(x *domain.Library) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.Library) error { return i.store.UpdateLibrary(ctx, x) },
				func(ctx context.Context, x *domain.Library) error { return i.store.CreateLibrary(ctx, x) },
			)
		},
	)
}

func (i *Importer) importContributors(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/contributors.jsonl",
		"contributors",
		func(c *domain.Contributor) string { return c.ID },
		func(c *domain.Contributor) bool { return c.IsDeleted() },
		func(ctx context.Context, c *domain.Contributor) persistOutcome {
			return upsertWithMerge(ctx, opts, c,
				func(ctx context.Context) (*domain.Contributor, error) { return i.store.GetContributor(ctx, c.ID) },
				func(x *domain.Contributor) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.Contributor) error { return i.store.UpdateContributor(ctx, x) },
				func(ctx context.Context, x *domain.Contributor) error { return i.store.CreateContributor(ctx, x) },
			)
		},
	)
}

func (i *Importer) importSeries(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/series.jsonl",
		"series",
		func(s *domain.Series) string { return s.ID },
		func(s *domain.Series) bool { return s.IsDeleted() },
		func(ctx context.Context, s *domain.Series) persistOutcome {
			return upsertWithMerge(ctx, opts, s,
				func(ctx context.Context) (*domain.Series, error) { return i.store.GetSeries(ctx, s.ID) },
				func(x *domain.Series) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.Series) error { return i.store.UpdateSeries(ctx, x) },
				func(ctx context.Context, x *domain.Series) error { return i.store.CreateSeries(ctx, x) },
			)
		},
	)
}

func (i *Importer) importTags(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/tags.jsonl",
		"tags",
		func(t *domain.Tag) string { return t.ID },
		nil, // tags have no soft-delete
		func(ctx context.Context, t *domain.Tag) persistOutcome {
			return upsertWithMerge(ctx, opts, t,
				func(ctx context.Context) (*domain.Tag, error) { return i.store.GetTagByIDForRestore(ctx, t.ID) },
				func(x *domain.Tag) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.Tag) error { return i.store.UpdateTagForRestore(ctx, x) },
				func(ctx context.Context, x *domain.Tag) error { return i.store.CreateTag(ctx, x) },
			)
		},
	)
}

func (i *Importer) importBooks(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/books.jsonl",
		"books",
		func(b *domain.Book) string { return b.ID },
		func(b *domain.Book) bool { return b.IsDeleted() },
		func(ctx context.Context, b *domain.Book) persistOutcome {
			return upsertWithMerge(ctx, opts, b,
				func(ctx context.Context) (*domain.Book, error) { return i.store.GetBookByID(ctx, b.ID) },
				func(x *domain.Book) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.Book) error { return i.store.UpdateBook(ctx, x) },
				func(ctx context.Context, x *domain.Book) error { return i.store.CreateBook(ctx, x) },
			)
		},
	)
}

func (i *Importer) importCollections(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/collections.jsonl",
		"collections",
		func(c *domain.Collection) string { return c.ID },
		nil, // collections have no soft-delete
		func(ctx context.Context, c *domain.Collection) persistOutcome {
			return upsertWithMerge(ctx, opts, c,
				func(ctx context.Context) (*domain.Collection, error) { return i.store.GetCollectionByID(ctx, c.ID) },
				func(x *domain.Collection) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.Collection) error { return i.store.AdminUpdateCollection(ctx, x) },
				func(ctx context.Context, x *domain.Collection) error { return i.store.CreateCollection(ctx, x) },
			)
		},
	)
}

func (i *Importer) importCollectionShares(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/collection_shares.jsonl",
		"collection_shares",
		func(s *domain.CollectionShare) string { return s.ID },
		func(s *domain.CollectionShare) bool { return s.IsDeleted() },
		func(ctx context.Context, s *domain.CollectionShare) persistOutcome {
			return upsertWithMerge(ctx, opts, s,
				func(ctx context.Context) (*domain.CollectionShare, error) { return i.store.GetShare(ctx, s.ID) },
				func(x *domain.CollectionShare) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.CollectionShare) error { return i.store.UpdateShare(ctx, x) },
				func(ctx context.Context, x *domain.CollectionShare) error { return i.store.CreateShare(ctx, x) },
			)
		},
	)
}

func (i *Importer) importShelves(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/shelves.jsonl",
		"shelves",
		func(s *domain.Shelf) string { return s.ID },
		nil, // shelves have no soft-delete
		func(ctx context.Context, s *domain.Shelf) persistOutcome {
			return upsertWithMerge(ctx, opts, s,
				func(ctx context.Context) (*domain.Shelf, error) { return i.store.GetShelf(ctx, s.ID) },
				func(x *domain.Shelf) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.Shelf) error { return i.store.UpdateShelf(ctx, x) },
				func(ctx context.Context, x *domain.Shelf) error { return i.store.CreateShelf(ctx, x) },
			)
		},
	)
}

func (i *Importer) importActivities(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"entities/activities.jsonl",
		"activities",
		nil, // activities are append-only; no useful pre-persist ID for errors
		nil, // no soft-delete; activities are imported unconditionally
		func(ctx context.Context, a *domain.Activity) persistOutcome {
			// Activities are immutable. Duplicate inserts are treated as
			// skips rather than errors so retried/overlapping backups don't
			// pollute the error list.
			if err := i.store.CreateActivity(ctx, a); err != nil {
				return persistOutcome{skipped: true}
			}
			return persistOutcome{}
		},
	)
}

func (i *Importer) importListeningEvents(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"listening/events.jsonl",
		"listening_events",
		nil, // events are append-only; duplicate-on-create is tolerated as skip
		nil,
		func(ctx context.Context, e *domain.ListeningEvent) persistOutcome {
			// Events are historical truth — duplicates aren't errors.
			if err := i.store.CreateListeningEvent(ctx, e); err != nil {
				return persistOutcome{skipped: true}
			}
			return persistOutcome{}
		},
	)
}

func (i *Importer) importReadingSessions(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (int, int, []RestoreError) {
	return importEntity(ctx, i, zr, opts,
		"listening/sessions.jsonl",
		"reading_sessions",
		func(s *domain.BookReadingSession) string { return s.ID },
		nil, // reading sessions have no soft-delete
		func(ctx context.Context, s *domain.BookReadingSession) persistOutcome {
			return upsertWithMerge(ctx, opts, s,
				func(ctx context.Context) (*domain.BookReadingSession, error) {
					return i.store.GetReadingSession(ctx, s.ID)
				},
				func(x *domain.BookReadingSession) time.Time { return x.UpdatedAt },
				func(ctx context.Context, x *domain.BookReadingSession) error {
					return i.store.UpdateReadingSession(ctx, x)
				},
				func(ctx context.Context, x *domain.BookReadingSession) error {
					return i.store.CreateReadingSession(ctx, x)
				},
			)
		},
	)
}

// importGenres is the lone non-JSONL importer: genres are stored as a single
// JSON array in entities/genres.json (legacy export format). The streaming
// helper expects line-delimited JSON, so genres get a small bespoke loader
// that decodes the array up front and then funnels through the same persist
// pipeline as everything else.
func (i *Importer) importGenres(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/genres.json")
	if err != nil {
		if errors.Is(err, stream.ErrFileNotFound) {
			return 0, 0, nil
		}
		return 0, 0, []RestoreError{{EntityType: "genres", Error: err.Error()}}
	}
	defer rc.Close()

	var genres []*domain.Genre
	if err := json.UnmarshalRead(rc, &genres); err != nil {
		return 0, 0, []RestoreError{{EntityType: "genres", Error: fmt.Sprintf("parse error: %v", err)}}
	}

	persist := func(ctx context.Context, g *domain.Genre) persistOutcome {
		return upsertWithMerge(ctx, opts, g,
			func(ctx context.Context) (*domain.Genre, error) { return i.store.GetGenre(ctx, g.ID) },
			func(x *domain.Genre) time.Time { return x.UpdatedAt },
			func(ctx context.Context, x *domain.Genre) error { return i.store.UpdateGenre(ctx, x) },
			func(ctx context.Context, x *domain.Genre) error { return i.store.CreateGenre(ctx, x) },
		)
	}

	for _, genre := range genres {
		if opts.Mode == RestoreModeMerge && genre.IsDeleted() {
			skipped++
			continue
		}
		if opts.DryRun {
			imported++
			continue
		}
		outcome := persist(ctx, genre)
		switch {
		case outcome.err != nil:
			errs = append(errs, RestoreError{
				EntityType: "genres",
				EntityID:   genre.ID,
				Error:      outcome.err.Error(),
			})
		case outcome.skipped:
			skipped++
		default:
			imported++
		}
	}

	return imported, skipped, errs
}

// importServer restores instance and server settings. Unlike the per-entity
// importers it deals with a single server.json document, has its own error
// shape, and runs only in full-mode restores — so it stays as a one-off.
func (i *Importer) importServer(ctx context.Context, zr *zip.ReadCloser) error {
	rc, err := stream.OpenFile(zr, "server.json")
	if err != nil {
		return err
	}
	defer rc.Close()

	var server export.ServerExport
	if err := json.UnmarshalRead(rc, &server); err != nil {
		return err
	}

	//nolint:nestif // Recovery cascade: update -> create-on-missing -> retry update; flatter is clearer.
	if server.Instance != nil {
		// Try to update existing instance, if not found create new one
		if err := i.store.UpdateInstance(ctx, server.Instance); err != nil {
			if errors.Is(err, store.ErrServerNotFound) {
				// Instance was cleared, create a new one first then update
				if _, err := i.store.CreateInstance(ctx); err != nil {
					return fmt.Errorf("create instance: %w", err)
				}
				// Now update with the backup data
				if err := i.store.UpdateInstance(ctx, server.Instance); err != nil {
					return fmt.Errorf("update instance: %w", err)
				}
			} else {
				return err
			}
		}
	}

	if server.Settings != nil {
		if err := i.store.UpdateServerSettings(ctx, server.Settings); err != nil {
			return err
		}
	}

	return nil
}
