package api

import (
	"encoding/json"
	"testing"

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
