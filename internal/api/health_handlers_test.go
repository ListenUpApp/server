package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheck_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

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
