// Package export provides backup export functionality.
package export

import (
	"archive/zip"
	"context"

	"encoding/json/v2"

	"github.com/listenupapp/listenup-server/internal/backup/stream"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// UserExport is the backup-safe representation of a user.
// Excludes sessions/tokens, keeps password hash for restore.
type UserExport struct {
	domain.Syncable
	Email        string                 `json:"email"`
	PasswordHash string                 `json:"password_hash"`
	IsRoot       bool                   `json:"is_root"`
	Role         domain.Role            `json:"role"`
	Status       domain.UserStatus      `json:"status,omitempty"`
	DisplayName  string                 `json:"display_name"`
	FirstName    string                 `json:"first_name"`
	LastName     string                 `json:"last_name"`
	Permissions  domain.UserPermissions `json:"permissions"`
	InvitedBy    string                 `json:"invited_by,omitempty"`
	ApprovedBy   string                 `json:"approved_by,omitempty"`
}

// ServerExport contains server identity and settings.
type ServerExport struct {
	Instance *domain.Instance       `json:"instance"`
	Settings *domain.ServerSettings `json:"settings,omitempty"`
}

func exportUsers(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/users.jsonl")
	if err != nil {
		return 0, err
	}

	users, err := s.ListAllUsers(ctx)
	if err != nil {
		return 0, err
	}

	for _, user := range users {
		export := UserExport{
			Syncable:     user.Syncable,
			Email:        user.Email,
			PasswordHash: user.PasswordHash,
			IsRoot:       user.IsRoot,
			Role:         user.Role,
			Status:       user.Status,
			DisplayName:  user.DisplayName,
			FirstName:    user.FirstName,
			LastName:     user.LastName,
			Permissions:  user.Permissions,
			InvitedBy:    user.InvitedBy,
			ApprovedBy:   user.ApprovedBy,
		}

		if err := w.Write(export); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportProfiles(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/profiles.jsonl")
	if err != nil {
		return 0, err
	}

	for profile, err := range s.StreamProfiles(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(profile); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportLibraries(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/libraries.jsonl")
	if err != nil {
		return 0, err
	}

	libraries, err := s.ListLibraries(ctx)
	if err != nil {
		return 0, err
	}

	for _, lib := range libraries {
		if err := w.Write(lib); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportBooks(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/books.jsonl")
	if err != nil {
		return 0, err
	}

	for book, err := range s.StreamBooks(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(book); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportContributors(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/contributors.jsonl")
	if err != nil {
		return 0, err
	}

	for contrib, err := range s.StreamContributors(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(contrib); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportSeries(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/series.jsonl")
	if err != nil {
		return 0, err
	}

	for series, err := range s.StreamSeries(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(series); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

// exportGenres exports as single JSON (hierarchical structure).
func exportGenres(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	genres, err := s.ListGenres(ctx)
	if err != nil {
		return 0, err
	}

	w, err := zw.Create("entities/genres.json")
	if err != nil {
		return 0, err
	}

	if err := json.MarshalWrite(w, genres); err != nil {
		return 0, err
	}

	return len(genres), nil
}

func exportTags(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/tags.jsonl")
	if err != nil {
		return 0, err
	}

	tags, err := s.ListTags(ctx)
	if err != nil {
		return 0, err
	}

	for _, tag := range tags {
		if err := w.Write(tag); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportCollections(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/collections.jsonl")
	if err != nil {
		return 0, err
	}

	for coll, err := range s.StreamCollections(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(coll); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportCollectionShares(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/collection_shares.jsonl")
	if err != nil {
		return 0, err
	}

	for share, err := range s.StreamCollectionShares(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(share); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportShelves(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/shelves.jsonl")
	if err != nil {
		return 0, err
	}

	for shelf, err := range s.StreamShelves(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(shelf); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportActivities(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "entities/activities.jsonl")
	if err != nil {
		return 0, err
	}

	for activity, err := range s.StreamActivities(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(activity); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportListeningEvents(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "listening/events.jsonl")
	if err != nil {
		return 0, err
	}

	for event, err := range s.StreamListeningEvents(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(event); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportReadingSessions(ctx context.Context, s store.Store, zw *zip.Writer) (int, error) {
	w, err := stream.NewWriter(zw, "listening/sessions.jsonl")
	if err != nil {
		return 0, err
	}

	for session, err := range s.ListAllSessions(ctx) {
		if err != nil {
			return w.Count(), err
		}
		if err := w.Write(session); err != nil {
			return w.Count(), err
		}
	}

	return w.Count(), nil
}

func exportServer(ctx context.Context, s store.Store, zw *zip.Writer, m *Manifest) error {
	instance, err := s.GetInstance(ctx)
	if err != nil {
		return err
	}

	settings, _ := s.GetServerSettings(ctx) // OK if not found

	// Populate manifest identity
	m.ServerID = instance.ID
	m.ServerName = instance.Name

	// Write server.json
	w, err := zw.Create("server.json")
	if err != nil {
		return err
	}

	export := ServerExport{
		Instance: instance,
		Settings: settings,
	}

	return json.MarshalWrite(w, export)
}
