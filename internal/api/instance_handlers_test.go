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

	var envelope testEnvelope[InstanceResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.NotEmpty(t, envelope.Data.ID)
	assert.Equal(t, "Test Server", envelope.Data.Name)
	assert.True(t, envelope.Data.SetupRequired, "Setup should be required before admin user is created")
}

func TestGetInstance_AfterSetup(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Complete setup
	_ = ts.createTestUserAndLogin(t)

	resp := ts.api.Get("/api/v1/instance")

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[InstanceResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.NotEmpty(t, envelope.Data.ID)
	assert.Equal(t, "Test Server", envelope.Data.Name)
	assert.False(t, envelope.Data.SetupRequired, "Setup should not be required after admin user is created")
}
