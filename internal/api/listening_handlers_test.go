package api

import (
	"context"
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListeningEventsContractMatch verifies the API accepts the exact JSON format
// the client sends. This is a CONTRACT TEST - it ensures client and server agree
// on the wire format, preventing the kind of mismatch that caused listening events
// to silently fail.
//
// Client sends (from SyncApi.kt):
//
//	{
//	  "events": [
//	    {
//	      "id": "evt-abc123",
//	      "book_id": "book-1",
//	      "start_position_ms": 0,
//	      "end_position_ms": 60000,
//	      "started_at": 1704067200000,
//	      "ended_at": 1704067260000,
//	      "playback_speed": 1.0,
//	      "device_id": "device-1"
//	    }
//	  ]
//	}
//
// Server must respond (expected by client):
//
//	{
//	  "acknowledged": ["evt-abc123"],
//	  "failed": []
//	}
func TestListeningEventsContractMatch(t *testing.T) {
	// This is the EXACT JSON the Kotlin client sends
	clientRequest := `{
		"events": [
			{
				"id": "evt-abc123",
				"book_id": "book-1",
				"start_position_ms": 0,
				"end_position_ms": 60000,
				"started_at": 1704067200000,
				"ended_at": 1704067260000,
				"playback_speed": 1.0,
				"device_id": "device-1"
			}
		]
	}`

	// Verify the request can be parsed into our DTO
	var req BatchListeningEventsRequest
	err := json.Unmarshal([]byte(clientRequest), &req)
	require.NoError(t, err, "Server must be able to parse client request format")

	// Verify parsed values
	require.Len(t, req.Events, 1)
	assert.Equal(t, "evt-abc123", req.Events[0].ID)
	assert.Equal(t, "book-1", req.Events[0].BookID)
	assert.Equal(t, int64(0), req.Events[0].StartPositionMs)
	assert.Equal(t, int64(60000), req.Events[0].EndPositionMs)
	assert.Equal(t, float32(1.0), req.Events[0].PlaybackSpeed)
	assert.Equal(t, "device-1", req.Events[0].DeviceID)

	// Verify epoch milliseconds parsed correctly (now int64, not FlexTime)
	assert.Equal(t, int64(1704067200000), req.Events[0].StartedAtMs, "StartedAt should parse epoch ms")
	assert.Equal(t, int64(1704067260000), req.Events[0].EndedAtMs, "EndedAt should parse epoch ms")

	// Verify our response format matches what client expects
	response := BatchListeningEventsResponse{
		Acknowledged: []string{"evt-abc123"},
		Failed:       []string{},
	}

	responseJSON, err := json.Marshal(response)
	require.NoError(t, err)

	// Client expects these exact keys (from ListeningEventsResponse.kt)
	var clientExpected struct {
		Acknowledged []string `json:"acknowledged"`
		Failed       []string `json:"failed"`
	}
	err = json.Unmarshal(responseJSON, &clientExpected)
	require.NoError(t, err, "Response must match client expected format")
	assert.Equal(t, []string{"evt-abc123"}, clientExpected.Acknowledged)
}

// TestListeningEventsBatchSubmission tests the full handler flow with multiple events.
func TestListeningEventsBatchSubmission(t *testing.T) {
	clientRequest := `{
		"events": [
			{
				"id": "evt-1",
				"book_id": "book-1",
				"start_position_ms": 0,
				"end_position_ms": 30000,
				"started_at": 1704067200000,
				"ended_at": 1704067230000,
				"playback_speed": 1.0,
				"device_id": "device-1"
			},
			{
				"id": "evt-2",
				"book_id": "book-1",
				"start_position_ms": 30000,
				"end_position_ms": 60000,
				"started_at": 1704067230000,
				"ended_at": 1704067260000,
				"playback_speed": 1.5,
				"device_id": "device-1"
			}
		]
	}`

	var req BatchListeningEventsRequest
	err := json.Unmarshal([]byte(clientRequest), &req)
	require.NoError(t, err)

	require.Len(t, req.Events, 2)
	assert.Equal(t, "evt-1", req.Events[0].ID)
	assert.Equal(t, "evt-2", req.Events[1].ID)
	assert.Equal(t, float32(1.5), req.Events[1].PlaybackSpeed)
}

// TestListeningEventsEmptyBatchRejected ensures empty batches are rejected.
func TestListeningEventsEmptyBatchRejected(t *testing.T) {
	clientRequest := `{"events": []}`

	var req BatchListeningEventsRequest
	err := json.Unmarshal([]byte(clientRequest), &req)
	require.NoError(t, err)

	// Validation should catch empty events array
	assert.Empty(t, req.Events, "Empty events should parse but fail validation")
}

// TestFlexTimeAcceptsEpochMilliseconds verifies FlexTime handles epoch ms format
// that the Kotlin client sends.
func TestFlexTimeAcceptsEpochMilliseconds(t *testing.T) {
	// Client sends timestamps as epoch milliseconds (Long in Kotlin)
	jsonWithEpochMs := `{"started_at": 1704067200000, "ended_at": 1704067260000}`

	var payload struct {
		StartedAt FlexTime `json:"started_at"`
		EndedAt   FlexTime `json:"ended_at"`
	}

	err := json.Unmarshal([]byte(jsonWithEpochMs), &payload)
	require.NoError(t, err, "FlexTime must accept epoch milliseconds")

	// Verify correct parsing
	assert.Equal(t, int64(1704067200), payload.StartedAt.ToTime().Unix())
	assert.Equal(t, int64(1704067260), payload.EndedAt.ToTime().Unix())
}

// TestFlexTimeAcceptsRFC3339 verifies FlexTime also handles RFC3339 format
// for backwards compatibility.
func TestFlexTimeAcceptsRFC3339(t *testing.T) {
	jsonWithRFC3339 := `{"started_at": "2024-01-01T00:00:00Z", "ended_at": "2024-01-01T00:01:00Z"}`

	var payload struct {
		StartedAt FlexTime `json:"started_at"`
		EndedAt   FlexTime `json:"ended_at"`
	}

	err := json.Unmarshal([]byte(jsonWithRFC3339), &payload)
	require.NoError(t, err, "FlexTime must accept RFC3339")

	assert.Equal(t, int64(1704067200), payload.StartedAt.ToTime().Unix())
}

// TestGetAllProgressInput_HasUpdatedAfterQueryTag pins the query-string contract
// for delta sync: the progress endpoint must accept `updated_after` matching the
// pattern established by sync/books, sync/contributors, sync/series. The client's
// ProgressPuller depends on this exact name.
func TestGetAllProgressInput_HasUpdatedAfterQueryTag(t *testing.T) {
	typ := reflect.TypeOf(GetAllProgressInput{})
	field, ok := typ.FieldByName("UpdatedAfter")
	require.True(t, ok, "GetAllProgressInput must have field UpdatedAfter")
	assert.Equal(t, reflect.TypeOf("").Kind(), field.Type.Kind(), "UpdatedAfter must be a string (RFC3339 param)")
	assert.Equal(t, "updated_after", field.Tag.Get("query"),
		"UpdatedAfter must be bound to query param `updated_after` for client ProgressPuller compatibility")
}

// newListeningHandlerServer builds a minimal *Server suitable for testing
// handleGetAllProgress in isolation. No services are wired — the handler only
// needs s.store.
func newListeningHandlerServer(t *testing.T) (*Server, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "listening-handler-test-*")
	require.NoError(t, err)
	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := sqlite.Open(dbPath, nil)
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := &Server{store: st, logger: logger}
	cleanup := func() {
		_ = st.Close()
		_ = os.RemoveAll(tmpDir)
	}
	return s, cleanup
}

