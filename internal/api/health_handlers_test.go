package api

import (
	"encoding/json/v2"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheck_Success(t *testing.T) {
	t.Parallel()
	ts := setupTestServer(t)

	resp := ts.api.Get("/health")

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[HealthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	// Status can be "healthy" or "degraded" depending on test setup
	// (minimal test server doesn't include search/SSE services)
	assert.Contains(t, []string{"healthy", "degraded"}, envelope.Data.Status)
	assert.NotEmpty(t, envelope.Data.Components)
	assert.Contains(t, envelope.Data.Components, "database")
}

func TestHealthCheck_AsyncIndexerComponentPresent(t *testing.T) {
	t.Parallel()
	ts := setupTestServer(t)

	resp := ts.api.Get("/health")
	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[HealthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	// The component must exist in the response. Status may be degraded
	// because setupTestServer doesn't wire the actual indexer.
	if _, ok := envelope.Data.Components["async_indexer"]; !ok {
		t.Error("expected async_indexer in Components map")
	}
}

func TestHealthCheck_WorkerComponentsPresent(t *testing.T) {
	t.Parallel()
	ts := setupTestServer(t)

	resp := ts.api.Get("/health")
	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[HealthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	for _, name := range []string{"file_watcher", "session_cleanup", "event_log_cleanup", "import_jobs"} {
		if _, ok := envelope.Data.Components[name]; !ok {
			t.Errorf("expected %s in Components map", name)
		}
	}
}
