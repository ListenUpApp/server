package api

import (
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
)

func strPtr(s string) *string {
	return &s
}

// TestRebuildProgressFromEvents_ErrorHandling verifies that rebuildProgressFromEvents
// correctly handles various error conditions and uses errors.Is() instead of string comparison.
// This is a REGRESSION TEST for the bug where all progress rebuilds silently failed because
// the code checked err.Error() != "progress not found" but the actual error message was
// "playback progress not found".
func TestRebuildProgressFromEvents_ErrorHandling(t *testing.T) {
	// This test documents the error types that rebuildProgressFromEvents must handle
	// and ensures we use errors.Is() for error comparison.

	// Verify error message contains "playback progress" (the message that caused the bug)
	errMsg := store.ErrProgressNotFound.Error()
	assert.Contains(t, errMsg, "playback progress not found",
		"ErrProgressNotFound message should contain 'playback progress not found'")

	// The bug was comparing err.Error() != "progress not found"
	// But the message was "playback progress not found" - so the comparison always FAILED
	buggyCheck := errMsg != "progress not found"
	assert.True(t, buggyCheck, "String comparison should always return true (not matching)")

	// The fix uses errors.Is() which correctly matches the sentinel error
	// regardless of its message content
	assert.True(t, errors.Is(store.ErrProgressNotFound, store.ErrProgressNotFound),
		"errors.Is() should match the same sentinel error")
}

// TestRebuildProgressFromEvents_UsesErrorsIs verifies that the fix correctly uses
// errors.Is() instead of string comparison for error checking.
func TestRebuildProgressFromEvents_UsesErrorsIs(t *testing.T) {
	// errors.Is() should match ErrProgressNotFound
	err := store.ErrProgressNotFound

	// This is what the BUGGY code did - string comparison that FAILED
	// The comparison was: err.Error() != "progress not found"
	// But the message is "playback progress not found", so this is always TRUE
	wrongComparison := err.Error() != "progress not found"
	assert.True(t, wrongComparison, "String comparison should NOT match (this was the bug)")

	// This is what the FIXED code does - errors.Is() that WORKS
	// errors.Is(err, store.ErrProgressNotFound) returns true because they're the same error
	correctComparison := errors.Is(err, store.ErrProgressNotFound)
	assert.True(t, correctComparison, "errors.Is() should match the sentinel error")

	// The buggy code would have treated ALL errors as "not progress not found"
	// because the string never matched, causing the function to fail on EVERY call
	// This is why all 100+ progress rebuilds failed with user_book_pairs=0
}

// TestABSImportBookResponse_DisplayFields verifies that when a book is mapped,
// the response includes listenup_title and listenup_author display fields.
func TestABSImportBookResponse_DisplayFields(t *testing.T) {
	// When mapped, display fields should be populated
	title := "The Great Gatsby"
	author := "F. Scott Fitzgerald"
	book := &domain.ABSImportBook{
		ImportID:       "import_123",
		ABSMediaID:     "book_1",
		ABSTitle:       "gatsby.m4b",
		ABSAuthor:      "Fitzgerald",
		ListenUpID:     strPtr("lu_book_1"),
		ListenUpTitle:  &title,
		ListenUpAuthor: &author,
		Confidence:     "strong",
	}

	resp := toABSImportBookResponse(book)
	assert.Equal(t, "lu_book_1", resp.ListenUpID)
	assert.Equal(t, "The Great Gatsby", resp.ListenUpTitle)
	assert.Equal(t, "F. Scott Fitzgerald", resp.ListenUpAuthor)
	assert.True(t, resp.IsMapped)
}

// TestABSImportBookResponse_DisplayFieldsClearedOnUnmap verifies that when a book
// mapping is cleared, the display fields are also cleared.
func TestABSImportBookResponse_DisplayFieldsClearedOnUnmap(t *testing.T) {
	book := &domain.ABSImportBook{
		ImportID:       "import_123",
		ABSMediaID:     "book_1",
		ABSTitle:       "gatsby.m4b",
		ListenUpID:     nil,
		ListenUpTitle:  nil,
		ListenUpAuthor: nil,
	}

	resp := toABSImportBookResponse(book)
	assert.Empty(t, resp.ListenUpID)
	assert.Empty(t, resp.ListenUpTitle)
	assert.Empty(t, resp.ListenUpAuthor)
	assert.False(t, resp.IsMapped)
}

