package abs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// EventEmitter broadcasts SSE events to connected clients.
// Small interface for dependency injection (satisfied by sse.Manager).
type EventEmitter interface {
	Emit(event any)
}

// Importer executes the actual import of ABS data into ListenUp.
// Use Analyzer first to preview what will be imported.
type Importer struct {
	store     *store.Store
	events    EventEmitter
	converter *Converter
	logger    *slog.Logger
}

// NewImporter creates an importer.
// The events parameter is optional - if nil, SSE events won't be emitted.
func NewImporter(s *store.Store, events EventEmitter, logger *slog.Logger) *Importer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Importer{
		store:     s,
		events:    events,
		converter: NewConverter(),
		logger:    logger,
	}
}

// Import executes the import using the provided mappings.
// The mappings should come from BuildFinalMappings after admin review.
func (im *Importer) Import(
	ctx context.Context,
	backup *Backup,
	userMap map[string]string, // ABS userID -> ListenUp userID
	bookMap map[string]string, // ABS itemID -> ListenUp bookID
	opts ImportOptions,
) (*ImportResult, error) {
	start := time.Now()
	result := &ImportResult{}

	// Diagnostic logging: show what mappings we received
	im.logger.Info("ABS import starting",
		"user_mappings_count", len(userMap),
		"book_mappings_count", len(bookMap),
		"import_sessions", opts.ImportSessions,
		"import_progress", opts.ImportProgress,
	)
	// Log a sample of the mappings
	userMapSample := make([]string, 0, 3)
	for absID, luID := range userMap {
		userMapSample = append(userMapSample, absID+"->"+luID)
		if len(userMapSample) >= 3 {
			break
		}
	}
	bookMapSample := make([]string, 0, 3)
	for absID, luID := range bookMap {
		bookMapSample = append(bookMapSample, absID+"->"+luID)
		if len(bookMapSample) >= 3 {
			break
		}
	}
	im.logger.Debug("mapping samples",
		"user_map_sample", userMapSample,
		"book_map_sample", bookMapSample,
	)

	// Track affected users for progress rebuild
	affectedUsers := make(map[string]bool)

	// 1. Import sessions if enabled
	if opts.ImportSessions {
		if err := im.importSessions(ctx, backup, userMap, bookMap, result, affectedUsers); err != nil {
			return result, fmt.Errorf("import sessions: %w", err)
		}
	}

	// 2. Import progress if enabled and no sessions imported for that book
	// Progress is only imported for books without session history
	if opts.ImportProgress {
		if err := im.importProgress(ctx, backup, userMap, bookMap, result, affectedUsers); err != nil {
			return result, fmt.Errorf("import progress: %w", err)
		}
	}

	// 3. Record affected users
	for userID := range affectedUsers {
		result.AffectedUserIDs = append(result.AffectedUserIDs, userID)
	}

	result.Duration = time.Since(start)

	im.logger.Info("ABS import completed",
		"sessions_imported", result.SessionsImported,
		"sessions_skipped", result.SessionsSkipped,
		"progress_imported", result.ProgressImported,
		"progress_skipped", result.ProgressSkipped,
		"events_created", result.EventsCreated,
		"affected_users", len(result.AffectedUserIDs),
		"duration", result.Duration,
	)

	return result, nil
}

