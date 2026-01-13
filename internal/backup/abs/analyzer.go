package abs

import (
	"context"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/store"
)

// Analyzer examines an ABS backup and produces a matching analysis.
// The analysis shows what can be automatically imported and what needs
// admin attention before import.
type Analyzer struct {
	store   *store.Store
	matcher *Matcher
	logger  *slog.Logger
}

// NewAnalyzer creates an analyzer with the given store and options.
func NewAnalyzer(s *store.Store, logger *slog.Logger, opts AnalysisOptions) *Analyzer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Analyzer{
		store:   s,
		matcher: NewMatcher(s, logger, opts),
		logger:  logger,
	}
}

// Analyze examines an ABS backup and produces a detailed analysis.
// This is a read-only operation that doesn't modify any data.
func (a *Analyzer) Analyze(ctx context.Context, backup *Backup) (*AnalysisResult, error) {
	start := time.Now()
	a.logger.Info("starting ABS analysis",
		"users", len(backup.ImportableUsers()),
		"books", len(backup.BookItems()),
		"sessions", len(backup.BookSessions()),
	)

	result := &AnalysisResult{
		BackupPath: backup.Path,
		AnalyzedAt: time.Now(),
	}

	// 1. Analyze users
	userStart := time.Now()
	a.analyzeUsers(ctx, backup, result)
	a.logger.Info("analyzed users", "matched", result.UsersMatched, "pending", result.UsersPending, "duration", time.Since(userStart))

	// 2. Analyze books (only books, not podcasts)
	bookStart := time.Now()
	a.analyzeBooks(ctx, backup, result)
	a.logger.Info("analyzed books", "matched", result.BooksMatched, "pending", result.BooksPending, "duration", time.Since(bookStart))

	// 3. Analyze sessions (what can be imported)
	sessionStart := time.Now()
	a.analyzeSessions(ctx, backup, result)
	a.logger.Info("analyzed sessions", "ready", result.SessionsReady, "pending", result.SessionsPending, "duration", time.Since(sessionStart))

	// 4. Analyze progress records
	progressStart := time.Now()
	a.analyzeProgress(ctx, backup, result)
	a.logger.Info("analyzed progress", "ready", result.ProgressReady, "pending", result.ProgressPending, "duration", time.Since(progressStart))

	a.logger.Info("ABS analysis complete", "total_duration", time.Since(start))

	return result, nil
}

// analyzeUsers matches ABS users to ListenUp users.
func (a *Analyzer) analyzeUsers(ctx context.Context, backup *Backup, result *AnalysisResult) {
	importableUsers := backup.ImportableUsers()
	result.TotalUsers = len(importableUsers)

	for i := range importableUsers {
		user := &importableUsers[i]
		match := a.matcher.MatchUser(ctx, user)
		result.UserMatches = append(result.UserMatches, *match)

		if match.Confidence.ShouldAutoImport() {
			result.UsersMatched++
		} else {
			result.UsersPending++
		}
	}
}

// analyzeBooks matches ABS library items to ListenUp books.
func (a *Analyzer) analyzeBooks(ctx context.Context, backup *Backup, result *AnalysisResult) {
	bookItems := backup.BookItems()
	result.TotalBooks = len(bookItems)

	// Pre-load all ListenUp books into memory for fast matching
	a.logger.Info("pre-loading ListenUp books for matching")
	loadStart := time.Now()
	a.matcher.PreloadBooks(ctx)
	a.logger.Info("books pre-loaded", "duration", time.Since(loadStart))

	for i := range bookItems {
		item := &bookItems[i]
		match := a.matcher.MatchBookFast(item)
		result.BookMatches = append(result.BookMatches, *match)

		if match.Confidence.ShouldAutoImport() {
			result.BooksMatched++
		} else {
			result.BooksPending++
		}

		// Log progress every 100 books
		if (i+1)%100 == 0 {
			a.logger.Info("book matching progress", "processed", i+1, "total", len(bookItems))
		}
	}
}

// analyzeSessions determines how many sessions can be imported.
func (a *Analyzer) analyzeSessions(_ context.Context, backup *Backup, result *AnalysisResult) {
	bookSessions := backup.BookSessions()
	result.TotalSessions = len(bookSessions)

	// Build lookup maps for matched users and books
	userMap := make(map[string]bool) // ABS userID -> is matched
	for _, m := range result.UserMatches {
		if m.Confidence.ShouldAutoImport() {
			userMap[m.ABSUser.ID] = true
		}
	}

	// Note: Sessions reference mediaItemId (books.id), not libraryItems.id
	// So we use ABSItem.MediaID for matching
	bookMap := make(map[string]bool) // ABS mediaId -> is matched
	for _, m := range result.BookMatches {
		if m.Confidence.ShouldAutoImport() {
			bookMap[m.ABSItem.MediaID] = true
		}
	}

	// Count sessions that can be imported
	// session.LibraryItemID actually contains mediaItemId from parsing
	for _, session := range bookSessions {
		if userMap[session.UserID] && bookMap[session.LibraryItemID] {
			result.SessionsReady++
		} else {
			result.SessionsPending++
		}
	}
}

// analyzeProgress determines how many progress records can be imported.
func (a *Analyzer) analyzeProgress(_ context.Context, backup *Backup, result *AnalysisResult) {
	// Build lookup maps for matched users and books
	userMap := make(map[string]bool)
	for _, m := range result.UserMatches {
		if m.Confidence.ShouldAutoImport() {
			userMap[m.ABSUser.ID] = true
		}
	}

	// Note: Progress references mediaItemId (books.id), not libraryItems.id
	// So we use ABSItem.MediaID for matching
	bookMap := make(map[string]bool)
	for _, m := range result.BookMatches {
		if m.Confidence.ShouldAutoImport() {
			bookMap[m.ABSItem.MediaID] = true
		}
	}

	// Count progress records per user
	for _, user := range backup.ImportableUsers() {
		if !userMap[user.ID] {
			continue
		}

		for _, progress := range user.Progress {
			if !progress.IsBook() {
				continue
			}

			if bookMap[progress.LibraryItemID] {
				result.ProgressReady++
			} else {
				result.ProgressPending++
			}
		}
	}
}

// BuildFinalMappings combines auto-matched IDs with admin-specified mappings.
// Returns complete user and book mappings for import.
// Note: Book mappings use MediaID (books.id) as the key since that's what sessions reference.
func BuildFinalMappings(analysis *AnalysisResult, adminMappings ImportOptions) (userMap, bookMap map[string]string) {
	userMap = make(map[string]string)
	bookMap = make(map[string]string)

	// Start with admin-specified mappings (highest priority)
	for absID, listenUpID := range adminMappings.UserMappings {
		userMap[absID] = listenUpID
	}
	for absID, listenUpID := range adminMappings.BookMappings {
		bookMap[absID] = listenUpID
	}

	// Add auto-matched users that weren't overridden
	for _, match := range analysis.UserMatches {
		if match.Confidence.ShouldAutoImport() && match.ListenUpID != "" {
			if _, exists := userMap[match.ABSUser.ID]; !exists {
				userMap[match.ABSUser.ID] = match.ListenUpID
			}
		}
	}

	// Add auto-matched books that weren't overridden
	// Use MediaID as the key since sessions reference mediaItemId (books.id)
	for _, match := range analysis.BookMatches {
		if match.Confidence.ShouldAutoImport() && match.ListenUpID != "" {
			if _, exists := bookMap[match.ABSItem.MediaID]; !exists {
				bookMap[match.ABSItem.MediaID] = match.ListenUpID
			}
		}
	}

	return userMap, bookMap
}