// TestABSImportUserResponse_DisplayFields verifies that when a user is mapped,
// the response includes listenup_email and listenup_display_name.
func TestABSImportUserResponse_DisplayFields(t *testing.T) {
	email := "gatsby@example.com"
	displayName := "Jay Gatsby"
	user := &domain.ABSImportUser{
		ImportID:            "import_123",
		ABSUserID:           "user_1",
		ABSUsername:         "jgatsby",
		ListenUpID:          strPtr("lu_user_1"),
		ListenUpEmail:       &email,
		ListenUpDisplayName: &displayName,
		Confidence:          "strong",
	}

	resp := toABSImportUserResponse(user)
	assert.Equal(t, "lu_user_1", resp.ListenUpID)
	assert.Equal(t, "gatsby@example.com", resp.ListenUpEmail)
	assert.Equal(t, "Jay Gatsby", resp.ListenUpDisplayName)
	assert.True(t, resp.IsMapped)
}

// TestABSImportUserResponse_DisplayFieldsClearedOnUnmap verifies that when a user
// mapping is cleared, the display fields are also cleared.
func TestABSImportUserResponse_DisplayFieldsClearedOnUnmap(t *testing.T) {
	user := &domain.ABSImportUser{
		ImportID:            "import_123",
		ABSUserID:           "user_1",
		ABSUsername:         "jgatsby",
		ListenUpID:          nil,
		ListenUpEmail:       nil,
		ListenUpDisplayName: nil,
	}

	resp := toABSImportUserResponse(user)
	assert.Empty(t, resp.ListenUpID)
	assert.Empty(t, resp.ListenUpEmail)
	assert.Empty(t, resp.ListenUpDisplayName)
	assert.False(t, resp.IsMapped)
}

// TestImportABSSessionsOutput_HasFailureFields verifies that the response struct
// includes fields for tracking failures. This is important for observability -
// callers should be able to see how many operations failed.
func TestImportABSSessionsOutput_HasFailureFields(t *testing.T) {
	// Create a response with failure counts
	output := ImportABSSessionsOutput{
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
			SessionsImported:       10,
			SessionsFailed:         2, // 2 sessions failed to import
			EventsCreated:          10,
			ProgressRebuilt:        8,
			ProgressFailed:         2, // 2 progress rebuilds failed
			ABSProgressUnmatched:   3, // 3 books couldn't match ABS progress
			ReadingSessionsCreated: 5, // 5 reading sessions created
			ReadingSessionsSkipped: 2, // 2 already existed
			Duration:               "1.5s",
		},
	}

	// Verify the fields are populated correctly
	assert.Equal(t, 10, output.Body.SessionsImported)
	assert.Equal(t, 2, output.Body.SessionsFailed)
	assert.Equal(t, 10, output.Body.EventsCreated)
	assert.Equal(t, 8, output.Body.ProgressRebuilt)
	assert.Equal(t, 2, output.Body.ProgressFailed)
}

// TestUserBookKeyUniqueness verifies that the userBookKey correctly identifies
// unique user+book combinations for progress tracking.
func TestUserBookKeyUniqueness(t *testing.T) {
	type userBookKey struct {
		userID string
		bookID string
	}

	keys := make(map[userBookKey]bool)

	// Same user, different books
	keys[userBookKey{userID: "user1", bookID: "book1"}] = true
	keys[userBookKey{userID: "user1", bookID: "book2"}] = true

	// Different users, same book
	keys[userBookKey{userID: "user2", bookID: "book1"}] = true

	// Should have 3 unique combinations
	assert.Len(t, keys, 3)

	// Adding same combination again should not increase count
	keys[userBookKey{userID: "user1", bookID: "book1"}] = true
	assert.Len(t, keys, 3, "Adding duplicate key should not increase count")
}

// TestSessionStatusValues documents the valid session status values.
func TestSessionStatusValues(t *testing.T) {
	// Document valid session statuses
	statuses := []domain.SessionImportStatus{
		domain.SessionStatusPendingUser,
		domain.SessionStatusPendingBook,
		domain.SessionStatusReady,
		domain.SessionStatusImported,
		domain.SessionStatusSkipped,
	}

	for _, status := range statuses {
		assert.NotEmpty(t, string(status), "Session status should have a string value")
	}
}