// importSessions converts and stores ABS sessions as listening events.
func (im *Importer) importSessions(
	ctx context.Context,
	backup *Backup,
	userMap, bookMap map[string]string,
	result *ImportResult,
	affectedUsers map[string]bool,
) error {
	sessions := backup.BookSessions()

	// Count unique user and book IDs in sessions for debugging
	sessionUserIDs := make(map[string]int)
	sessionBookIDs := make(map[string]int)
	for _, session := range sessions {
		sessionUserIDs[session.UserID]++
		sessionBookIDs[session.LibraryItemID]++
	}
	im.logger.Info("session analysis",
		"total_sessions", len(sessions),
		"unique_user_ids", len(sessionUserIDs),
		"unique_book_ids", len(sessionBookIDs),
	)
	// Log first few user/book IDs from sessions
	sampleSessionUsers := make([]string, 0, 3)
	for id := range sessionUserIDs {
		sampleSessionUsers = append(sampleSessionUsers, id)
		if len(sampleSessionUsers) >= 3 {
			break
		}
	}
	sampleSessionBooks := make([]string, 0, 3)
	for id := range sessionBookIDs {
		sampleSessionBooks = append(sampleSessionBooks, id)
		if len(sampleSessionBooks) >= 3 {
			break
		}
	}
	im.logger.Debug("session ID samples",
		"user_ids", sampleSessionUsers,
		"book_ids", sampleSessionBooks,
	)

	// Group sessions by user+book for batch conversion
	sessionGroups := make(map[string][]Session)
	for _, session := range sessions {
		key := session.UserID + ":" + session.LibraryItemID
		sessionGroups[key] = append(sessionGroups[key], session)
	}

	for key, sessions := range sessionGroups {
		// Parse key to get ABS IDs
		var absUserID, absItemID string
		fmt.Sscanf(key, "%s:%s", &absUserID, &absItemID)

		// This is a simplification - the key format needs proper handling
		// Let's extract properly from the first session
		if len(sessions) == 0 {
			continue
		}
		absUserID = sessions[0].UserID
		absItemID = sessions[0].LibraryItemID

		// Look up ListenUp IDs
		listenUpUserID, userOK := userMap[absUserID]
		listenUpBookID, bookOK := bookMap[absItemID]

		if !userOK || !bookOK {
			// Log the first few skipped sessions for debugging
			if result.SessionsSkipped < 5 {
				im.logger.Debug("sessions skipped - no mapping",
					"abs_user_id", absUserID,
					"abs_item_id", absItemID,
					"user_found", userOK,
					"book_found", bookOK,
					"session_count", len(sessions),
				)
			}
			result.SessionsSkipped += len(sessions)
			continue
		}

		// Convert sessions to events
		events := im.converter.ConvertSessions(sessions, listenUpUserID, listenUpBookID)

		// Store each event
		for _, event := range events {
			if err := im.store.CreateListeningEvent(ctx, event); err != nil {
				// Log but continue - we want to import what we can
				im.logger.Warn("failed to create listening event",
					"error", err,
					"user_id", listenUpUserID,
					"book_id", listenUpBookID,
				)
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("failed to create event for user %s book %s: %v", listenUpUserID, listenUpBookID, err))
				continue
			}
			result.EventsCreated++
		}

		result.SessionsImported += len(sessions)
		affectedUsers[listenUpUserID] = true
	}

	return nil
}

// importProgress imports MediaProgress records for books without session history.
func (im *Importer) importProgress(
	ctx context.Context,
	backup *Backup,
	userMap, bookMap map[string]string,
	result *ImportResult,
	affectedUsers map[string]bool,
) error {
	// Track which user+book combos already have sessions
	hasSessionHistory := make(map[string]bool)
	for _, session := range backup.BookSessions() {
		key := session.UserID + ":" + session.LibraryItemID
		hasSessionHistory[key] = true
	}

	for _, user := range backup.ImportableUsers() {
		listenUpUserID, userOK := userMap[user.ID]
		if !userOK {
			continue
		}

		for _, progress := range user.Progress {
			if !progress.IsBook() {
				continue
			}

			listenUpBookID, bookOK := bookMap[progress.LibraryItemID]
			if !bookOK {
				result.ProgressSkipped++
				continue
			}

			// Skip if we already imported sessions for this user+book
			key := user.ID + ":" + progress.LibraryItemID
			if hasSessionHistory[key] {
				continue // Sessions will provide better data
			}

			// Convert progress to synthetic events
			events := im.converter.ProgressToEvents(&progress, listenUpUserID, listenUpBookID)

			for _, event := range events {
				if err := im.store.CreateListeningEvent(ctx, event); err != nil {
					im.logger.Warn("failed to create progress event",
						"error", err,
						"user_id", listenUpUserID,
						"book_id", listenUpBookID,
					)
					continue
				}
				result.EventsCreated++
			}

			result.ProgressImported++
			affectedUsers[listenUpUserID] = true
		}
	}

	return nil
}

