package api

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestSeries creates a test series in the store and returns it.
func createTestSeries(t *testing.T, server *Server, name string) *domain.Series {
	t.Helper()

	ctx := context.Background()

	seriesID, err := id.Generate("series")
	require.NoError(t, err)

	now := time.Now()
	series := &domain.Series{
		Syncable: domain.Syncable{
			ID:        seriesID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:        name,
		Description: "Original Description",
	}

	err = server.store.CreateSeries(ctx, series)
	require.NoError(t, err)

	return series
}

// --- UpdateSeries Tests ---

func TestHandleUpdateSeries_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Test Series")

	// Create update request - change name and description.
	newName := "Updated Series Name"
	newDescription := "Updated Description"
	reqBody := SeriesUpdateRequest{
		Name:        &newName,
		Description: &newDescription,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/series/"+series.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.NotNil(t, result.Data)

	// Verify the series was updated.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, newName, data["name"])
	assert.Equal(t, newDescription, data["description"])
}

func TestHandleUpdateSeries_PartialUpdate(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Partial Update Series")

	// Only update the description - name should remain unchanged.
	newDescription := "Only Description Changed"
	reqBody := SeriesUpdateRequest{
		Description: &newDescription,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/series/"+series.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)

	data, ok := result.Data.(map[string]any)
	require.True(t, ok)

	// Name should remain unchanged.
	assert.Equal(t, series.Name, data["name"])
	// Description should be updated.
	assert.Equal(t, newDescription, data["description"])
}

func TestHandleUpdateSeries_EmptyNameRejected(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Empty Name Test")

	// Try to set name to empty string.
	emptyName := ""
	reqBody := SeriesUpdateRequest{
		Name: &emptyName,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/series/"+series.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "name cannot be empty")
}

func TestHandleUpdateSeries_Unauthorized(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	series := createTestSeries(t, server, "Unauthorized Series")

	newName := "Should Not Update"
	reqBody := SeriesUpdateRequest{
		Name: &newName,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// No Authorization header.
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/series/"+series.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
}

func TestHandleUpdateSeries_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)

	newName := "Should Not Matter"
	reqBody := SeriesUpdateRequest{
		Name: &newName,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/series/nonexistent-series-id", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "not found")
}

func TestHandleUpdateSeries_InvalidBody(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Invalid Body Series")

	// Send invalid JSON.
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/series/"+series.ID, bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result response.Envelope
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
}

func TestHandleUpdateSeries_EmptyBody(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Empty Body Series")

	// Send empty request body (valid but no changes).
	reqBody := SeriesUpdateRequest{}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/series/"+series.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	// Empty body is valid - just no fields are updated.
	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)
}

func TestHandleUpdateSeries_ClearDescription(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Clear Description Series")

	// Clear the description by setting it to empty string.
	emptyDescription := ""
	reqBody := SeriesUpdateRequest{
		Description: &emptyDescription,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/series/"+series.ID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)

	data, ok := result.Data.(map[string]any)
	require.True(t, ok)

	// Description should be empty or omitted from JSON.
	description, exists := data["description"]
	if exists {
		assert.Equal(t, "", description)
	}
	// Either way, it should NOT be the original value.
	assert.NotEqual(t, "Original Description", description)

	// Name unchanged.
	assert.Equal(t, series.Name, data["name"])
}

// --- UploadSeriesCover Tests ---

func TestHandleUploadSeriesCover_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Cover Upload Series")

	// Create a minimal valid JPEG.
	jpegData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
		0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
		0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
		0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
		0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
		0x82, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, 0xFB,
		0xD5, 0xDB, 0x20, 0xFF, 0xD9,
	}

	// Create multipart form.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "cover.jpg")
	require.NoError(t, err)
	_, err = part.Write(jpegData)
	require.NoError(t, err)
	writer.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/series/"+series.ID+"/cover", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)

	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, data["cover_url"], series.ID)
}

func TestHandleUploadSeriesCover_InvalidImageFormat(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Invalid Cover Series")

	// Create fake "image" that's not a valid format.
	fakeData := []byte("this is not an image file, just plain text")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "fake.jpg")
	require.NoError(t, err)
	_, err = part.Write(fakeData)
	require.NoError(t, err)
	writer.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/series/"+series.ID+"/cover", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "Invalid image format")
}

func TestHandleUploadSeriesCover_NoFile(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "No File Series")

	// Create multipart form without a file.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/series/"+series.ID+"/cover", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleUploadSeriesCover_Unauthorized(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	series := createTestSeries(t, server, "Unauthorized Cover Series")

	// Minimal valid PNG (1x1 transparent).
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "cover.png")
	require.NoError(t, err)
	_, err = part.Write(pngData)
	require.NoError(t, err)
	writer.Close()

	// No Authorization header.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/series/"+series.ID+"/cover", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleUploadSeriesCover_SeriesNotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)

	// Minimal valid PNG.
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "cover.png")
	require.NoError(t, err)
	_, err = part.Write(pngData)
	require.NoError(t, err)
	writer.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/series/nonexistent-series-id/cover", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- GetSeriesCover Tests ---

func TestHandleGetSeriesCover_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	series := createTestSeries(t, server, "No Cover Series")

	// Try to get cover for series that doesn't have one.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/series/"+series.ID+"/cover", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetSeriesCover_SeriesNotExist(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Try to get cover for series that doesn't exist.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/series/nonexistent-series-id/cover", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- DeleteSeriesCover Tests ---

func TestHandleDeleteSeriesCover_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	series := createTestSeries(t, server, "Delete Cover Series")

	// First upload a cover.
	jpegData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
		0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
		0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
		0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
		0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
		0x82, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, 0xFB,
		0xD5, 0xDB, 0x20, 0xFF, 0xD9,
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "cover.jpg")
	require.NoError(t, err)
	_, err = part.Write(jpegData)
	require.NoError(t, err)
	writer.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/series/"+series.ID+"/cover", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Now delete the cover.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/series/"+series.ID+"/cover", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify cover is gone.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/series/"+series.ID+"/cover", nil)
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleDeleteSeriesCover_Unauthorized(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	series := createTestSeries(t, server, "Unauthorized Delete Series")

	// No Authorization header.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/series/"+series.ID+"/cover", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleDeleteSeriesCover_SeriesNotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/series/nonexistent-series-id/cover", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