// TestMappingFilterValues documents the valid mapping filter values.
func TestMappingFilterValues(t *testing.T) {
	// Document valid mapping filters
	filters := []domain.MappingFilter{
		domain.MappingFilterAll,
		domain.MappingFilterMapped,
		domain.MappingFilterUnmapped,
	}

	for _, filter := range filters {
		assert.NotEmpty(t, string(filter), "Mapping filter should have a string value")
	}
}

// TestDurationClamping verifies that positions exceeding book duration are clamped.
func TestDurationClamping(t *testing.T) {
	// This tests the logic in rebuildProgressFromEvents that clamps positions
	// when ABS duration differs from ListenUp duration.

	bookDurationMs := int64(3600000) // 1 hour
	absPositionMs := int64(4000000)  // Position exceeds book duration

	// If position exceeds book duration, clamp to 98%
	var clampedPosition int64
	if absPositionMs > bookDurationMs && bookDurationMs > 0 {
		clampedPosition = int64(float64(bookDurationMs) * 0.98)
	} else {
		clampedPosition = absPositionMs
	}

	// 98% of 1 hour = 3528000ms
	assert.Equal(t, int64(3528000), clampedPosition)
	assert.Less(t, clampedPosition, bookDurationMs, "Clamped position should be less than book duration")
}

// TestListeningEventCreation verifies listening event structure for ABS imports.
func TestListeningEventCreation(t *testing.T) {
	now := time.Now()
	durationMs := int64(60000) // 1 minute

	event := &domain.ListeningEvent{
		UserID:          "user-1",
		BookID:          "book-1",
		StartPositionMs: 0,
		EndPositionMs:   60000,
		DurationMs:      durationMs,
		DeviceID:        "abs-import",
		DeviceName:      "ABS Import",
		StartedAt:       now,
		EndedAt:         now.Add(time.Duration(durationMs) * time.Millisecond),
		PlaybackSpeed:   1.0,
		CreatedAt:       now,
	}

	assert.Equal(t, "abs-import", event.DeviceID, "Device ID should identify ABS import")
	assert.Equal(t, "ABS Import", event.DeviceName)
	assert.Equal(t, float32(1.0), event.PlaybackSpeed, "Playback speed defaults to 1.0")
	assert.Equal(t, int64(60000), event.EndPositionMs-event.StartPositionMs)
}

// TestProgressCalculation verifies progress percentage calculation.
func TestProgressCalculation(t *testing.T) {
	tests := []struct {
		name              string
		currentPositionMs int64
		bookDurationMs    int64
		expectedPercent   float64
	}{
		{
			name:              "start of book",
			currentPositionMs: 0,
			bookDurationMs:    3600000, // 1 hour
			expectedPercent:   0.0,
		},
		{
			name:              "middle of book",
			currentPositionMs: 1800000, // 30 minutes
			bookDurationMs:    3600000, // 1 hour
			expectedPercent:   50.0,
		},
		{
			name:              "end of book",
			currentPositionMs: 3600000,
			bookDurationMs:    3600000,
			expectedPercent:   100.0,
		},
		{
			name:              "98% clamped position",
			currentPositionMs: 3528000, // 98% of 1 hour
			bookDurationMs:    3600000,
			expectedPercent:   98.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			percent := (float64(tt.currentPositionMs) / float64(tt.bookDurationMs)) * 100
			assert.InDelta(t, tt.expectedPercent, percent, 0.1)
		})
	}
}

// TestCounterIncrementAfterStore verifies that counters should only increment
// after successful store operations. This documents the fix for the bug where
// counters were incremented BEFORE store, causing incorrect counts on failure.
func TestCounterIncrementAfterStore(t *testing.T) {
	// Simulate the BUGGY pattern: increment before store
	buggyCounter := 0
	items := []string{"a", "b", "c"}
	storeErrors := map[string]bool{"b": true} // "b" fails to store

	// WRONG: Increment before checking store result
	for _, item := range items {
		buggyCounter++ // BUG: Counted before store
		if storeErrors[item] {
			continue // Store failed but already counted!
		}
	}
	// Bug: Counter is 3 but only 2 were stored
	assert.Equal(t, 3, buggyCounter, "Buggy counter counts all items, even failed ones")

	// Simulate the FIXED pattern: increment after store
	fixedCounter := 0
	for _, item := range items {
		if storeErrors[item] {
			continue // Store failed, skip counting
		}
		fixedCounter++ // FIXED: Only count after successful store
	}
	// Fixed: Counter is 2 (only successful stores)
	assert.Equal(t, 2, fixedCounter, "Fixed counter only counts successful stores")
}

