package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetInstance_BeforeSetup(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Get("/api/v1/instance")

	assert.Equal(t, http.StatusOK, resp.Code)

	var instanceResp InstanceResponse
	err := json.Unmarshal(resp.Body.Bytes(), &instanceResp)
	require.NoError(t, err)

	assert.NotEmpty(t, instanceResp.ID)
	assert.Equal(t, "Test Server", instanceResp.Name)
	assert.True(t, instanceResp.SetupRequired, "Setup should be required before admin user is created")
}

func TestGetInstance_AfterSetup(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Complete setup
	_ = ts.createTestUserAndLogin(t)

	resp := ts.api.Get("/api/v1/instance")

	assert.Equal(t, http.StatusOK, resp.Code)

	var instanceResp InstanceResponse
	err := json.Unmarshal(resp.Body.Bytes(), &instanceResp)
	require.NoError(t, err)

	assert.NotEmpty(t, instanceResp.ID)
	assert.Equal(t, "Test Server", instanceResp.Name)
	assert.False(t, instanceResp.SetupRequired, "Setup should not be required after admin user is created")
}