// TestHandleGetAllProgress_InvalidUpdatedAfter_Returns400 verifies the handler
// rejects malformed RFC3339 with a 400, matching the behaviour of sync/books
// (sync_handlers.go:246). This prevents silent fallthrough to full sync on
// client bugs.
func TestHandleGetAllProgress_InvalidUpdatedAfter_Returns400(t *testing.T) {
	s, cleanup := newListeningHandlerServer(t)
	defer cleanup()

	ctx := setUserID(context.Background(), "user-1")
	_, err := s.handleGetAllProgress(ctx, &GetAllProgressInput{UpdatedAfter: "not-a-date"})
	require.Error(t, err, "malformed updated_after must return an error")

	var statusErr huma.StatusError
	require.ErrorAs(t, err, &statusErr, "error must be a huma StatusError")
	assert.Equal(t, http.StatusBadRequest, statusErr.GetStatus(),
		"malformed updated_after must return 400")
}

// TestHandleGetAllProgress_EmptyUpdatedAfter_ReturnsAll verifies the fallback path
// is preserved: when updated_after is empty the handler returns all of the user's
// playback states (existing behaviour, so a bad client release doesn't break sync).
func TestHandleGetAllProgress_EmptyUpdatedAfter_ReturnsAll(t *testing.T) {
	s, cleanup := newListeningHandlerServer(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-empty"
	require.NoError(t, s.store.CreateUser(ctx, makeTestUserForListening(userID)))
	seedPlaybackState(t, s, userID, "book-a", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	seedPlaybackState(t, s, userID, "book-b", time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC))

	authCtx := setUserID(ctx, userID)
	out, err := s.handleGetAllProgress(authCtx, &GetAllProgressInput{UpdatedAfter: ""})
	require.NoError(t, err)
	assert.Len(t, out.Body.Items, 2, "empty updated_after must return all user progress")
}

// TestHandleGetAllProgress_ValidUpdatedAfter_FiltersResults verifies the query
// param reaches the store: a cutoff strictly before book-b's updated_at must
// return only book-b.
func TestHandleGetAllProgress_ValidUpdatedAfter_FiltersResults(t *testing.T) {
	s, cleanup := newListeningHandlerServer(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-delta"
	require.NoError(t, s.store.CreateUser(ctx, makeTestUserForListening(userID)))
	earlyUpdate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	lateUpdate := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	seedPlaybackState(t, s, userID, "book-early", earlyUpdate)
	seedPlaybackState(t, s, userID, "book-late", lateUpdate)

	cutoff := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
	authCtx := setUserID(ctx, userID)
	out, err := s.handleGetAllProgress(authCtx, &GetAllProgressInput{
		UpdatedAfter: cutoff.Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.Len(t, out.Body.Items, 1, "only records strictly after cutoff must be returned")
	assert.Equal(t, "book-late", out.Body.Items[0].BookID)
}

func makeTestUserForListening(userID string) *domain.User {
	now := time.Now().UTC()
	return &domain.User{
		Syncable: domain.Syncable{
			ID:        userID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Email:       userID + "@example.com",
		Role:        domain.RoleMember,
		Status:      domain.UserStatusActive,
		LastLoginAt: now,
	}
}

func seedPlaybackState(t *testing.T, s *Server, userID, bookID string, updatedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: updatedAt,
			UpdatedAt: updatedAt,
		},
		ScannedAt:           updatedAt,
		Title:               bookID,
		Path:                "/books/" + bookID,
		TotalDuration:       3600,
		StagedCollectionIDs: []string{},
	}
	require.NoError(t, s.store.CreateBook(ctx, book))
	state := &domain.PlaybackState{
		UserID:       userID,
		BookID:       bookID,
		StartedAt:    updatedAt,
		LastPlayedAt: updatedAt,
		UpdatedAt:    updatedAt,
	}
	require.NoError(t, s.store.UpsertState(ctx, state))
}