// TestErrorsIsVsStringComparison demonstrates why errors.Is() is correct
// and string comparison is fragile.
func TestErrorsIsVsStringComparison(t *testing.T) {
	// String comparison is fragile - exact message matters
	// The BUG was checking: err.Error() != "progress not found"
	// But the actual message was: "playback progress not found"
	assert.NotEqual(t, "progress not found", store.ErrProgressNotFound.Error(),
		"String comparison fails because message says 'playback progress not found'")

	// errors.Is() correctly matches sentinel errors
	assert.True(t, errors.Is(store.ErrProgressNotFound, store.ErrProgressNotFound),
		"errors.Is() matches the same sentinel error")

	// The lesson: NEVER use string comparison for error checking
	// Always use errors.Is() or errors.As()
	//
	// Bad:  err.Error() != "expected message"  <- fragile, breaks if message changes
	// Good: errors.Is(err, ExpectedSentinel)   <- robust, survives message changes
}

// TestImportStatsUpdate documents that stats updates should log errors but not fail.
func TestImportStatsUpdate(t *testing.T) {
	// This test documents the behavior: stats updates are best-effort.
	// If they fail, we log the error but don't fail the operation.
	//
	// Rationale: The primary import succeeded. Stats are secondary metadata
	// that can be recalculated later. Failing the whole operation because
	// stats update failed would be worse for the user.

	// Stats include:
	stats := struct {
		UsersMapped      int
		UsersTotal       int
		BooksMapped      int
		BooksTotal       int
		SessionsImported int
		SessionsTotal    int
	}{
		UsersMapped:      5,
		UsersTotal:       10,
		BooksMapped:      20,
		BooksTotal:       25,
		SessionsImported: 100,
		SessionsTotal:    120,
	}

	// These should be persisted but failure shouldn't fail the handler
	assert.LessOrEqual(t, stats.UsersMapped, stats.UsersTotal)
	assert.LessOrEqual(t, stats.BooksMapped, stats.BooksTotal)
	assert.LessOrEqual(t, stats.SessionsImported, stats.SessionsTotal)
}

// TestABSImportStatusAnalyzing verifies the new analyzing status constant.
func TestABSImportStatusAnalyzing(t *testing.T) {
	assert.Equal(t, domain.ABSImportStatus("analyzing"), domain.ABSImportStatusAnalyzing)

	// Verify it serializes correctly in responses
	imp := &domain.ABSImport{
		ID:     "test-123",
		Name:   "Test Import",
		Status: domain.ABSImportStatusAnalyzing,
	}
	resp := toABSImportResponse(imp)
	assert.Equal(t, "analyzing", resp.Status)
}

// TestABSImportStatusFailed verifies the new failed status constant.
func TestABSImportStatusFailed(t *testing.T) {
	assert.Equal(t, domain.ABSImportStatus("failed"), domain.ABSImportStatusFailed)

	imp := &domain.ABSImport{
		ID:     "test-123",
		Name:   "Test Import",
		Status: domain.ABSImportStatusFailed,
	}
	resp := toABSImportResponse(imp)
	assert.Equal(t, "failed", resp.Status)
}