// RebuildProgressForUsers rebuilds PlaybackProgress for affected users.
// Call this after import to materialize progress from imported events.
func (im *Importer) RebuildProgressForUsers(ctx context.Context, userIDs []string) error {
	for _, userID := range userIDs {
		if err := im.rebuildUserProgress(ctx, userID); err != nil {
			im.logger.Warn("failed to rebuild progress for user",
				"user_id", userID,
				"error", err,
			)
			// Continue with other users
		}
	}
	return nil
}

// rebuildUserProgress rebuilds all progress records for a single user.
func (im *Importer) rebuildUserProgress(ctx context.Context, userID string) error {
	// Get all events for this user
	events, err := im.store.GetEventsForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get events for user: %w", err)
	}

	// Group events by book
	eventsByBook := make(map[string][]*domain.ListeningEvent)
	for _, event := range events {
		eventsByBook[event.BookID] = append(eventsByBook[event.BookID], event)
	}

	// Rebuild progress for each book
	for bookID, bookEvents := range eventsByBook {
		if err := im.rebuildBookProgress(ctx, userID, bookID, bookEvents); err != nil {
			im.logger.Warn("failed to rebuild book progress",
				"user_id", userID,
				"book_id", bookID,
				"error", err,
			)
			continue
		}
	}

	return nil
}

// rebuildBookProgress rebuilds progress for a user+book from events.
func (im *Importer) rebuildBookProgress(
	ctx context.Context,
	userID, bookID string,
	events []*domain.ListeningEvent,
) error {
	if len(events) == 0 {
		return nil
	}

	// Get book duration
	book, err := im.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	// Check if progress already exists (may have IsFinished from ABS import)
	existingProgress, _ := im.store.GetProgress(ctx, userID, bookID)

	// Sort events by time
	sortEventsByTime(events)

	// Build progress from first event
	progress := domain.NewPlaybackProgress(events[0], book.TotalDuration)

	// Update with remaining events
	for _, event := range events[1:] {
		progress.UpdateFromEvent(event, book.TotalDuration)
	}

	// IMPORTANT: Preserve IsFinished from ABS import if it was set.
	// The ABS import sets IsFinished based on the actual ABS media progress,
	// which may be more accurate than our position-based calculation
	// (e.g., when book duration was 0 during import, or position tracking differs).
	if existingProgress != nil && existingProgress.IsFinished && !progress.IsFinished {
		im.logger.Debug("preserving IsFinished from ABS import",
			"user_id", userID,
			"book_id", bookID,
			"existing_finished_at", existingProgress.FinishedAt,
		)
		progress.IsFinished = true
		progress.FinishedAt = existingProgress.FinishedAt
	}

	// Save progress
	if err := im.store.UpsertProgress(ctx, progress); err != nil {
		return fmt.Errorf("upsert progress: %w", err)
	}

	// Emit SSE event so clients update their Continue Listening section
	if im.events != nil {
		im.events.Emit(sse.NewProgressUpdatedEvent(userID, progress))
	}

	return nil
}

// sortEventsByTime sorts events by StartedAt ascending.
func sortEventsByTime(events []*domain.ListeningEvent) {
	for i := 1; i < len(events); i++ {
		j := i
		for j > 0 && events[j].StartedAt.Before(events[j-1].StartedAt) {
			events[j], events[j-1] = events[j-1], events[j]
			j--
		}
	}
}
