// Package store defines the persistence interface for the ListenUp server.
package store

import (
	"context"
	"iter"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/metadata/audible"
)

// Store defines the interface for all persistence operations.
type Store interface {
	// Lifecycle
	Close() error
	SetSearchIndexer(indexer SearchIndexer)
	SetTranscodeDeleter(deleter TranscodeDeleter)
	SetBulkMode(enabled bool)
	IsBulkMode() bool
	InvalidateGenreCache()

	// Users
	CreateUser(ctx context.Context, user *domain.User) error
	GetUser(ctx context.Context, id string) (*domain.User, error)
	GetUsersByIDs(ctx context.Context, ids []string) ([]*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	UpdateUser(ctx context.Context, user *domain.User) error
	ListUsers(ctx context.Context) ([]*domain.User, error)
	ListAllUsers(ctx context.Context) ([]*domain.User, error)
	ListPendingUsers(ctx context.Context) ([]*domain.User, error)
	BroadcastUserPending(user *domain.User)
	BroadcastUserApproved(user *domain.User)
	BroadcastUserDeleted(userID, reason string)

	// Auth Sessions
	CreateSession(ctx context.Context, session *domain.Session) error
	GetSession(ctx context.Context, id string) (*domain.Session, error)
	GetSessionByRefreshToken(ctx context.Context, tokenHash string) (*domain.Session, error)
	UpdateSession(ctx context.Context, session *domain.Session) error
	DeleteSession(ctx context.Context, id string) error
	ListUserSessions(ctx context.Context, userID string) ([]*domain.Session, error)
	DeleteAllUserSessions(ctx context.Context, userID string) error
	DeleteExpiredSessions(ctx context.Context) (int, error)

	// User Profiles
	GetUserProfile(ctx context.Context, userID string) (*domain.UserProfile, error)
	GetUserProfilesByIDs(ctx context.Context, userIDs []string) (map[string]*domain.UserProfile, error)
	SaveUserProfile(ctx context.Context, profile *domain.UserProfile) error
	DeleteUserProfile(ctx context.Context, userID string) error

	// User Settings
	GetUserSettings(ctx context.Context, userID string) (*domain.UserSettings, error)
	UpsertUserSettings(ctx context.Context, settings *domain.UserSettings) error
	DeleteUserSettings(ctx context.Context, userID string) error
	GetOrCreateUserSettings(ctx context.Context, userID string) (*domain.UserSettings, error)

	// Books
	CreateBook(ctx context.Context, book *domain.Book) error
	GetBook(ctx context.Context, id string, userID string) (*domain.Book, error)
	GetBookNoAccessCheck(ctx context.Context, id string) (*domain.Book, error)
	GetBookByPath(ctx context.Context, path string) (*domain.Book, error)
	GetBookByInode(ctx context.Context, inode int64) (*domain.Book, error)
	GetBookByASIN(ctx context.Context, asin string) (*domain.Book, error)
	GetBookByISBN(ctx context.Context, isbn string) (*domain.Book, error)
	BookExists(ctx context.Context, id string) (bool, error)
	UpdateBook(ctx context.Context, book *domain.Book) error
	DeleteBook(ctx context.Context, id string) error
	ListBooks(ctx context.Context, params PaginationParams) (*PaginatedResult[*domain.Book], error)
	ListAllBooks(ctx context.Context) ([]*domain.Book, error)
	CountBooks(ctx context.Context) (int, error)
	GetAllBookIDs(ctx context.Context) ([]string, error)
	GetBooksByCollectionPaginated(ctx context.Context, userID, collectionID string, params PaginationParams) (*PaginatedResult[*domain.Book], error)
	GetBooksDeletedAfter(ctx context.Context, timestamp time.Time) ([]string, error)
	SearchBooksByTitle(ctx context.Context, title string) ([]*domain.Book, error)
	TouchEntity(ctx context.Context, entityType, id string) error
	EnrichBook(ctx context.Context, book *domain.Book) (*dto.Book, error)
	SetBookContributors(ctx context.Context, bookID string, contributors []ContributorInput) (*domain.Book, error)
	SetBookSeries(ctx context.Context, bookID string, seriesInputs []SeriesInput) (*domain.Book, error)
	SetBookGenres(ctx context.Context, bookID string, genreIDs []string) error
	BroadcastBookCreated(ctx context.Context, book *domain.Book) error

	// Libraries
	CreateLibrary(ctx context.Context, lib *domain.Library) error
	GetLibrary(ctx context.Context, id string) (*domain.Library, error)
	GetDefaultLibrary(ctx context.Context) (*domain.Library, error)
	UpdateLibrary(ctx context.Context, lib *domain.Library) error
	DeleteLibrary(ctx context.Context, id string) error
	ListLibraries(ctx context.Context) ([]*domain.Library, error)
	EnsureLibrary(ctx context.Context, scanPath string, userID string) (*BootstrapResult, error)

	// Collections
	CreateCollection(ctx context.Context, coll *domain.Collection) error
	GetCollection(ctx context.Context, id string, userID string) (*domain.Collection, error)
	UpdateCollection(ctx context.Context, coll *domain.Collection, userID string) error
	DeleteCollection(ctx context.Context, id string, userID string) error
	ListCollectionsByLibrary(ctx context.Context, libraryID string, userID string) ([]*domain.Collection, error)
	ListAllCollectionsByLibrary(ctx context.Context, libraryID string) ([]*domain.Collection, error)
	GetInboxForLibrary(ctx context.Context, libraryID string) (*domain.Collection, error)
	AddBookToCollection(ctx context.Context, bookID, collectionID string, userID string) error
	RemoveBookFromCollection(ctx context.Context, bookID, collectionID string, userID string) error
	GetCollectionsForBook(ctx context.Context, bookID string) ([]*domain.Collection, error)
	AdminGetCollection(ctx context.Context, id string) (*domain.Collection, error)
	AdminListAllCollections(ctx context.Context) ([]*domain.Collection, error)
	AdminUpdateCollection(ctx context.Context, coll *domain.Collection) error
	AdminDeleteCollection(ctx context.Context, id string) error
	AdminAddBookToCollection(ctx context.Context, bookID, collectionID string) error
	AdminRemoveBookFromCollection(ctx context.Context, bookID, collectionID string) error
	EnsureGlobalAccessCollection(ctx context.Context, libraryID, ownerID string) (*domain.Collection, error)

	// Collection Access
	GetCollectionsForUser(ctx context.Context, userID string) ([]*domain.Collection, error)
	GetCollectionsContainingBook(ctx context.Context, bookID string) ([]*domain.Collection, error)
	GetBooksForUser(ctx context.Context, userID string) ([]*domain.Book, error)
	GetBooksForUserUpdatedAfter(ctx context.Context, userID string, timestamp time.Time) ([]*domain.Book, error)
	CanUserAccessBook(ctx context.Context, userID, bookID string) (bool, error)
	CanUserAccessCollection(ctx context.Context, userID, collectionID string) (bool, domain.SharePermission, bool, error)
	GetAccessibleBookIDSet(ctx context.Context, userID string) (map[string]bool, error)

	// Collection Shares
	CreateShare(ctx context.Context, share *domain.CollectionShare) error
	GetShare(ctx context.Context, id string) (*domain.CollectionShare, error)
	GetShareForUserAndCollection(ctx context.Context, userID, collectionID string) (*domain.CollectionShare, error)
	GetSharesForUser(ctx context.Context, userID string) ([]*domain.CollectionShare, error)
	GetSharesForCollection(ctx context.Context, collectionID string) ([]*domain.CollectionShare, error)
	DeleteShare(ctx context.Context, id string) error
	UpdateShare(ctx context.Context, share *domain.CollectionShare) error
	DeleteSharesForCollection(ctx context.Context, collectionID string) error

	// Contributors
	CreateContributor(ctx context.Context, contributor *domain.Contributor) error
	GetContributor(ctx context.Context, id string) (*domain.Contributor, error)
	GetContributorByASIN(ctx context.Context, asin string) (*domain.Contributor, error)
	GetContributorsByIDs(ctx context.Context, ids []string) ([]*domain.Contributor, error)
	GetOrCreateContributorByName(ctx context.Context, name string) (*domain.Contributor, error)
	GetOrCreateContributorByNameWithAlias(ctx context.Context, name string) (*domain.Contributor, bool, error)
	UpdateContributor(ctx context.Context, contributor *domain.Contributor) error
	DeleteContributor(ctx context.Context, id string) error
	ListContributors(ctx context.Context, params PaginationParams) (*PaginatedResult[*domain.Contributor], error)
	ListAllContributors(ctx context.Context) ([]*domain.Contributor, error)
	CountContributors(ctx context.Context) (int, error)
	CountBooksForContributor(ctx context.Context, contributorID string) (int, error)
	CountBooksForAllContributors(ctx context.Context) (map[string]int, error)
	GetBooksByContributor(ctx context.Context, contributorID string) ([]*domain.Book, error)
	GetBooksByContributorRole(ctx context.Context, contributorID string, role domain.ContributorRole) ([]*domain.Book, error)
	SearchContributorsByName(ctx context.Context, query string, limit int) ([]*domain.Contributor, error)
	GetContributorsUpdatedAfter(ctx context.Context, timestamp time.Time) ([]*domain.Contributor, error)
	GetContributorsDeletedAfter(ctx context.Context, timestamp time.Time) ([]string, error)
	MergeContributors(ctx context.Context, sourceID, targetID string) (*domain.Contributor, error)
	UnmergeContributor(ctx context.Context, sourceID, aliasName string) (*domain.Contributor, error)
	GetContributorBookIDMap(ctx context.Context) (map[string][]string, error)
	GetBookIDsByContributor(ctx context.Context, contributorID string) ([]string, error)

	// Series
	CreateSeries(ctx context.Context, series *domain.Series) error
	GetSeries(ctx context.Context, id string) (*domain.Series, error)
	GetSeriesByIDs(ctx context.Context, ids []string) ([]*domain.Series, error)
	GetSeriesByASIN(ctx context.Context, asin string) (*domain.Series, error)
	GetOrCreateSeriesByName(ctx context.Context, name string) (*domain.Series, error)
	UpdateSeries(ctx context.Context, series *domain.Series) error
	DeleteSeries(ctx context.Context, id string) error
	ListSeries(ctx context.Context, params PaginationParams) (*PaginatedResult[*domain.Series], error)
	ListAllSeries(ctx context.Context) ([]*domain.Series, error)
	CountSeries(ctx context.Context) (int, error)
	CountBooksInSeries(ctx context.Context, seriesID string) (int, error)
	CountBooksForMultipleSeries(ctx context.Context, seriesIDs []string) (map[string]int, error)
	GetBooksBySeries(ctx context.Context, seriesID string) ([]*domain.Book, error)
	GetBookIDsBySeries(ctx context.Context, seriesID string) ([]string, error)
	GetSeriesUpdatedAfter(ctx context.Context, timestamp time.Time) ([]*domain.Series, error)
	GetSeriesDeletedAfter(ctx context.Context, timestamp time.Time) ([]string, error)
	GetSeriesBookIDMap(ctx context.Context) (map[string][]string, error)

	// Genres
	CreateGenre(ctx context.Context, g *domain.Genre) error
	GetGenre(ctx context.Context, id string) (*domain.Genre, error)
	GetGenresByIDs(ctx context.Context, ids []string) ([]*domain.Genre, error)
	GetGenreBySlug(ctx context.Context, slug string) (*domain.Genre, error)
	GetOrCreateGenreBySlug(ctx context.Context, slug, name, parentID string) (*domain.Genre, error)
	ListGenres(ctx context.Context) ([]*domain.Genre, error)
	GetGenreChildren(ctx context.Context, parentID string) ([]*domain.Genre, error)
	UpdateGenre(ctx context.Context, g *domain.Genre) error
	DeleteGenre(ctx context.Context, id string) error
	MoveGenre(ctx context.Context, genreID, newParentID string) error
	MergeGenres(ctx context.Context, sourceID, targetID string) error
	AddBookGenre(ctx context.Context, bookID, genreID string) error
	RemoveBookGenre(ctx context.Context, bookID, genreID string) error
	GetGenreIDsForBook(ctx context.Context, bookID string) ([]string, error)
	GetBookIDsForGenre(ctx context.Context, genreID string) ([]string, error)
	GetBookIDsForGenreTree(ctx context.Context, genreID string) ([]string, error)
	CreateGenreAlias(ctx context.Context, alias *domain.GenreAlias) error
	GetGenreAliasByRaw(ctx context.Context, raw string) (*domain.GenreAlias, error)
	TrackUnmappedGenre(ctx context.Context, raw string, bookID string) error
	ListUnmappedGenres(ctx context.Context) ([]*domain.UnmappedGenre, error)
	ResolveUnmappedGenre(ctx context.Context, raw string, genreIDs []string, userID string) error
	SeedDefaultGenres(ctx context.Context) error

	// Tags
	CreateTag(ctx context.Context, t *domain.Tag) error
	GetTagByID(ctx context.Context, tagID string) (*domain.Tag, error)
	GetTagBySlug(ctx context.Context, slug string) (*domain.Tag, error)
	ListTags(ctx context.Context) ([]*domain.Tag, error)
	DeleteTag(ctx context.Context, tagID string) error
	FindOrCreateTagBySlug(ctx context.Context, slug string) (*domain.Tag, bool, error)
	AddTagToBook(ctx context.Context, bookID, tagID string) error
	RemoveTagFromBook(ctx context.Context, bookID, tagID string) error
	GetTagsForBook(ctx context.Context, bookID string) ([]*domain.Tag, error)
	GetTagsForBookIDs(ctx context.Context, bookIDs []string) (map[string][]*domain.Tag, error)
	GetTagIDsForBook(ctx context.Context, bookID string) ([]string, error)
	GetBookIDsForTag(ctx context.Context, tagID string) ([]string, error)
	CleanupTagsForDeletedBook(ctx context.Context, bookID string) error
	RecalculateTagBookCount(ctx context.Context, tagID string) error
	GetTagSlugsForBook(ctx context.Context, bookID string) ([]string, error)

	// Shelves (née Lenses)
	CreateLens(ctx context.Context, lens *domain.Shelf) error
	GetLens(ctx context.Context, id string) (*domain.Shelf, error)
	UpdateLens(ctx context.Context, lens *domain.Shelf) error
	DeleteLens(ctx context.Context, id string) error
	ListLensesByOwner(ctx context.Context, ownerID string) ([]*domain.Shelf, error)
	ListAllLenses(ctx context.Context) ([]*domain.Shelf, error)
	GetLensesContainingBook(ctx context.Context, bookID string) ([]*domain.Shelf, error)
	AddBookToLens(ctx context.Context, lensID, bookID string) error
	RemoveBookFromLens(ctx context.Context, lensID, bookID string) error
	DeleteLensesForUser(ctx context.Context, userID string) error
	// Shelf-named aliases (new names from Lens→Shelf rename)
	CreateShelf(ctx context.Context, shelf *domain.Shelf) error
	GetShelf(ctx context.Context, id string) (*domain.Shelf, error)
	UpdateShelf(ctx context.Context, shelf *domain.Shelf) error
	DeleteShelf(ctx context.Context, id string) error
	ListShelvesByOwner(ctx context.Context, ownerID string) ([]*domain.Shelf, error)
	ListAllShelves(ctx context.Context) ([]*domain.Shelf, error)
	GetShelvesContainingBook(ctx context.Context, bookID string) ([]*domain.Shelf, error)
	AddBookToShelf(ctx context.Context, shelfID, bookID string) error
	RemoveBookFromShelf(ctx context.Context, shelfID, bookID string) error
	DeleteShelvesForUser(ctx context.Context, userID string) error

	// Listening
	CreateListeningEvent(ctx context.Context, event *domain.ListeningEvent) error
	GetListeningEvent(ctx context.Context, id string) (*domain.ListeningEvent, error)
	GetEventsForUser(ctx context.Context, userID string) ([]*domain.ListeningEvent, error)
	GetEventsForBook(ctx context.Context, bookID string) ([]*domain.ListeningEvent, error)
	GetEventsForUserBook(ctx context.Context, userID, bookID string) ([]*domain.ListeningEvent, error)
	GetEventsForUserInRange(ctx context.Context, userID string, start, end time.Time) ([]*domain.ListeningEvent, error)
	DeleteEventsForUserBook(ctx context.Context, userID, bookID string) error
	GetState(ctx context.Context, userID, bookID string) (*domain.PlaybackState, error)
	UpsertState(ctx context.Context, progress *domain.PlaybackState) error
	DeleteState(ctx context.Context, userID, bookID string) error
	GetStateForUser(ctx context.Context, userID string) ([]*domain.PlaybackState, error)
	GetStateFinishedInRange(ctx context.Context, userID string, start, end time.Time) ([]*domain.PlaybackState, error)
	GetContinueListening(ctx context.Context, userID string, limit int) ([]*domain.PlaybackState, error)

	// Book Preferences
	GetBookPreferences(ctx context.Context, userID, bookID string) (*domain.BookPreferences, error)
	UpsertBookPreferences(ctx context.Context, prefs *domain.BookPreferences) error
	DeleteBookPreferences(ctx context.Context, userID, bookID string) error
	GetAllBookPreferences(ctx context.Context, userID string) ([]*domain.BookPreferences, error)

	// Reading Sessions
	CreateReadingSession(ctx context.Context, session *domain.BookReadingSession) error
	GetReadingSession(ctx context.Context, id string) (*domain.BookReadingSession, error)
	UpdateReadingSession(ctx context.Context, session *domain.BookReadingSession) error
	DeleteReadingSession(ctx context.Context, id string) error
	GetActiveSession(ctx context.Context, userID, bookID string) (*domain.BookReadingSession, error)
	GetUserReadingSessions(ctx context.Context, userID string, limit int) ([]*domain.BookReadingSession, error)
	GetUserBookSessions(ctx context.Context, userID, bookID string) ([]*domain.BookReadingSession, error)
	GetBookSessions(ctx context.Context, bookID string) ([]*domain.BookReadingSession, error)
	GetAllActiveSessions(ctx context.Context) ([]*domain.BookReadingSession, error)
	CleanupStaleSessions(ctx context.Context, maxAge time.Duration) (int, error)
	ListAllSessions(ctx context.Context) iter.Seq2[*domain.BookReadingSession, error]

	// Activities
	CreateActivity(ctx context.Context, activity *domain.Activity) error
	GetActivity(ctx context.Context, id string) (*domain.Activity, error)
	GetActivitiesFeed(ctx context.Context, limit int, before *time.Time, beforeID string) ([]*domain.Activity, error)
	GetUserActivities(ctx context.Context, userID string, limit int) ([]*domain.Activity, error)
	GetBookActivities(ctx context.Context, bookID string, limit int) ([]*domain.Activity, error)
	GetUserMilestoneState(ctx context.Context, userID string) (*domain.UserMilestoneState, error)
	UpdateUserMilestoneState(ctx context.Context, userID string, streakDays, listenHours int) error

	// User Stats
	GetUserStats(ctx context.Context, userID string) (*domain.UserStats, error)
	GetAllUserStats(ctx context.Context) ([]*domain.UserStats, error)
	EnsureUserStats(ctx context.Context, userID string) error
	IncrementListenTime(ctx context.Context, userID string, deltaMs int64) error
	IncrementBooksFinishedAtomic(ctx context.Context, userID string, delta int) error
	UpdateUserStreak(ctx context.Context, userID string, currentStreak, longestStreak int, lastListenedDate string) error
	UpdateUserStatsLastListened(ctx context.Context, userID string, date string) error
	UpdateUserStatsFromEvent(ctx context.Context, userID string, deltaMs int64, lastListenedDate string) error
	SetUserStats(ctx context.Context, stats *domain.UserStats) error
	ClearAllUserStats(ctx context.Context) error

	// Invites
	CreateInvite(ctx context.Context, invite *domain.Invite) error
	GetInvite(ctx context.Context, id string) (*domain.Invite, error)
	GetInviteByCode(ctx context.Context, code string) (*domain.Invite, error)
	UpdateInvite(ctx context.Context, invite *domain.Invite) error
	DeleteInvite(ctx context.Context, inviteID string) error
	ListInvites(ctx context.Context) ([]*domain.Invite, error)
	ListInvitesByCreator(ctx context.Context, creatorID string) ([]*domain.Invite, error)

	// Instance
	GetInstance(ctx context.Context) (*domain.Instance, error)
	CreateInstance(ctx context.Context) (*domain.Instance, error)
	UpdateInstance(ctx context.Context, instance *domain.Instance) error
	InitializeInstance(ctx context.Context) (*domain.Instance, error)

	// Server Settings
	GetServerSettings(ctx context.Context) (*domain.ServerSettings, error)
	UpdateServerSettings(ctx context.Context, settings *domain.ServerSettings) error

	// Metadata Caching
	GetCachedBook(ctx context.Context, region audible.Region, asin string) (*CachedBook, error)
	SetCachedBook(ctx context.Context, region audible.Region, asin string, book *audible.Book) error
	DeleteCachedBook(ctx context.Context, region audible.Region, asin string) error
	GetCachedChapters(ctx context.Context, region audible.Region, asin string) (*CachedChapters, error)
	SetCachedChapters(ctx context.Context, region audible.Region, asin string, chapters []audible.Chapter) error
	DeleteCachedChapters(ctx context.Context, region audible.Region, asin string) error
	GetCachedSearch(ctx context.Context, region audible.Region, query string) (*CachedSearch, error)
	SetCachedSearch(ctx context.Context, region audible.Region, query string, results []audible.SearchResult) error
	DeleteCachedSearch(ctx context.Context, region audible.Region, query string) error

	// Transcoding
	CreateTranscodeJob(ctx context.Context, job *domain.TranscodeJob) error
	GetTranscodeJob(ctx context.Context, id string) (*domain.TranscodeJob, error)
	UpdateTranscodeJob(ctx context.Context, job *domain.TranscodeJob) error
	DeleteTranscodeJob(ctx context.Context, id string) error
	GetTranscodeJobByAudioFile(ctx context.Context, audioFileID string) (*domain.TranscodeJob, error)
	GetTranscodeJobByAudioFileAndVariant(ctx context.Context, audioFileID string, variant domain.TranscodeVariant) (*domain.TranscodeJob, error)
	ListTranscodeJobsByBook(ctx context.Context, bookID string) ([]*domain.TranscodeJob, error)
	ListTranscodeJobsByStatus(ctx context.Context, status domain.TranscodeStatus) ([]*domain.TranscodeJob, error)
	ListPendingTranscodeJobs(ctx context.Context) ([]*domain.TranscodeJob, error)
	ListAllTranscodeJobs(ctx context.Context) iter.Seq2[*domain.TranscodeJob, error]
	DeleteTranscodeJobsByBook(ctx context.Context, bookID string) (int, error)

	// ABS Import
	CreateABSImport(ctx context.Context, imp *domain.ABSImport) error
	GetABSImport(ctx context.Context, id string) (*domain.ABSImport, error)
	ListABSImports(ctx context.Context) ([]*domain.ABSImport, error)
	UpdateABSImport(ctx context.Context, imp *domain.ABSImport) error
	DeleteABSImport(ctx context.Context, id string) error
	CreateABSImportUser(ctx context.Context, user *domain.ABSImportUser) error
	GetABSImportUser(ctx context.Context, importID, absUserID string) (*domain.ABSImportUser, error)
	ListABSImportUsers(ctx context.Context, importID string, filter domain.MappingFilter) ([]*domain.ABSImportUser, error)
	UpdateABSImportUserMapping(ctx context.Context, importID, absUserID string, listenUpID, luEmail, luDisplayName *string) error
	CreateABSImportBook(ctx context.Context, book *domain.ABSImportBook) error
	GetABSImportBook(ctx context.Context, importID, absMediaID string) (*domain.ABSImportBook, error)
	ListABSImportBooks(ctx context.Context, importID string, filter domain.MappingFilter) ([]*domain.ABSImportBook, error)
	UpdateABSImportBookMapping(ctx context.Context, importID, absMediaID string, listenUpID, luTitle, luAuthor *string) error
	CreateABSImportSession(ctx context.Context, session *domain.ABSImportSession) error
	GetABSImportSession(ctx context.Context, importID, sessionID string) (*domain.ABSImportSession, error)
	ListABSImportSessions(ctx context.Context, importID string, filter domain.SessionStatusFilter) ([]*domain.ABSImportSession, error)
	UpdateABSImportSessionStatus(ctx context.Context, importID, sessionID string, status domain.SessionImportStatus) error
	SkipABSImportSession(ctx context.Context, importID, sessionID, reason string) error
	RecalculateSessionStatuses(ctx context.Context, importID string) error
	GetABSImportStats(ctx context.Context, importID string) (mapped, unmapped, ready, imported int, err error)
	CreateABSImportProgress(ctx context.Context, progress *domain.ABSImportProgress) error
	GetABSImportProgress(ctx context.Context, importID, absUserID, absMediaID string) (*domain.ABSImportProgress, error)
	ListABSImportProgressForUser(ctx context.Context, importID, absUserID string) ([]*domain.ABSImportProgress, error)
	FindABSImportProgressByListenUpBook(ctx context.Context, importID, absUserID, listenUpBookID string) (*domain.ABSImportProgress, error)

	// Export/Backup
	StreamCollectionShares(ctx context.Context) iter.Seq2[*domain.CollectionShare, error]
	StreamBooks(ctx context.Context) iter.Seq2[*domain.Book, error]
	StreamContributors(ctx context.Context) iter.Seq2[*domain.Contributor, error]
	StreamSeries(ctx context.Context) iter.Seq2[*domain.Series, error]
	StreamCollections(ctx context.Context) iter.Seq2[*domain.Collection, error]
	StreamLenses(ctx context.Context) iter.Seq2[*domain.Shelf, error]
	StreamShelves(ctx context.Context) iter.Seq2[*domain.Shelf, error]
	StreamActivities(ctx context.Context) iter.Seq2[*domain.Activity, error]
	StreamListeningEvents(ctx context.Context) iter.Seq2[*domain.ListeningEvent, error]
	StreamProfiles(ctx context.Context) iter.Seq2[*domain.UserProfile, error]
	ClearAllData(ctx context.Context) error
	ClearAllProgress(ctx context.Context) error
	SaveProgress(ctx context.Context, progress *domain.PlaybackState) error
	GetCollectionNoAccessCheck(ctx context.Context, id string) (*domain.Collection, error)
	UpdateCollectionNoAccessCheck(ctx context.Context, coll *domain.Collection) error
	GetTagByIDForRestore(ctx context.Context, tagID string) (*domain.Tag, error)
	UpdateTagForRestore(ctx context.Context, t *domain.Tag) error

	// Batch Operations
	NewBatchWriter(maxSize int) BatchWriter

	// Checkpoint
	GetLibraryCheckpoint(ctx context.Context) (time.Time, error)
}

// BatchWriter provides efficient bulk write operations.
type BatchWriter interface {
	CreateBook(ctx context.Context, book *domain.Book) error
	Flush(ctx context.Context) error
	Cancel()
	Count() int
}

// EventEmitter is the interface for emitting SSE events.
type EventEmitter interface {
	Emit(event any)
	SetScanning(scanning bool)
}

// NoopEmitter is a no-op implementation of EventEmitter for testing.
type NoopEmitter struct{}

func (NoopEmitter) Emit(_ any)         {}
func (NoopEmitter) SetScanning(_ bool) {}

// NewNoopEmitter creates a new no-op emitter for testing.
func NewNoopEmitter() EventEmitter { return NoopEmitter{} }

// SearchIndexer is the interface for updating the search index.
type SearchIndexer interface {
	IndexBook(ctx context.Context, book *domain.Book) error
	DeleteBook(ctx context.Context, bookID string) error
	IndexContributor(ctx context.Context, c *domain.Contributor) error
	DeleteContributor(ctx context.Context, contributorID string) error
	IndexSeries(ctx context.Context, s *domain.Series) error
	DeleteSeries(ctx context.Context, seriesID string) error
}

// NoopSearchIndexer is a no-op implementation for testing.
type NoopSearchIndexer struct{}

func (NoopSearchIndexer) IndexBook(context.Context, *domain.Book) error               { return nil }
func (NoopSearchIndexer) DeleteBook(context.Context, string) error                    { return nil }
func (NoopSearchIndexer) IndexContributor(context.Context, *domain.Contributor) error { return nil }
func (NoopSearchIndexer) DeleteContributor(context.Context, string) error             { return nil }
func (NoopSearchIndexer) IndexSeries(context.Context, *domain.Series) error           { return nil }
func (NoopSearchIndexer) DeleteSeries(context.Context, string) error                  { return nil }

// NewNoopSearchIndexer creates a new no-op search indexer for testing.
func NewNoopSearchIndexer() SearchIndexer { return NoopSearchIndexer{} }

// TranscodeDeleter is the interface for cleaning up transcoded files.
type TranscodeDeleter interface {
	DeleteTranscodesForBook(ctx context.Context, bookID string) error
}

// NoopTranscodeDeleter is a no-op implementation for when transcoding is disabled.
type NoopTranscodeDeleter struct{}

func (NoopTranscodeDeleter) DeleteTranscodesForBook(context.Context, string) error { return nil }

// NewNoopTranscodeDeleter creates a new no-op transcode deleter.
func NewNoopTranscodeDeleter() TranscodeDeleter { return NoopTranscodeDeleter{} }
