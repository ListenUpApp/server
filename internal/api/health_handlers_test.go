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

	var healthResp HealthResponse
	err := json.Unmarshal(resp.Body.Bytes(), &healthResp)
	require.NoError(t, err)

	assert.Equal(t, "healthy", healthResp.Status)
}
