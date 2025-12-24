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

	var manifestResp SyncManifestResponse
	err := json.Unmarshal(resp.Body.Bytes(), &manifestResp)
	require.NoError(t, err)

	// Empty library initially
	assert.NotEmpty(t, manifestResp.LibraryVersion)
	assert.NotEmpty(t, manifestResp.Checkpoint)
	assert.Empty(t, manifestResp.BookIDs)
	assert.Equal(t, 0, manifestResp.Counts.Books)
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

	var booksResp SyncBooksResponse
	err := json.Unmarshal(resp.Body.Bytes(), &booksResp)
	require.NoError(t, err)

	// Empty library initially
	assert.Empty(t, booksResp.Books)
	assert.False(t, booksResp.HasMore)
}

func TestGetSyncBooks_WithPagination(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	token := ts.createTestUserAndLogin(t)

	resp := ts.api.Get("/api/v1/sync/books?limit=10",
		"Authorization: Bearer "+token)

	assert.Equal(t, http.StatusOK, resp.Code)

	var booksResp SyncBooksResponse
	err := json.Unmarshal(resp.Body.Bytes(), &booksResp)
	require.NoError(t, err)

	assert.False(t, booksResp.HasMore)
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

	var contribResp SyncContributorsResponse
	err := json.Unmarshal(resp.Body.Bytes(), &contribResp)
	require.NoError(t, err)

	// Empty library initially
	assert.Empty(t, contribResp.Contributors)
	assert.False(t, contribResp.HasMore)
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

	var seriesResp SyncSeriesResponse
	err := json.Unmarshal(resp.Body.Bytes(), &seriesResp)
	require.NoError(t, err)

	// Empty library initially
	assert.Empty(t, seriesResp.Series)
	assert.False(t, seriesResp.HasMore)
}

func TestGetSyncSeries_Unauthorized(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Get("/api/v1/sync/series")

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}
