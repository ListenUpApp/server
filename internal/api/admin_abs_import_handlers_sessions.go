package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) handleListABSImportSessions(ctx context.Context, input *ListABSImportSessionsInput) (*ListABSImportSessionsOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	filter := domain.SessionStatusFilter(input.Status)
	if filter == "" {
		filter = domain.SessionFilterAll
	}

	sessions, err := s.services.ABSImport.ListABSImportSessions(ctx, input.ID, filter)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list sessions", err)
	}

	// Also get all sessions for summary - if this fails, we still return filtered results
	// but with empty summary (better than hiding the error completely)
	allSessions, err := s.services.ABSImport.ListABSImportSessions(ctx, input.ID, domain.SessionFilterAll)
	if err != nil {
		s.logger.Error("failed to get all sessions for summary", slog.String("error", err.Error()))
		// Continue with empty allSessions - summary will be zeros
		allSessions = nil
	}

	resp := &ListABSImportSessionsOutput{}
	resp.Body.Sessions = make([]ABSImportSessionResponse, len(sessions))
	for i, sess := range sessions {
		resp.Body.Sessions[i] = toABSImportSessionResponse(sess)
	}

	// Calculate summary
	for _, sess := range allSessions {
		resp.Body.Summary.Total++
		switch sess.Status {
		case domain.SessionStatusPendingUser, domain.SessionStatusPendingBook:
			resp.Body.Summary.Pending++
		case domain.SessionStatusReady:
			resp.Body.Summary.Ready++
		case domain.SessionStatusImported:
			resp.Body.Summary.Imported++
		case domain.SessionStatusSkipped:
			resp.Body.Summary.Skipped++
		}
	}

	return resp, nil
}

