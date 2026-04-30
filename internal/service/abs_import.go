package service

import (
	"context"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// absImportServiceStore is the narrow store interface ABSImportService depends on.
type absImportServiceStore interface {
	store.ABSImportStore
	// User resolution for auto-match display info and user-mapping handlers.
	GetUser(ctx context.Context, id string) (*domain.User, error)
	// Book resolution for auto-match display info and book-mapping handlers.
	GetBook(ctx context.Context, id string, userID string) (*domain.Book, error)
	GetBookByID(ctx context.Context, id string) (*domain.Book, error)
	// Playback state (used during session import).
	GetState(ctx context.Context, userID, bookID string) (*domain.PlaybackState, error)
	UpsertState(ctx context.Context, progress *domain.PlaybackState) error
	// Listening events (used during session import).
	CreateListeningEvent(ctx context.Context, event *domain.ListeningEvent) error
	GetEventsForUserBook(ctx context.Context, userID, bookID string) ([]*domain.ListeningEvent, error)
	// Reading sessions (populate Readers section after import).
	CreateReadingSession(ctx context.Context, session *domain.BookReadingSession) error
	GetUserBookSessions(ctx context.Context, userID, bookID string) ([]*domain.BookReadingSession, error)
}

// ABSImportService owns the entire Audiobookshelf import workflow.
// ACL is not enforced here — all ABS handlers are admin-only via middleware.
type ABSImportService struct {
	store  absImportServiceStore
	logger *slog.Logger
}

// NewABSImportService creates a new ABSImportService.
func NewABSImportService(s absImportServiceStore, logger *slog.Logger) *ABSImportService {
	return &ABSImportService{store: s, logger: logger}
}

// ============================================================
// Import CRUD
// ============================================================

// CreateABSImport persists a new import record.
func (s *ABSImportService) CreateABSImport(ctx context.Context, imp *domain.ABSImport) error {
	return s.store.CreateABSImport(ctx, imp)
}

// GetABSImport returns an import by ID.
func (s *ABSImportService) GetABSImport(ctx context.Context, id string) (*domain.ABSImport, error) {
	return s.store.GetABSImport(ctx, id)
}

// ListABSImports returns all imports.
func (s *ABSImportService) ListABSImports(ctx context.Context) ([]*domain.ABSImport, error) {
	return s.store.ListABSImports(ctx)
}

// UpdateABSImport persists changes to an import record.
func (s *ABSImportService) UpdateABSImport(ctx context.Context, imp *domain.ABSImport) error {
	return s.store.UpdateABSImport(ctx, imp)
}

// DeleteABSImport removes an import and its staging data.
func (s *ABSImportService) DeleteABSImport(ctx context.Context, id string) error {
	return s.store.DeleteABSImport(ctx, id)
}

// ============================================================
// User mappings
// ============================================================

// ListABSImportUsers returns import users filtered by mapping state.
func (s *ABSImportService) ListABSImportUsers(ctx context.Context, importID string, filter domain.MappingFilter) ([]*domain.ABSImportUser, error) {
	return s.store.ListABSImportUsers(ctx, importID, filter)
}

// GetABSImportUser returns a single import user.
func (s *ABSImportService) GetABSImportUser(ctx context.Context, importID, absUserID string) (*domain.ABSImportUser, error) {
	return s.store.GetABSImportUser(ctx, importID, absUserID)
}

// GetUser returns a ListenUp user (used to resolve display info when mapping).
func (s *ABSImportService) GetUser(ctx context.Context, id string) (*domain.User, error) {
	return s.store.GetUser(ctx, id)
}

// UpdateABSImportUserMapping sets or clears the ListenUp user mapping.
func (s *ABSImportService) UpdateABSImportUserMapping(ctx context.Context, importID, absUserID string, listenUpID, luEmail, luDisplayName *string) error {
	return s.store.UpdateABSImportUserMapping(ctx, importID, absUserID, listenUpID, luEmail, luDisplayName)
}

// RecalculateSessionStatusesForUser recomputes session statuses after a user mapping change.
func (s *ABSImportService) RecalculateSessionStatusesForUser(ctx context.Context, importID, absUserID string) error {
	return s.store.RecalculateSessionStatusesForUser(ctx, importID, absUserID)
}

// CreateABSImportUser stores a staged import user row.
func (s *ABSImportService) CreateABSImportUser(ctx context.Context, user *domain.ABSImportUser) error {
	return s.store.CreateABSImportUser(ctx, user)
}

// ============================================================
// Book mappings
// ============================================================

// ListABSImportBooks returns import books filtered by mapping state.
func (s *ABSImportService) ListABSImportBooks(ctx context.Context, importID string, filter domain.MappingFilter) ([]*domain.ABSImportBook, error) {
	return s.store.ListABSImportBooks(ctx, importID, filter)
}

// GetABSImportBook returns a single import book.
func (s *ABSImportService) GetABSImportBook(ctx context.Context, importID, absMediaID string) (*domain.ABSImportBook, error) {
	return s.store.GetABSImportBook(ctx, importID, absMediaID)
}

// GetBook returns a ListenUp book (used to resolve display info when mapping).
func (s *ABSImportService) GetBook(ctx context.Context, id string, userID string) (*domain.Book, error) {
	return s.store.GetBook(ctx, id, userID)
}

// GetBookByID returns a ListenUp book without user ACL filtering.
func (s *ABSImportService) GetBookByID(ctx context.Context, id string) (*domain.Book, error) {
	return s.store.GetBookByID(ctx, id)
}

// UpdateABSImportBookMapping sets or clears the ListenUp book mapping.
func (s *ABSImportService) UpdateABSImportBookMapping(ctx context.Context, importID, absMediaID string, listenUpID, luTitle, luAuthor *string) error {
	return s.store.UpdateABSImportBookMapping(ctx, importID, absMediaID, listenUpID, luTitle, luAuthor)
}

// RecalculateSessionStatusesForBook recomputes session statuses after a book mapping change.
func (s *ABSImportService) RecalculateSessionStatusesForBook(ctx context.Context, importID, absMediaID string) error {
	return s.store.RecalculateSessionStatusesForBook(ctx, importID, absMediaID)
}

// CreateABSImportBook stores a staged import book row.
func (s *ABSImportService) CreateABSImportBook(ctx context.Context, book *domain.ABSImportBook) error {
	return s.store.CreateABSImportBook(ctx, book)
}

// ============================================================
// Session mappings
// ============================================================

// ListABSImportSessions returns import sessions filtered by status.
func (s *ABSImportService) ListABSImportSessions(ctx context.Context, importID string, filter domain.SessionStatusFilter) ([]*domain.ABSImportSession, error) {
	return s.store.ListABSImportSessions(ctx, importID, filter)
}

// GetABSImportSession returns a single import session.
func (s *ABSImportService) GetABSImportSession(ctx context.Context, importID, sessionID string) (*domain.ABSImportSession, error) {
	return s.store.GetABSImportSession(ctx, importID, sessionID)
}

// UpdateABSImportSessionStatus marks a session with a new import status.
func (s *ABSImportService) UpdateABSImportSessionStatus(ctx context.Context, importID, sessionID string, status domain.SessionImportStatus) error {
	return s.store.UpdateABSImportSessionStatus(ctx, importID, sessionID, status)
}

// SkipABSImportSession marks a session as skipped with a reason.
func (s *ABSImportService) SkipABSImportSession(ctx context.Context, importID, sessionID, reason string) error {
	return s.store.SkipABSImportSession(ctx, importID, sessionID, reason)
}

// CreateABSImportSession stores a staged import session row.
func (s *ABSImportService) CreateABSImportSession(ctx context.Context, sess *domain.ABSImportSession) error {
	return s.store.CreateABSImportSession(ctx, sess)
}

// RecalculateSessionStatuses recomputes all session statuses for an import.
func (s *ABSImportService) RecalculateSessionStatuses(ctx context.Context, importID string) error {
	return s.store.RecalculateSessionStatuses(ctx, importID)
}

// ============================================================
// Progress (media progress from ABS backup)
// ============================================================

// CreateABSImportProgress stores a staged media progress row.
func (s *ABSImportService) CreateABSImportProgress(ctx context.Context, progress *domain.ABSImportProgress) error {
	return s.store.CreateABSImportProgress(ctx, progress)
}

// ListABSImportProgressForUser returns all media progress entries for an ABS user.
func (s *ABSImportService) ListABSImportProgressForUser(ctx context.Context, importID, absUserID string) ([]*domain.ABSImportProgress, error) {
	return s.store.ListABSImportProgressForUser(ctx, importID, absUserID)
}

// FindABSImportProgressByListenUpBook resolves ABS progress via the mapped ListenUp book ID.
func (s *ABSImportService) FindABSImportProgressByListenUpBook(ctx context.Context, importID, absUserID, listenUpBookID string) (*domain.ABSImportProgress, error) {
	return s.store.FindABSImportProgressByListenUpBook(ctx, importID, absUserID, listenUpBookID)
}

// ============================================================
// Playback state / listening events (used during session import)
// ============================================================

// GetState returns the playback state for a user+book pair.
func (s *ABSImportService) GetState(ctx context.Context, userID, bookID string) (*domain.PlaybackState, error) {
	return s.store.GetState(ctx, userID, bookID)
}

// UpsertState creates or updates a playback state record.
func (s *ABSImportService) UpsertState(ctx context.Context, progress *domain.PlaybackState) error {
	return s.store.UpsertState(ctx, progress)
}

// CreateListeningEvent stores a new listening event.
func (s *ABSImportService) CreateListeningEvent(ctx context.Context, event *domain.ListeningEvent) error {
	return s.store.CreateListeningEvent(ctx, event)
}

// GetEventsForUserBook returns all listening events for a user+book pair.
func (s *ABSImportService) GetEventsForUserBook(ctx context.Context, userID, bookID string) ([]*domain.ListeningEvent, error) {
	return s.store.GetEventsForUserBook(ctx, userID, bookID)
}

// CreateReadingSession stores a new reading session record.
func (s *ABSImportService) CreateReadingSession(ctx context.Context, session *domain.BookReadingSession) error {
	return s.store.CreateReadingSession(ctx, session)
}

// GetUserBookSessions returns all reading sessions for a user+book pair.
func (s *ABSImportService) GetUserBookSessions(ctx context.Context, userID, bookID string) ([]*domain.BookReadingSession, error) {
	return s.store.GetUserBookSessions(ctx, userID, bookID)
}
