package api

import (
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/service"
)

// Services groups all business logic services used by the API server.
// This reduces the parameter count for NewServer and improves testability.
type Services struct {
	Instance   *service.InstanceService
	Auth       *service.AuthService
	Book       *service.BookService
	Collection *service.CollectionService
	Sharing    *service.SharingService
	Sync       *service.SyncService
	Listening  *service.ListeningService
	Genre      *service.GenreService
	Tag        *service.TagService
	Search     *service.SearchService
}

// StorageServices groups file storage handlers used by the API server.
type StorageServices struct {
	Covers            *images.Storage // Book cover images
	ContributorImages *images.Storage // Contributor profile photos
}
