package backupimport

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"

	"encoding/json/v2"

	"github.com/listenupapp/listenup-server/internal/backup/export"
	"github.com/listenupapp/listenup-server/internal/backup/stream"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (i *Importer) importUsers(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/users.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "users", Error: err.Error()})
		return
	}

	reader := stream.NewReader[export.UserExport](rc)

	for userExport, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "users",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		// Convert export to domain
		user := &domain.User{
			Syncable:     userExport.Syncable,
			Email:        userExport.Email,
			PasswordHash: userExport.PasswordHash,
			IsRoot:       userExport.IsRoot,
			Role:         userExport.Role,
			Status:       userExport.Status,
			DisplayName:  userExport.DisplayName,
			FirstName:    userExport.FirstName,
			LastName:     userExport.LastName,
			Permissions:  userExport.Permissions,
			InvitedBy:    userExport.InvitedBy,
			ApprovedBy:   userExport.ApprovedBy,
		}

		// Merge mode: skip soft-deleted
		if opts.Mode == RestoreModeMerge && user.IsDeleted() {
			skipped++
			continue
		}

		if opts.DryRun {
			imported++
			continue
		}

		// Check for existing
		existing, _ := i.store.Users.Get(ctx, user.ID)

		if existing != nil {
			// Entity exists - handle based on mode
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !user.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				// MergeKeepBackup falls through to update
				}
			}

			// Update existing
			if err := i.store.Users.Update(ctx, user.ID, user); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "users",
					EntityID:   user.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue // Critical: don't fall through to Create
		}

		// Create new
		if err := i.store.Users.Create(ctx, user.ID, user); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "users",
				EntityID:   user.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importProfiles(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/profiles.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "profiles", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.UserProfile](rc)

	for profile, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "profiles",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		if opts.DryRun {
			imported++
			continue
		}

		// Profiles use upsert semantics
		if err := i.store.SaveUserProfile(ctx, &profile); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "profiles",
				EntityID:   profile.UserID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importLibraries(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/libraries.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "libraries", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.Library](rc)

	for lib, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "libraries",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		// Library doesn't have soft-delete

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.GetLibrary(ctx, lib.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !lib.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.UpdateLibrary(ctx, &lib); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "libraries",
					EntityID:   lib.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CreateLibrary(ctx, &lib); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "libraries",
				EntityID:   lib.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importContributors(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/contributors.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "contributors", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.Contributor](rc)

	for contrib, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "contributors",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		if opts.Mode == RestoreModeMerge && contrib.IsDeleted() {
			skipped++
			continue
		}

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.GetContributor(ctx, contrib.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !contrib.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.UpdateContributor(ctx, &contrib); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "contributors",
					EntityID:   contrib.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CreateContributor(ctx, &contrib); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "contributors",
				EntityID:   contrib.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importSeries(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/series.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "series", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.Series](rc)

	for series, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "series",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		if opts.Mode == RestoreModeMerge && series.IsDeleted() {
			skipped++
			continue
		}

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.GetSeries(ctx, series.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !series.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.UpdateSeries(ctx, &series); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "series",
					EntityID:   series.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CreateSeries(ctx, &series); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "series",
				EntityID:   series.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importGenres(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/genres.json")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "genres", Error: err.Error()})
		return
	}
	defer rc.Close()

	var genres []*domain.Genre
	if err := json.UnmarshalRead(rc, &genres); err != nil {
		errs = append(errs, RestoreError{
			EntityType: "genres",
			Error:      fmt.Sprintf("parse error: %v", err),
		})
		return
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

		existing, _ := i.store.GetGenre(ctx, genre.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !genre.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.UpdateGenre(ctx, genre); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "genres",
					EntityID:   genre.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CreateGenre(ctx, genre); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "genres",
				EntityID:   genre.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importTags(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/tags.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "tags", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.Tag](rc)

	for tag, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "tags",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		// Tag doesn't have soft-delete

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.GetTagByIDForRestore(ctx, tag.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !tag.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.UpdateTagForRestore(ctx, &tag); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "tags",
					EntityID:   tag.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CreateTag(ctx, &tag); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "tags",
				EntityID:   tag.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importBooks(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/books.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "books", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.Book](rc)

	for book, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "books",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		if opts.Mode == RestoreModeMerge && book.IsDeleted() {
			skipped++
			continue
		}

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.GetBookNoAccessCheck(ctx, book.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !book.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.UpdateBook(ctx, &book); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "books",
					EntityID:   book.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CreateBook(ctx, &book); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "books",
				EntityID:   book.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importCollections(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/collections.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "collections", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.Collection](rc)

	for coll, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "collections",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		// Collection doesn't have soft-delete

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.GetCollectionNoAccessCheck(ctx, coll.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !coll.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.UpdateCollectionNoAccessCheck(ctx, &coll); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "collections",
					EntityID:   coll.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CreateCollection(ctx, &coll); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "collections",
				EntityID:   coll.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importCollectionShares(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/collection_shares.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "collection_shares", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.CollectionShare](rc)

	for share, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "collection_shares",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		if opts.Mode == RestoreModeMerge && share.IsDeleted() {
			skipped++
			continue
		}

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.CollectionShares.Get(ctx, share.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !share.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.CollectionShares.Update(ctx, share.ID, &share); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "collection_shares",
					EntityID:   share.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CollectionShares.Create(ctx, share.ID, &share); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "collection_shares",
				EntityID:   share.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importLenses(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/lenses.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "lenses", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.Lens](rc)

	for lens, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "lenses",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		// Lens doesn't have soft-delete

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.GetLens(ctx, lens.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !lens.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.UpdateLens(ctx, &lens); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "lenses",
					EntityID:   lens.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.CreateLens(ctx, &lens); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "lenses",
				EntityID:   lens.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importActivities(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "entities/activities.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "activities", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.Activity](rc)

	for activity, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "activities",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		// Activities are typically not merged - just import unconditionally
		if opts.DryRun {
			imported++
			continue
		}

		// Create activity - duplicates will fail silently
		if err := i.store.CreateActivity(ctx, &activity); err != nil {
			// Don't count as error - activity might already exist
			skipped++
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importListeningEvents(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "listening/events.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "listening_events", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.ListeningEvent](rc)

	for event, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "listening_events",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		// Events are immutable and imported unconditionally (they're historical truth)
		if opts.DryRun {
			imported++
			continue
		}

		if err := i.store.CreateListeningEvent(ctx, &event); err != nil {
			// Don't fail on duplicates - event might already exist
			skipped++
			continue
		}
		imported++
	}

	return
}

func (i *Importer) importReadingSessions(ctx context.Context, zr *zip.ReadCloser, opts RestoreOptions) (imported, skipped int, errs []RestoreError) {
	rc, err := stream.OpenFile(zr, "listening/sessions.jsonl")
	if err != nil {
		if err == stream.ErrFileNotFound {
			return 0, 0, nil
		}
		errs = append(errs, RestoreError{EntityType: "reading_sessions", Error: err.Error()})
		return
	}

	reader := stream.NewReader[domain.BookReadingSession](rc)

	for session, err := range reader.All() {
		if err != nil {
			errs = append(errs, RestoreError{
				EntityType: "reading_sessions",
				Error:      fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		if opts.DryRun {
			imported++
			continue
		}

		existing, _ := i.store.Sessions.Get(ctx, session.ID)
		if existing != nil {
			if opts.Mode == RestoreModeMerge {
				switch opts.MergeStrategy {
				case MergeKeepLocal:
					skipped++
					continue
				case MergeNewest:
					if !session.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						continue
					}
				}
			}
			if err := i.store.Sessions.Update(ctx, session.ID, &session); err != nil {
				errs = append(errs, RestoreError{
					EntityType: "reading_sessions",
					EntityID:   session.ID,
					Error:      err.Error(),
				})
				continue
			}
			imported++
			continue
		}

		if err := i.store.Sessions.Create(ctx, session.ID, &session); err != nil {
			errs = append(errs, RestoreError{
				EntityType: "reading_sessions",
				EntityID:   session.ID,
				Error:      err.Error(),
			})
			continue
		}
		imported++
	}

	return
}

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