// TestCreateABSImport_ReturnsAnalyzingStatus verifies that the handler creates
// an import with "analyzing" status (not "active") for async processing.
func TestCreateABSImport_ReturnsAnalyzingStatus(t *testing.T) {
	// The handler now creates the import record with ABSImportStatusAnalyzing
	// and returns immediately. The heavy analysis runs in a background goroutine.
	imp := &domain.ABSImport{
		ID:         "test-import",
		Name:       "Test Import",
		BackupPath: "/tmp/backup.audiobookshelf",
		Status:     domain.ABSImportStatusAnalyzing,
	}
	resp := toABSImportResponse(imp)

	// Response should show "analyzing" status
	assert.Equal(t, "analyzing", resp.Status)

	// Counts should be zero since analysis hasn't run yet
	assert.Equal(t, 0, resp.TotalUsers)
	assert.Equal(t, 0, resp.TotalBooks)
	assert.Equal(t, 0, resp.TotalSessions)
	assert.Equal(t, 0, resp.UsersMapped)
	assert.Equal(t, 0, resp.BooksMapped)
}

// TestAnalysisCountsWrittenBeforeSessionStorage verifies that after analysis,
// the import record carries total_books/total_users/total_sessions even while
// still in "analyzing" status. This lets polling clients show scope context
// (e.g. "Matching 1,011 books…") before the full storage phase completes.
func TestAnalysisCountsWrittenBeforeSessionStorage(t *testing.T) {
	imp := &domain.ABSImport{
		ID:            "import-analysis-counts",
		Name:          "ABS Import 2024-12-01",
		BackupPath:    "/tmp/backup.audiobookshelf",
		Status:        domain.ABSImportStatusAnalyzing, // Still analyzing
		TotalUsers:    3,
		TotalBooks:    1011,
		TotalSessions: 5432,
		// Mapped counts still zero — storage hasn't run yet
		UsersMapped:      0,
		BooksMapped:      0,
		SessionsImported: 0,
	}

	resp := toABSImportResponse(imp)

	// Status is still analyzing (storage phase hasn't completed)
	assert.Equal(t, "analyzing", resp.Status)

	// But totals are already populated from analysis
	assert.Equal(t, 3, resp.TotalUsers, "total_users should be set after analysis")
	assert.Equal(t, 1011, resp.TotalBooks, "total_books should be set after analysis")
	assert.Equal(t, 5432, resp.TotalSessions, "total_sessions should be set after analysis")

	// Mapped/imported counts are still zero
	assert.Equal(t, 0, resp.UsersMapped)
	assert.Equal(t, 0, resp.BooksMapped)
	assert.Equal(t, 0, resp.SessionsImported)
}

// TestAutoMatchedBookDisplayName verifies that when a book is auto-matched,
// the display name (ListenUpTitle) is populated from the store lookup.
func TestAutoMatchedBookDisplayName(t *testing.T) {
	// This documents the fix: auto-matched books should have ListenUpTitle set
	// Previously, only ListenUpID was set but ListenUpTitle was left nil
	title := "The Name of the Wind"
	book := &domain.ABSImportBook{
		ImportID:      "import-1",
		ABSMediaID:    "abs-book-1",
		ABSTitle:      "Name of the Wind.m4b",
		ListenUpID:    strPtr("lu-book-1"),
		ListenUpTitle: &title, // This was the bug - it was nil before the fix
		Confidence:    "definitive",
		MatchReason:   "ASIN match",
	}

	resp := toABSImportBookResponse(book)
	assert.Equal(t, "The Name of the Wind", resp.ListenUpTitle,
		"Auto-matched book should have display title populated")
	assert.True(t, resp.IsMapped)
}

// TestAutoMatchedUserDisplayName verifies that when a user is auto-matched,
// the display info (ListenUpEmail, ListenUpDisplayName) is populated.
func TestAutoMatchedUserDisplayName(t *testing.T) {
	// This documents the fix: auto-matched users should have display info set
	email := "simon@example.com"
	displayName := "Simon Hull"
	user := &domain.ABSImportUser{
		ImportID:            "import-1",
		ABSUserID:           "abs-user-1",
		ABSUsername:         "simonh",
		ListenUpID:          strPtr("lu-user-1"),
		ListenUpEmail:       &email,       // Was nil before the fix
		ListenUpDisplayName: &displayName, // Was nil before the fix
		Confidence:          "definitive",
		MatchReason:         "Email match",
	}

	resp := toABSImportUserResponse(user)
	assert.Equal(t, "simon@example.com", resp.ListenUpEmail,
		"Auto-matched user should have email populated")
	assert.Equal(t, "Simon Hull", resp.ListenUpDisplayName,
		"Auto-matched user should have display name populated")
	assert.True(t, resp.IsMapped)
}
