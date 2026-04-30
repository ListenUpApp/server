package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/backup/abs"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// runImportAnalysis is launched by importJobManager.Submit. The supplied ctx
// is canceled on server shutdown so in-flight store calls return promptly
// instead of writing to a database that is being closed. ABSImportStatus has
// no dedicated canceled state, so on cancellation we log and mark the
// import as failed for visibility — the user can re-run the analysis.
func (s *Server) runImportAnalysis(ctx context.Context, importID, backupPath string) {
	// finalizeCtx returns a context safe for the final status write. If ctx
	// is already canceled (shutdown path) we use a short detached context
	// so the failure can still be recorded; otherwise we propagate ctx.
	finalizeCtx := func() (context.Context, context.CancelFunc) {
		if ctx.Err() == nil {
			return ctx, func() {}
		}
		return context.WithTimeout(context.Background(), 5*time.Second)
	}

	setFailed := func(err error) {
		s.logger.Error("import analysis failed",
			slog.String("import_id", importID),
			slog.String("error", err.Error()))
		fctx, cancel := finalizeCtx()
		defer cancel()
		imp, getErr := s.store.GetABSImport(fctx, importID)
		if getErr != nil {
			s.logger.Error("failed to get import for status update", slog.String("error", getErr.Error()))
			return
		}
		imp.Status = domain.ABSImportStatusFailed
		imp.UpdatedAt = time.Now()
		if updateErr := s.store.UpdateABSImport(fctx, imp); updateErr != nil {
			s.logger.Error("failed to update import status to failed", slog.String("error", updateErr.Error()))
		}
	}

	// checkCancel records the import as failed and returns true if ctx has
	// been canceled (e.g. by server shutdown). Called between phases so
	// long-running loops bail out promptly instead of pushing more writes
	// at a closing store.
	checkCancel := func() bool {
		if err := ctx.Err(); err != nil {
			s.logger.Warn("import analysis canceled",
				slog.String("import_id", importID),
				slog.String("error", err.Error()))
			setFailed(err)
			return true
		}
		return false
	}

	// Parse the backup
	backup, err := abs.Parse(ctx, backupPath)
	if err != nil {
		setFailed(fmt.Errorf("failed to parse ABS backup: %w", err))
		return
	}

	// Run initial analysis to get matches
	opts := abs.AnalysisOptions{
		MatchByEmail:    true,
		MatchByPath:     true,
		FuzzyMatchBooks: true,
		FuzzyThreshold:  0.85,
		UserMappings:    make(map[string]string),
		BookMappings:    make(map[string]string),
	}
	analyzer := abs.NewAnalyzer(s.store, s.logger, opts)
	analysis, err := analyzer.Analyze(ctx, backup)
	if err != nil {
		setFailed(fmt.Errorf("analysis failed: %w", err))
		return
	}

	if checkCancel() {
		return
	}

	// Write total counts to import record immediately so polling clients
	// can show scope (e.g. "Matching 1,011 books…") during the storage phase.
	if imp, err := s.store.GetABSImport(ctx, importID); err == nil {
		imp.TotalUsers = analysis.TotalUsers
		imp.TotalBooks = analysis.TotalBooks
		imp.TotalSessions = analysis.TotalSessions
		imp.UpdatedAt = time.Now()
		if err := s.store.UpdateABSImport(ctx, imp); err != nil {
			s.logger.Error("failed to write analysis counts to import record",
				slog.String("import_id", importID),
				slog.String("error", err.Error()))
		}
	}

	// Store all parsed users with analysis results
	usersMapped := 0
	for _, um := range analysis.UserMatches {
		if checkCancel() {
			return
		}
		user := &domain.ABSImportUser{
			ImportID:      importID,
			ABSUserID:     um.ABSUser.ID,
			ABSUsername:   um.ABSUser.Username,
			ABSEmail:      um.ABSUser.Email,
			SessionCount:  len(um.ABSUser.Progress),
			TotalListenMs: calculateUserListenTime(um.ABSUser.Progress),
			Confidence:    um.Confidence.String(),
			MatchReason:   um.MatchReason,
			Suggestions:   extractUserSuggestionIDs(um.Suggestions),
		}

		// Apply auto-match if confidence is strong enough
		wasAutoMapped := false
		if um.Confidence.ShouldAutoImport() && um.ListenUpID != "" {
			user.ListenUpID = &um.ListenUpID
			now := time.Now()
			user.MappedAt = &now
			wasAutoMapped = true

			// Resolve display info for auto-matched user
			luUser, err := s.store.GetUser(ctx, um.ListenUpID)
			if err == nil {
				if luUser.Email != "" {
					user.ListenUpEmail = &luUser.Email
				}
				if luUser.DisplayName != "" {
					user.ListenUpDisplayName = &luUser.DisplayName
				}
			} else {
				s.logger.Warn("failed to resolve display info for auto-matched user",
					slog.String("listenup_id", um.ListenUpID),
					slog.String("error", err.Error()))
			}
		}

		if err := s.store.CreateABSImportUser(ctx, user); err != nil {
			s.logger.Error("failed to store import user",
				slog.String("abs_user_id", um.ABSUser.ID),
				slog.String("error", err.Error()))
			continue
		}

		// Store user's media progress entries
		progressStored := 0
		finishedStored := 0
		for _, mp := range um.ABSUser.Progress {
			if !mp.IsBook() {
				continue
			}
			if mp.LibraryItemID == "" {
				s.logger.Warn("skipping media progress with empty library item ID",
					slog.String("abs_user_id", um.ABSUser.ID),
					slog.String("progress_id", mp.ID))
				continue
			}
			progress := &domain.ABSImportProgress{
				ImportID:    importID,
				ABSUserID:   um.ABSUser.ID,
				ABSMediaID:  mp.LibraryItemID,
				CurrentTime: int64(mp.CurrentTime * 1000),
				Duration:    int64(mp.Duration * 1000),
				Progress:    mp.Progress,
				IsFinished:  mp.IsFinished,
				LastUpdate:  mp.LastUpdateTime(),
				Status:      domain.SessionStatusPendingBook,
			}
			if mp.FinishedAt > 0 {
				finishedAt := time.UnixMilli(mp.FinishedAt)
				progress.FinishedAt = &finishedAt
			}
			if err := s.store.CreateABSImportProgress(ctx, progress); err != nil {
				s.logger.Error("failed to store import progress",
					slog.String("abs_user_id", um.ABSUser.ID),
					slog.String("abs_media_id", mp.LibraryItemID),
					slog.String("error", err.Error()))
			} else {
				progressStored++
				if mp.IsFinished {
					finishedStored++
				}
			}
		}
		s.logger.Info("stored ABS import progress for user",
			slog.String("abs_user_id", um.ABSUser.ID),
			slog.String("abs_username", um.ABSUser.Username),
			slog.Int("progress_stored", progressStored),
			slog.Int("finished_stored", finishedStored))

		if wasAutoMapped {
			usersMapped++
		}
	}

	// Store all parsed books with analysis results
	booksMapped := 0
	for _, bm := range analysis.BookMatches {
		if checkCancel() {
			return
		}
		book := &domain.ABSImportBook{
			ImportID:      importID,
			ABSMediaID:    bm.ABSItem.MediaID,
			ABSTitle:      bm.ABSItem.Media.Metadata.Title,
			ABSAuthor:     bm.ABSItem.Media.Metadata.PrimaryAuthor(),
			ABSDurationMs: bm.ABSItem.Media.DurationMs(),
			ABSASIN:       bm.ABSItem.Media.Metadata.ASIN,
			ABSISBN:       bm.ABSItem.Media.Metadata.ISBN,
			SessionCount:  countSessionsForBook(backup.Sessions, bm.ABSItem.MediaID),
			Confidence:    bm.Confidence.String(),
			MatchReason:   bm.MatchReason,
			Suggestions:   extractBookSuggestionIDs(bm.Suggestions),
		}

		wasAutoMapped := false
		if bm.Confidence.ShouldAutoImport() && bm.ListenUpID != "" {
			book.ListenUpID = &bm.ListenUpID
			now := time.Now()
			book.MappedAt = &now
			wasAutoMapped = true

			// Resolve display info for auto-matched book
			luBook, err := s.store.GetBook(ctx, bm.ListenUpID, "")
			if err == nil {
				if luBook.Title != "" {
					book.ListenUpTitle = &luBook.Title
				}
			} else {
				s.logger.Warn("failed to resolve display info for auto-matched book",
					slog.String("listenup_id", bm.ListenUpID),
					slog.String("error", err.Error()))
			}
		}

		if err := s.store.CreateABSImportBook(ctx, book); err != nil {
			s.logger.Error("failed to store import book",
				slog.String("abs_media_id", bm.ABSItem.MediaID),
				slog.String("title", bm.ABSItem.Media.Metadata.Title),
				slog.String("error", err.Error()))
			continue
		}

		if wasAutoMapped {
			booksMapped++
		}
	}

	s.logger.Info("import creation: book mapping summary",
		slog.Int("total_books_in_abs", len(analysis.BookMatches)),
		slog.Int("auto_mapped_to_listenup", booksMapped),
		slog.Int("unmapped_books", len(analysis.BookMatches)-booksMapped))

	// Build book media ID lookup for session normalization
	bookMediaIDLookup := make(map[string]string)
	for _, bm := range analysis.BookMatches {
		bookMediaIDLookup[bm.ABSItem.MediaID] = bm.ABSItem.MediaID
		if bm.ABSItem.ID != "" && bm.ABSItem.ID != bm.ABSItem.MediaID {
			bookMediaIDLookup[bm.ABSItem.ID] = bm.ABSItem.MediaID
		}
	}

	// Store all sessions
	sessionsStored := 0
	sessionsNormalized := 0
	sessionsUnmatched := 0
	unmatchedSample := make([]string, 0, 5)
	for _, session := range backup.Sessions {
		if checkCancel() {
			return
		}
		absMediaID := session.LibraryItemID
		if normalizedID, found := bookMediaIDLookup[absMediaID]; found {
			if normalizedID != absMediaID {
				sessionsNormalized++
			}
			absMediaID = normalizedID
		} else {
			sessionsUnmatched++
			if len(unmatchedSample) < 5 {
				unmatchedSample = append(unmatchedSample, absMediaID)
			}
		}

		sess := &domain.ABSImportSession{
			ImportID:      importID,
			ABSSessionID:  session.ID,
			ABSUserID:     session.UserID,
			ABSMediaID:    absMediaID,
			StartTime:     session.StartedAtTime(),
			Duration:      session.DurationMs(),
			StartPosition: session.StartPositionMs(),
			EndPosition:   session.EndPositionMs(),
			Status:        domain.SessionStatusPendingUser,
		}

		if err := s.store.CreateABSImportSession(ctx, sess); err != nil {
			s.logger.Error("failed to store import session",
				slog.String("session_id", session.ID),
				slog.String("user_id", session.UserID),
				slog.String("error", err.Error()))
			continue
		}
		sessionsStored++
	}

	if sessionsUnmatched > 0 {
		s.logger.Warn("sessions with unmatched ABSMediaID",
			slog.Int("unmatched_count", sessionsUnmatched),
			slog.Int("total_sessions", len(backup.Sessions)),
			slog.Any("unmatched_sample", unmatchedSample))
	}

	if checkCancel() {
		return
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatuses(ctx, importID); err != nil {
		s.logger.Error("failed to recalculate session statuses",
			slog.String("import_id", importID),
			slog.String("error", err.Error()))
	}

	// Update import to active with counts
	imp, err := s.store.GetABSImport(ctx, importID)
	if err != nil {
		setFailed(fmt.Errorf("failed to get import for final update: %w", err))
		return
	}
	imp.Status = domain.ABSImportStatusAnalyzed
	imp.TotalUsers = analysis.TotalUsers
	imp.TotalBooks = analysis.TotalBooks
	imp.TotalSessions = analysis.TotalSessions
	imp.UsersMapped = usersMapped
	imp.BooksMapped = booksMapped
	imp.UpdatedAt = time.Now()
	if err := s.store.UpdateABSImport(ctx, imp); err != nil {
		s.logger.Error("failed to update import to active",
			slog.String("import_id", importID),
			slog.String("error", err.Error()))
		return
	}

	s.logger.Info("import analysis completed",
		slog.String("import_id", importID),
		slog.Int("total_users", analysis.TotalUsers),
		slog.Int("total_books", analysis.TotalBooks),
		slog.Int("total_sessions", analysis.TotalSessions),
		slog.Int("users_mapped", usersMapped),
		slog.Int("books_mapped", booksMapped),
		slog.Int("sessions_stored", sessionsStored))
}

// === Analysis helpers ===

func calculateUserListenTime(progress []abs.MediaProgress) int64 {
	var total int64
	for _, p := range progress {
		total += int64(p.CurrentTime * 1000) // Convert seconds to milliseconds
	}
	return total
}

func countSessionsForBook(sessions []abs.Session, mediaID string) int {
	count := 0
	for _, s := range sessions {
		if s.LibraryItemID == mediaID {
			count++
		}
	}
	return count
}

func extractUserSuggestionIDs(suggestions []abs.UserSuggestion) []string {
	ids := make([]string, len(suggestions))
	for i, s := range suggestions {
		ids[i] = s.UserID
	}
	return ids
}

func extractBookSuggestionIDs(suggestions []abs.BookSuggestion) []string {
	ids := make([]string, len(suggestions))
	for i, s := range suggestions {
		ids[i] = s.BookID
	}
	return ids
}
