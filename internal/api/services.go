package api

import (
	"github.com/listenupapp/listenup-server/internal/media/images"
	"github.com/listenupapp/listenup-server/internal/service"
)

// Services groups all business logic services used by the API server.
// This reduces the parameter count for NewServer and improves testability.
type Services struct {
	Instance       *service.InstanceService
	Auth           *service.AuthService
	Book           *service.BookService
	Collection     *service.CollectionService
	Sharing        *service.SharingService
	Sync           *service.SyncService
	Listening      *service.ListeningService
	Stats          *service.StatsService
	Genre          *service.GenreService
	Tag            *service.TagService
	Search         *service.SearchService
	Invite         *service.InviteService
	Admin          *service.AdminService
	Transcode      *service.TranscodeService
	Metadata       *service.MetadataService       // Audible metadata fetching
	Chapter        *service.ChapterService        // Chapter name alignment
	Cover          *service.CoverService          // Multi-source cover search and download
	Lens           *service.LensService           // Personal curation lenses
	Inbox          *service.InboxService          // Inbox staging workflow
	Settings       *service.SettingsService       // Server-wide settings
	Social         *service.SocialService         // Social features (leaderboards)
	ReadingSession *service.ReadingSessionService // Reading session tracking
	Activity       *service.ActivityService       // Activity feed
}

// StorageServices groups file storage handlers used by the API server.
type StorageServices struct {
	Covers            *images.Storage // Book cover images
	ContributorImages *images.Storage // Contributor profile photos
	SeriesCovers      *images.Storage // Series cover images
}
