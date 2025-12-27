package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSyncManifest_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Get auth token
	token := ts.createTestUserAndLogin(t)

	resp := ts.api.Get("/api/v1/sync/manifest",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[SyncManifestResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	// Empty library initially
	assert.NotEmpty(t, envelope.Data.LibraryVersion)
	assert.NotEmpty(t, envelope.Data.Checkpoint)
	assert.Empty(t, envelope.Data.BookIDs)
	assert.Equal(t, 0, envelope.Data.Counts.Books)
}

func TestGetSyncManifest_Unauthorized(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Get("/api/v1/sync/manifest")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestGetSyncManifest_InvalidToken(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Get("/api/v1/sync/manifest",
		"Authorization: Bearer invalid-token")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestGetSyncBooks_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	token := ts.createTestUserAndLogin(t)

	resp := ts.api.Get("/api/v1/sync/books",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[SyncBooksResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	// Empty library initially
	assert.Empty(t, envelope.Data.Books)
	assert.False(t, envelope.Data.HasMore)
}

func TestGetSyncBooks_WithPagination(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	token := ts.createTestUserAndLogin(t)

	resp := ts.api.Get("/api/v1/sync/books?limit=10",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[SyncBooksResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.False(t, envelope.Data.HasMore)
}

func TestGetSyncBooks_Unauthorized(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Get("/api/v1/sync/books")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestGetSyncContributors_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	token := ts.createTestUserAndLogin(t)

	resp := ts.api.Get("/api/v1/sync/contributors",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[SyncContributorsResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	// Empty library initially
	assert.Empty(t, envelope.Data.Contributors)
	assert.False(t, envelope.Data.HasMore)
}

func TestGetSyncContributors_Unauthorized(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Get("/api/v1/sync/contributors")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestGetSyncSeries_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	token := ts.createTestUserAndLogin(t)

	resp := ts.api.Get("/api/v1/sync/series",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[SyncSeriesResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	// Empty library initially
	assert.Empty(t, envelope.Data.Series)
	assert.False(t, envelope.Data.HasMore)
}

func TestGetSyncSeries_Unauthorized(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Get("/api/v1/sync/series")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}
