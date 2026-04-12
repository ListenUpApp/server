package sse

import (
	"encoding/json/v2"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewProgressUpdatedEvent_PopulatesFinishedAt verifies the SSE event carries
// FinishedAt from domain.PlaybackState so the client can stateful-merge completion
// timestamps across devices (server-side contribution to Bug 2).
func TestNewProgressUpdatedEvent_PopulatesFinishedAt(t *testing.T) {
	finishedAt := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	state := &domain.PlaybackState{
		UserID:            "user-1",
		BookID:            "book-1",
		CurrentPositionMs: 3_600_000,
		IsFinished:        true,
		FinishedAt:        &finishedAt,
		StartedAt:         time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		LastPlayedAt:      finishedAt,
		TotalListenTimeMs: 3_600_000,
	}

	evt := NewProgressUpdatedEvent("user-1", state, 7_200_000)

	data, ok := evt.Data.(ProgressUpdatedEventData)
	require.True(t, ok, "event data must be ProgressUpdatedEventData")
	require.NotNil(t, data.FinishedAt, "FinishedAt must be populated when state is finished")
	assert.True(t, data.FinishedAt.Equal(finishedAt), "FinishedAt must equal state.FinishedAt")
}

// TestNewProgressUpdatedEvent_PopulatesStartedAt verifies StartedAt is carried on the
// event so the client's stateful-merge handler can preserve both timestamps on echoes.
func TestNewProgressUpdatedEvent_PopulatesStartedAt(t *testing.T) {
	startedAt := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	state := &domain.PlaybackState{
		UserID:       "user-1",
		BookID:       "book-1",
		StartedAt:    startedAt,
		LastPlayedAt: startedAt,
	}

	evt := NewProgressUpdatedEvent("user-1", state, 7_200_000)

	data, ok := evt.Data.(ProgressUpdatedEventData)
	require.True(t, ok)
	require.NotNil(t, data.StartedAt, "StartedAt must be populated from state")
	assert.True(t, data.StartedAt.Equal(startedAt), "StartedAt must equal state.StartedAt")
}

// TestNewProgressUpdatedEvent_OmitsFinishedAtWhenNil verifies the JSON payload
// omits finished_at when the state has no completion timestamp, so the client
// receives field-absent rather than null.
func TestNewProgressUpdatedEvent_OmitsFinishedAtWhenNil(t *testing.T) {
	state := &domain.PlaybackState{
		UserID:     "user-1",
		BookID:     "book-1",
		IsFinished: false,
		FinishedAt: nil,
		StartedAt:  time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	}

	evt := NewProgressUpdatedEvent("user-1", state, 7_200_000)

	payload, err := json.Marshal(evt.Data)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "finished_at",
		"finished_at must be absent when FinishedAt is nil")
}

// TestProgressUpdatedEventData_JSONContract pins the wire format the client expects.
// Any change to these keys is a breaking contract change for every connected client.
func TestProgressUpdatedEventData_JSONContract(t *testing.T) {
	finishedAt := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	data := ProgressUpdatedEventData{
		BookID:            "book-1",
		CurrentPositionMs: 3_600_000,
		Progress:          0.5,
		TotalListenTimeMs: 3_600_000,
		IsFinished:        true,
		FinishedAt:        &finishedAt,
		StartedAt:         &startedAt,
		LastPlayedAt:      finishedAt,
	}

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))

	for _, key := range []string{
		"book_id",
		"current_position_ms",
		"progress",
		"total_listen_time_ms",
		"is_finished",
		"finished_at",
		"started_at",
		"last_played_at",
	} {
		assert.Contains(t, decoded, key, "wire format must include %q", key)
	}
}