func (s *Server) handleImportABSSessions(ctx context.Context, input *ImportABSSessionsInput) (*ImportABSSessionsOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	// Get ready sessions
	readySessions, err := s.services.ABSImport.ListABSImportSessions(ctx, input.ID, domain.SessionFilterReady)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get sessions", err)
	}

	if len(readySessions) == 0 {
		return &ImportABSSessionsOutput{
			Body: struct {
				SessionsImported       int    `json:"sessions_imported" doc:"Sessions successfully imported"`
				SessionsFailed         int    `json:"sessions_failed" doc:"Sessions that failed to import"`
				EventsCreated          int    `json:"events_created" doc:"Listening events created"`
				ProgressRebuilt        int    `json:"progress_rebuilt" doc:"User+book progress records rebuilt"`
				ProgressFailed         int    `json:"progress_failed" doc:"Progress rebuilds that failed"`
				ABSProgressUnmatched   int    `json:"abs_progress_unmatched" doc:"Books where ABS progress could not be matched (finished status may be incorrect)"`
				ReadingSessionsCreated int    `json:"reading_sessions_created" doc:"BookReadingSession records created for readers section"`
				ReadingSessionsSkipped int    `json:"reading_sessions_skipped" doc:"Sessions skipped (already existed)"`
				Duration               string `json:"duration" doc:"Import duration"`
			}{
				SessionsImported:       0,
				SessionsFailed:         0,
				EventsCreated:          0,
				ProgressRebuilt:        0,
				ProgressFailed:         0,
				ABSProgressUnmatched:   0,
				ReadingSessionsCreated: 0,
				ReadingSessionsSkipped: 0,
				Duration:               time.Since(start).String(),
			},
		}, nil
	}

	// Get all user and book mappings - these MUST succeed or import will silently fail
	users, err := s.services.ABSImport.ListABSImportUsers(ctx, input.ID, domain.MappingFilterMapped)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load user mappings", err)
	}
	books, err := s.services.ABSImport.ListABSImportBooks(ctx, input.ID, domain.MappingFilterMapped)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load book mappings", err)
	}

	userMap := make(map[string]string) // ABS user ID -> ListenUp user ID
	for _, u := range users {
		if u.ListenUpID != nil {
			userMap[u.ABSUserID] = *u.ListenUpID
		}
	}

	bookMap := make(map[string]string) // ABS media ID -> ListenUp book ID
	for _, b := range books {
		if b.ListenUpID != nil {
			bookMap[b.ABSMediaID] = *b.ListenUpID
		}
	}

	// DIAGNOSTIC: Log mapping counts and sample data
	s.logger.Info("execute import: mapping summary",
		slog.Int("user_mappings", len(userMap)),
		slog.Int("book_mappings", len(bookMap)),
		slog.Int("ready_sessions", len(readySessions)))

	// DIAGNOSTIC: Log sample of book mappings for debugging
	bookMapSample := make([]string, 0, 5)
	for absID, luID := range bookMap {
		bookMapSample = append(bookMapSample, absID+" -> "+luID)
		if len(bookMapSample) >= 5 {
			break
		}
	}
	s.logger.Debug("execute import: book mapping sample", slog.Any("samples", bookMapSample))

	// Import each ready session
	sessionsImported := 0
	sessionsFailed := 0
	eventsCreated := 0

	// Track user+book combinations to update progress after import
	// Include ABS IDs so we can look up finished status from ABSImportProgress
	type userBookKey struct {
		userID     string // ListenUp user ID
		bookID     string // ListenUp book ID
		absUserID  string // ABS user ID (for progress lookup)
		absMediaID string // ABS media ID (for progress lookup)
	}
	affectedUserBooks := make(map[userBookKey]bool)

	for i, sess := range readySessions {
		listenUpUserID, userOK := userMap[sess.ABSUserID]
		listenUpBookID, bookOK := bookMap[sess.ABSMediaID]

		if !userOK || !bookOK {
			continue // Skip if mappings not found (shouldn't happen for ready sessions)
		}

		// DEBUG: Log first 5 sessions to trace position values
		if i < 5 {
			s.logger.Debug("creating listening event from ABS session",
				slog.String("session_id", sess.ABSSessionID),
				slog.String("abs_media_id", sess.ABSMediaID),
				slog.String("listenup_book_id", listenUpBookID),
				slog.Int64("start_position", sess.StartPosition),
				slog.Int64("end_position", sess.EndPosition),
				slog.Int64("duration", sess.Duration),
			)
		}

		// Generate event ID
		eventID, err := id.Generate("evt")
		if err != nil {
			s.logger.Error("failed to generate event ID",
				slog.String("session_id", sess.ABSSessionID),
				slog.String("error", err.Error()))
			sessionsFailed++
			continue
		}

		// Create listening event
		event := &domain.ListeningEvent{
			ID:              eventID,
			UserID:          listenUpUserID,
			BookID:          listenUpBookID,
			StartPositionMs: sess.StartPosition,
			EndPositionMs:   sess.EndPosition,
			DurationMs:      sess.Duration,
			DeviceID:        "abs-import",
			DeviceName:      "ABS Import",
			StartedAt:       sess.StartTime,
			EndedAt:         sess.StartTime.Add(time.Duration(sess.Duration) * time.Millisecond),
			PlaybackSpeed:   1.0,
			CreatedAt:       time.Now(),
		}

		if err := s.services.ABSImport.CreateListeningEvent(ctx, event); err != nil {
			s.logger.Error("failed to create listening event",
				slog.String("session_id", sess.ABSSessionID),
				slog.String("error", err.Error()))
			sessionsFailed++
			continue
		}

		eventsCreated++
		affectedUserBooks[userBookKey{
			userID:     listenUpUserID,
			bookID:     listenUpBookID,
			absUserID:  sess.ABSUserID,
			absMediaID: sess.ABSMediaID,
		}] = true

		// Mark session as imported - if this fails, the event was still created
		// so we count it as imported but log the status update failure
		if err := s.services.ABSImport.UpdateABSImportSessionStatus(ctx, input.ID, sess.ABSSessionID, domain.SessionStatusImported); err != nil {
			s.logger.Error("failed to mark session imported (event was created)",
				slog.String("session_id", sess.ABSSessionID),
				slog.String("error", err.Error()))
		}

		sessionsImported++
	}

	// DIAGNOSTIC: Log session import results
	s.logger.Info("execute import: session import complete",
		slog.Int("sessions_imported", sessionsImported),
		slog.Int("sessions_failed", sessionsFailed),
		slog.Int("events_created", eventsCreated),
		slog.Int("unique_user_book_combos", len(affectedUserBooks)))

	// Rebuild PlaybackProgress for all affected user+book combinations
	// This is critical for Continue Listening to work after ABS import
	progressRebuilt := 0
	progressFailed := 0
	progressUnmatchedABS := 0 // Books where we couldn't match ABS progress data
	for key := range affectedUserBooks {
		matched, err := s.rebuildProgressFromEvents(ctx, input.ID, key.userID, key.bookID, key.absUserID, key.absMediaID)
		if err != nil {
			s.logger.Error("failed to rebuild progress",
				slog.String("user_id", key.userID),
				slog.String("book_id", key.bookID),
				slog.String("error", err.Error()))
			progressFailed++
		} else {
			progressRebuilt++
			if !matched {
				progressUnmatchedABS++
			}
		}
	}
	s.logger.Info("rebuilt progress for imported sessions",
		slog.Int("progress_rebuilt", progressRebuilt),
		slog.Int("progress_failed", progressFailed),
		slog.Int("progress_unmatched_abs", progressUnmatchedABS))

	// Create progress records from MediaProgress entries that don't have sessions
	// This ensures the Readers section is populated even for books without detailed history
	//
	// IMPORTANT: We iterate over stored ABSImportProgress entries directly rather than
	// iterating over bookMap. This handles cases where the ABSMediaID in progress
	// (from mediaProgresses.mediaItemId) might differ from the key used in bookMap
	// (from libraryItems.mediaId).
	progressFromMediaProgress := 0
	progressSkippedNoBook := 0
	progressSkippedHasProgress := 0
	for _, u := range users {
		if u.ListenUpID == nil {
			continue
		}
		listenUpUserID := *u.ListenUpID

		// Get all MediaProgress entries stored for this user
		allProgress, err := s.services.ABSImport.ListABSImportProgressForUser(ctx, input.ID, u.ABSUserID)
		if err != nil {
			s.logger.Warn("failed to list ABS progress for user",
				slog.String("abs_user_id", u.ABSUserID),
				slog.String("error", err.Error()))
			continue
		}

		// DIAGNOSTIC: Log progress entries for this user
		s.logger.Info("found ABS progress entries for user",
			slog.String("abs_user_id", u.ABSUserID),
			slog.String("listenup_user_id", listenUpUserID),
			slog.Int("count", len(allProgress)))

		for _, absProgress := range allProgress {
			if absProgress.CurrentTime == 0 {
				s.logger.Debug("skipping progress with zero position",
					slog.String("abs_media_id", absProgress.ABSMediaID))
				continue // No position to import
			}

			// Find the matching book - first try direct lookup, then try via bookMap
			var listenUpBookID string
			absBook, err := s.services.ABSImport.GetABSImportBook(ctx, input.ID, absProgress.ABSMediaID)
			if err == nil && absBook != nil && absBook.ListenUpID != nil {
				listenUpBookID = *absBook.ListenUpID
				s.logger.Debug("found book via direct lookup",
					slog.String("abs_progress_media_id", absProgress.ABSMediaID),
					slog.String("listenup_book_id", listenUpBookID),
					slog.Bool("is_finished", absProgress.IsFinished))
			} else {
				// Fallback: check if bookMap has this ID (handles potential ID variations)
				if id, ok := bookMap[absProgress.ABSMediaID]; ok {
					listenUpBookID = id
					s.logger.Debug("found book via bookMap fallback",
						slog.String("abs_progress_media_id", absProgress.ABSMediaID),
						slog.String("listenup_book_id", listenUpBookID))
				} else {
					s.logger.Warn("no book mapping found for ABS progress - SKIPPING",
						slog.String("abs_media_id", absProgress.ABSMediaID),
						slog.Int64("current_time_ms", absProgress.CurrentTime),
						slog.Bool("is_finished", absProgress.IsFinished),
						slog.Float64("progress_pct", absProgress.Progress*100))
					progressSkippedNoBook++
					continue
				}
			}

			// Skip if we already have progress from sessions
			key := userBookKey{
				userID:     listenUpUserID,
				bookID:     listenUpBookID,
				absUserID:  u.ABSUserID,
				absMediaID: absProgress.ABSMediaID,
			}
			if affectedUserBooks[key] {
				s.logger.Debug("skipping - already handled via sessions",
					slog.String("abs_media_id", absProgress.ABSMediaID),
					slog.String("listenup_book_id", listenUpBookID))
				continue // Already handled via sessions
			}

			// Only create if no existing state
			existingProgress, _ := s.services.ABSImport.GetState(ctx, listenUpUserID, listenUpBookID)
			if existingProgress != nil {
				// Get book duration for logging
				existingBook, _ := s.services.ABSImport.GetBookByID(ctx, listenUpBookID)
				existingDuration := int64(0)
				if existingBook != nil {
					existingDuration = existingBook.TotalDuration
				}
				s.logger.Debug("skipping - existing progress found",
					slog.String("abs_media_id", absProgress.ABSMediaID),
					slog.String("listenup_book_id", listenUpBookID),
					slog.Bool("existing_is_finished", existingProgress.IsFinished),
					slog.Float64("existing_progress_pct", existingProgress.ComputeProgress(existingDuration)*100))
				progressSkippedHasProgress++
				continue // Already has progress
			}

			// Get book duration from ListenUp
			book, err := s.services.ABSImport.GetBookByID(ctx, listenUpBookID)
			if err != nil || book == nil {
				continue
			}

			// Calculate position - clamp if needed, but don't skip books without duration
			bookDurationMs := book.TotalDuration
			positionMs := absProgress.CurrentTime
			if bookDurationMs > 0 && positionMs > bookDurationMs {
				positionMs = int64(float64(bookDurationMs) * 0.98)
			}

			progress := &domain.PlaybackState{
				UserID:            listenUpUserID,
				BookID:            listenUpBookID,
				CurrentPositionMs: positionMs,
				StartedAt:         absProgress.LastUpdate, // Best approximation
				LastPlayedAt:      absProgress.LastUpdate,
				TotalListenTimeMs: positionMs, // Approximate
				UpdatedAt:         time.Now(),
				IsFinished:        absProgress.IsFinished,
			}
			if absProgress.IsFinished {
				progress.FinishedAt = absProgress.FinishedAt
			}

			if err := s.services.ABSImport.UpsertState(ctx, progress); err != nil {
				s.logger.Warn("failed to create state from MediaProgress",
					slog.String("user_id", listenUpUserID),
					slog.String("book_id", listenUpBookID),
					slog.String("error", err.Error()))
			} else {
				progressFromMediaProgress++
				s.logger.Debug("created progress from MediaProgress",
					slog.String("user_id", listenUpUserID),
					slog.String("book_id", listenUpBookID),
					slog.Bool("is_finished", progress.IsFinished),
					slog.Float64("progress_pct", progress.ComputeProgress(bookDurationMs)*100))
			}
		}
	}
	if progressFromMediaProgress > 0 || progressSkippedNoBook > 0 {
		s.logger.Info("created progress from MediaProgress",
			slog.Int("created", progressFromMediaProgress),
			slog.Int("skipped_no_book", progressSkippedNoBook),
			slog.Int("skipped_has_progress", progressSkippedHasProgress))
	}

	// Create BookReadingSession records from ABS progress (populates the "Readers" section)
	// This is a separate pass to ensure sessions are created for all user+book combinations
	// that have progress, regardless of whether the progress was created in this import or existed before.
	readingSessionsCreated := 0
	readingSessionsSkipped := 0
	for _, u := range users {
		if u.ListenUpID == nil {
			continue
		}
		listenUpUserID := *u.ListenUpID

		// Get all MediaProgress entries stored for this user
		allProgress, err := s.services.ABSImport.ListABSImportProgressForUser(ctx, input.ID, u.ABSUserID)
		if err != nil {
			continue
		}

		for _, absProgress := range allProgress {
			// Find the ListenUp book ID
			var listenUpBookID string
			absBook, err := s.services.ABSImport.GetABSImportBook(ctx, input.ID, absProgress.ABSMediaID)
			if err == nil && absBook != nil && absBook.ListenUpID != nil {
				listenUpBookID = *absBook.ListenUpID
			} else if bookID, ok := bookMap[absProgress.ABSMediaID]; ok {
				listenUpBookID = bookID
			} else {
				continue // Can't create session without book mapping
			}

			// Check if user already has a session for this book
			existingSessions, err := s.services.ABSImport.GetUserBookSessions(ctx, listenUpUserID, listenUpBookID)
			if err == nil && len(existingSessions) > 0 {
				readingSessionsSkipped++
				continue // Already has a reading session
			}

			// Generate session ID
			sessionID, err := id.Generate("rsession")
			if err != nil {
				s.logger.Warn("failed to generate reading session ID",
					slog.String("user_id", listenUpUserID),
					slog.String("book_id", listenUpBookID),
					slog.String("error", err.Error()))
				continue
			}

			// Determine timestamps (use LastUpdate as best approximation for started time)
			now := time.Now()
			startedAt := absProgress.LastUpdate
			if startedAt.IsZero() {
				startedAt = now
			}

			// Calculate estimated listen time from progress
			var listenTimeMs int64
			if absProgress.Duration > 0 {
				listenTimeMs = int64(absProgress.Progress * float64(absProgress.Duration))
			}

			// Create the reading session
			session := &domain.BookReadingSession{
				ID:            sessionID,
				UserID:        listenUpUserID,
				BookID:        listenUpBookID,
				StartedAt:     startedAt,
				FinishedAt:    nil,
				IsCompleted:   absProgress.IsFinished,
				FinalProgress: absProgress.Progress,
				ListenTimeMs:  listenTimeMs,
				CreatedAt:     now,
				UpdatedAt:     now,
			}

			// If finished, set the FinishedAt timestamp
			if absProgress.IsFinished {
				if absProgress.FinishedAt != nil {
					session.FinishedAt = absProgress.FinishedAt
				} else {
					session.FinishedAt = &now
				}
			}

			// Store the session
			if err := s.services.ABSImport.CreateReadingSession(ctx, session); err != nil {
				s.logger.Warn("failed to create reading session",
					slog.String("session_id", sessionID),
					slog.String("user_id", listenUpUserID),
					slog.String("book_id", listenUpBookID),
					slog.String("error", err.Error()))
				continue
			}

			readingSessionsCreated++
			s.logger.Debug("created reading session from ABS progress",
				slog.String("session_id", sessionID),
				slog.String("user_id", listenUpUserID),
				slog.String("book_id", listenUpBookID),
				slog.Bool("is_finished", absProgress.IsFinished),
				slog.Float64("progress", absProgress.Progress))
		}
	}
	if readingSessionsCreated > 0 || readingSessionsSkipped > 0 {
		s.logger.Info("created reading sessions from ABS progress",
			slog.Int("created", readingSessionsCreated),
			slog.Int("skipped_existing", readingSessionsSkipped))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	return &ImportABSSessionsOutput{
		Body: struct {
			SessionsImported       int    `json:"sessions_imported" doc:"Sessions successfully imported"`
			SessionsFailed         int    `json:"sessions_failed" doc:"Sessions that failed to import"`
			EventsCreated          int    `json:"events_created" doc:"Listening events created"`
			ProgressRebuilt        int    `json:"progress_rebuilt" doc:"User+book progress records rebuilt"`
			ProgressFailed         int    `json:"progress_failed" doc:"Progress rebuilds that failed"`
			ABSProgressUnmatched   int    `json:"abs_progress_unmatched" doc:"Books where ABS progress could not be matched (finished status may be incorrect)"`
			ReadingSessionsCreated int    `json:"reading_sessions_created" doc:"BookReadingSession records created for readers section"`
			ReadingSessionsSkipped int    `json:"reading_sessions_skipped" doc:"Sessions skipped (already existed)"`
			Duration               string `json:"duration" doc:"Import duration"`
		}{
			SessionsImported:       sessionsImported,
			SessionsFailed:         sessionsFailed,
			EventsCreated:          eventsCreated,
			ProgressRebuilt:        progressRebuilt,
			ProgressFailed:         progressFailed,
			ABSProgressUnmatched:   progressUnmatchedABS,
			ReadingSessionsCreated: readingSessionsCreated,
			ReadingSessionsSkipped: readingSessionsSkipped,
			Duration:               time.Since(start).String(),
		},
	}, nil
}

func (s *Server) handleSkipABSSession(ctx context.Context, input *SkipABSSessionInput) (*SkipABSSessionOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	reason := input.Body.Reason
	if reason == "" {
		reason = "Skipped by admin"
	}

	if err := s.services.ABSImport.SkipABSImportSession(ctx, input.ID, input.SessionID, reason); err != nil {
		return nil, huma.Error500InternalServerError("failed to skip session", err)
	}

	sess, err := s.services.ABSImport.GetABSImportSession(ctx, input.ID, input.SessionID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get session", err)
	}

	return &SkipABSSessionOutput{
		Body: toABSImportSessionResponse(sess),
	}, nil
}

func toABSImportSessionResponse(s *domain.ABSImportSession) ABSImportSessionResponse {
	resp := ABSImportSessionResponse{
		ABSSessionID:  s.ABSSessionID,
		ABSUserID:     s.ABSUserID,
		ABSMediaID:    s.ABSMediaID,
		StartTime:     s.StartTime.Format(time.RFC3339),
		Duration:      s.Duration,
		StartPosition: s.StartPosition,
		EndPosition:   s.EndPosition,
		Status:        string(s.Status),
	}
	if s.ImportedAt != nil {
		resp.ImportedAt = s.ImportedAt.Format(time.RFC3339)
	}
	if s.SkipReason != nil {
		resp.SkipReason = *s.SkipReason
	}
	return resp
}

// rebuildProgressFromEvents rebuilds PlaybackProgress for a user+book from all events.
// This is used after ABS import to ensure Continue Listening works correctly.
// Handles duration mismatch: if ABS position > ListenUp duration, clamps to 98% to avoid
// incorrectly marking books as completed.
// Also honors the finished status from ABSImportProgress if the book was marked completed in ABS.
// Returns (absProgressMatched, error) where absProgressMatched indicates if ABS progress was found.
func (s *Server) rebuildProgressFromEvents(ctx context.Context, importID, userID, bookID, absUserID, _ string) (absProgressMatched bool, err error) {
	// Get all events for this user+book
	events, err := s.services.ABSImport.GetEventsForUserBook(ctx, userID, bookID)
	if err != nil {
		return false, fmt.Errorf("get events: %w", err)
	}

	if len(events) == 0 {
		return true, nil // Nothing to do - consider matched (no progress to check)
	}

	// DEBUG: Log events being used for progress rebuild
	s.logger.Debug("events for progress rebuild",
		slog.String("user_id", userID),
		slog.String("book_id", bookID),
		slog.Int("event_count", len(events)),
	)
	for i, e := range events {
		if i < 3 { // Log first 3 events
			s.logger.Debug("event detail",
				slog.String("event_id", e.ID),
				slog.String("event_book_id", e.BookID),
				slog.Int64("end_position_ms", e.EndPositionMs),
			)
		}
	}

	// Get book duration from ListenUp
	book, err := s.services.ABSImport.GetBookByID(ctx, bookID)
	if err != nil {
		return false, fmt.Errorf("get book: %w", err)
	}
	if book == nil {
		return false, fmt.Errorf("book %s not found (nil returned without error)", bookID)
	}
	bookDurationMs := book.TotalDuration
	if bookDurationMs <= 0 {
		return false, fmt.Errorf("book %s has invalid duration: %d", bookID, bookDurationMs)
	}

	// Find the latest event and calculate totals
	var latestEvent *domain.ListeningEvent
	var totalListenTimeMs int64
	var maxPositionMs int64
	var earliestStartedAt time.Time

	for _, event := range events {
		totalListenTimeMs += event.DurationMs

		// Track max position (forward progress only)
		if event.EndPositionMs > maxPositionMs {
			maxPositionMs = event.EndPositionMs
		}

		// Track latest event by EndedAt
		if latestEvent == nil || event.EndedAt.After(latestEvent.EndedAt) {
			latestEvent = event
		}

		// Track earliest start
		if earliestStartedAt.IsZero() || event.StartedAt.Before(earliestStartedAt) {
			earliestStartedAt = event.StartedAt
		}
	}

	// Handle duration mismatch: if imported position exceeds book duration,
	// clamp to 98% to avoid incorrectly marking as completed.
	// This can happen when ABS had a different duration than ListenUp.
	if maxPositionMs > bookDurationMs && bookDurationMs > 0 {
		s.logger.Warn("ABS position exceeds book duration, clamping",
			slog.String("user_id", userID),
			slog.String("book_id", bookID),
			slog.Int64("abs_position_ms", maxPositionMs),
			slog.Int64("book_duration_ms", bookDurationMs))
		// Clamp to 98% - this ensures the book appears in Continue Listening
		// rather than being incorrectly marked as finished
		maxPositionMs = int64(float64(bookDurationMs) * 0.98)
	}

	// Get existing state or create new
	existingProgress, err := s.services.ABSImport.GetState(ctx, userID, bookID)
	if err != nil && !errors.Is(err, store.ErrProgressNotFound) {
		// Real error, not just "not found"
		return false, fmt.Errorf("get state: %w", err)
	}

	var progress *domain.PlaybackState
	needsSave := true
	if existingProgress != nil {
		// Update existing - only if we have newer data
		if latestEvent.EndedAt.After(existingProgress.LastPlayedAt) {
			progress = existingProgress
			progress.CurrentPositionMs = maxPositionMs
			progress.TotalListenTimeMs += totalListenTimeMs - existingProgress.TotalListenTimeMs // Add delta
			progress.LastPlayedAt = latestEvent.EndedAt
			progress.UpdatedAt = time.Now()

			// Check completion (but we already clamped to avoid false positives)
			if bookDurationMs > 0 && float64(progress.CurrentPositionMs) >= float64(bookDurationMs)*0.99 {
				progress.IsFinished = true
				now := time.Now()
				progress.FinishedAt = &now
			}
		} else {
			// Existing progress is newer - don't update position data
			// BUT still check ABS finished status below (user might have finished in ABS
			// but only played a few seconds in ListenUp)
			progress = existingProgress
			needsSave = false // Will be set to true if we update IsFinished
		}
	} else {
		// Create new progress
		progress = &domain.PlaybackState{
			UserID:            userID,
			BookID:            bookID,
			CurrentPositionMs: maxPositionMs,
			StartedAt:         earliestStartedAt,
			LastPlayedAt:      latestEvent.EndedAt,
			TotalListenTimeMs: totalListenTimeMs,
			UpdatedAt:         time.Now(),
		}

		// Check completion
		if bookDurationMs > 0 && float64(progress.CurrentPositionMs) >= float64(bookDurationMs)*0.99 {
			progress.IsFinished = true
			now := time.Now()
			progress.FinishedAt = &now
		}
	}

	// Check if ABS marked this book as finished (honors original listening history)
	// This handles cases where the position doesn't reach 99% but the user marked it complete in ABS
	//
	// IMPORTANT: We look up ABSImportProgress by ListenUp book ID rather than ABS media ID.
	// This is because ABS playbackSessions.mediaItemId and mediaProgresses.mediaItemId can contain
	// DIFFERENT UUIDs for the same logical book. By resolving through the ListenUp book ID,
	// we correctly match progress entries regardless of which ABS ID scheme was used.
	absProgressMatched = true // Assume matched unless we explicitly fail to find it
	s.logger.Info("checking ABS finished status for book",
		slog.String("book_id", bookID),
		slog.String("import_id", importID),
		slog.String("abs_user_id", absUserID),
		slog.Bool("progress_is_finished", progress.IsFinished),
		slog.Float64("progress_pct", progress.ComputeProgress(bookDurationMs)*100))
	if !progress.IsFinished && importID != "" && absUserID != "" {
		absProgress, err := s.services.ABSImport.FindABSImportProgressByListenUpBook(ctx, importID, absUserID, bookID)
		// gocritic: ifElseChain — switching on three different conditions
		// (err, absProgress nil, absProgress.IsFinished) doesn't translate
		// cleanly to a switch; the chain reads sequentially.
		//nolint:gocritic // ifElseChain: branches gate on different vars; switch would obscure.
		if err != nil {
			s.logger.Warn("failed to find ABS import progress by book",
				slog.String("abs_user_id", absUserID),
				slog.String("book_id", bookID),
				slog.String("error", err.Error()))
			absProgressMatched = false
		} else if absProgress == nil {
			s.logger.Warn("no ABS import progress maps to this book - cannot honor ABS finished status",
				slog.String("import_id", importID),
				slog.String("abs_user_id", absUserID),
				slog.String("book_id", bookID))
			absProgressMatched = false
		} else if absProgress.IsFinished {
			progress.IsFinished = true
			if absProgress.FinishedAt != nil {
				progress.FinishedAt = absProgress.FinishedAt
			} else {
				now := time.Now()
				progress.FinishedAt = &now
			}
			progress.UpdatedAt = time.Now()
			needsSave = true // ABS says finished, must save this
			s.logger.Info("marking book as finished from ABS import",
				slog.String("user_id", userID),
				slog.String("book_id", bookID),
				slog.String("abs_media_id", absProgress.ABSMediaID))
		} else {
			s.logger.Debug("ABS import progress found but not finished",
				slog.String("abs_user_id", absUserID),
				slog.String("abs_media_id", absProgress.ABSMediaID),
				slog.Bool("abs_is_finished", absProgress.IsFinished))
		}
	}

	// Save progress if we have changes
	if !needsSave {
		s.logger.Debug("skipping save - existing progress is newer and no ABS finished status change",
			slog.String("book_id", bookID))
		return absProgressMatched, nil
	}

	if err := s.services.ABSImport.UpsertState(ctx, progress); err != nil {
		return absProgressMatched, fmt.Errorf("upsert state: %w", err)
	}

	// VERIFY: Read back what was actually saved to confirm IsFinished persisted
	savedProgress, verifyErr := s.services.ABSImport.GetState(ctx, userID, bookID)
	if verifyErr != nil {
		s.logger.Error("VERIFY FAILED: could not read back saved progress",
			slog.String("user_id", userID),
			slog.String("book_id", bookID),
			slog.String("error", verifyErr.Error()))
	} else {
		s.logger.Info("VERIFY: read back saved progress",
			slog.String("book_id", bookID),
			slog.Bool("intended_is_finished", progress.IsFinished),
			slog.Bool("saved_is_finished", savedProgress.IsFinished),
			slog.Bool("match", progress.IsFinished == savedProgress.IsFinished))
		if progress.IsFinished != savedProgress.IsFinished {
			s.logger.Error("VERIFY MISMATCH: IsFinished was not saved correctly!",
				slog.String("book_id", bookID),
				slog.Bool("intended", progress.IsFinished),
				slog.Bool("actual", savedProgress.IsFinished))
		}
	}

	s.logger.Debug("rebuilt progress from events",
		slog.String("user_id", userID),
		slog.String("book_id", bookID),
		slog.Int64("position_ms", progress.CurrentPositionMs),
		slog.Float64("progress", progress.ComputeProgress(bookDurationMs)),
		slog.Bool("is_finished", progress.IsFinished))

	return absProgressMatched, nil
}
