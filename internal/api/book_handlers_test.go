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

// createTestBook creates a test book in the store and returns it.
func createTestBook(t *testing.T, server *Server, title string) *domain.Book {
	t.Helper()

	ctx := context.Background()

	bookID, err := id.Generate("book")
	require.NoError(t, err)

	now := time.Now()
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Title:       title,
		Subtitle:    "Original Subtitle",
		Description: "Original Description",
		Publisher:   "Original Publisher",
		PublishYear: "2020",
		Language:    "en",
		ISBN:        "978-0-123456-78-9",
		ASIN:        "B01234567X",
		Path:        "/audiobooks/" + title,
		AudioFiles: []domain.AudioFileInfo{
			{
				ID:       "af-1",
				Path:     "/audiobooks/" + title + "/01.mp3",
				Filename: "01.mp3",
				Format:   "mp3",
				Size:     1000000,
				Duration: 3600000,
				Inode:    12345,
			},
		},
		TotalDuration: 3600000,
		TotalSize:     1000000,
	}

	err = server.store.CreateBook(ctx, book)
	require.NoError(t, err)

	return book
}

func TestHandleUpdateBook_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create test user and book.
	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Test Book")

	// Create update request - change title and description.
	newTitle := "Updated Title"
	newDescription := "Updated Description"
	reqBody := BookUpdateRequest{
		Title:       &newTitle,
		Description: &newDescription,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/books/"+book.ID, bytes.NewReader(body))
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

	// Verify the book was updated.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, newTitle, data["title"])
	assert.Equal(t, newDescription, data["description"])

	// Verify unchanged fields remained the same.
	assert.Equal(t, book.Subtitle, data["subtitle"])
	assert.Equal(t, book.Publisher, data["publisher"])
}

func TestHandleUpdateBook_PartialUpdate(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Partial Update Book")

	// Only update the title - all other fields should remain unchanged.
	newTitle := "Only Title Changed"
	reqBody := BookUpdateRequest{
		Title: &newTitle,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/books/"+book.ID, bytes.NewReader(body))
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

	// Title should be updated.
	assert.Equal(t, newTitle, data["title"])

	// All other fields should remain unchanged.
	assert.Equal(t, book.Subtitle, data["subtitle"])
	assert.Equal(t, book.Description, data["description"])
	assert.Equal(t, book.Publisher, data["publisher"])
	assert.Equal(t, book.PublishYear, data["publish_year"])
	assert.Equal(t, book.Language, data["language"])
	assert.Equal(t, book.ISBN, data["isbn"])
	assert.Equal(t, book.ASIN, data["asin"])
}

func TestHandleUpdateBook_Unauthorized(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	book := createTestBook(t, server, "Unauthorized Book")

	newTitle := "Should Not Update"
	reqBody := BookUpdateRequest{
		Title: &newTitle,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// No Authorization header.
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/books/"+book.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
}

func TestHandleUpdateBook_NotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)

	newTitle := "Should Not Matter"
	reqBody := BookUpdateRequest{
		Title: &newTitle,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/books/nonexistent-book-id", bytes.NewReader(body))
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

func TestHandleUpdateBook_InvalidBody(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Invalid Body Book")

	// Send invalid JSON.
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/books/"+book.ID, bytes.NewReader([]byte("invalid json")))
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

func TestHandleUpdateBook_EmptyBody(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Empty Body Book")

	// Send empty request body (valid but no changes).
	reqBody := BookUpdateRequest{}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/books/"+book.ID, bytes.NewReader(body))
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

func TestHandleUpdateBook_ClearField(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Clear Field Book")

	// Clear the subtitle by setting it to empty string.
	emptySubtitle := ""
	reqBody := BookUpdateRequest{
		Subtitle: &emptySubtitle,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/books/"+book.ID, bytes.NewReader(body))
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

	// Subtitle should be empty or omitted from JSON (both are valid for cleared fields).
	subtitle, exists := data["subtitle"]
	if exists {
		assert.Equal(t, "", subtitle)
	}
	// Either way, it should NOT be the original value.
	assert.NotEqual(t, "Original Subtitle", subtitle)

	// Other fields unchanged.
	assert.Equal(t, book.Title, data["title"])
}

func TestHandleUpdateBook_BooleanFields(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Boolean Fields Book")

	// Set abridged to true.
	abridged := true
	reqBody := BookUpdateRequest{
		Abridged: &abridged,
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/books/"+book.ID, bytes.NewReader(body))
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

	assert.Equal(t, true, data["abridged"])
}

// --- SetBookContributors Tests ---

func TestHandleSetBookContributors_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Contributors Test Book")

	reqBody := SetContributorsRequest{
		Contributors: []ContributorInput{
			{Name: "Brandon Sanderson", Roles: []string{"author"}},
			{Name: "Michael Kramer", Roles: []string{"narrator"}},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/contributors", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)

	// Verify book has contributors in response.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)

	contributors, ok := data["contributors"].([]any)
	require.True(t, ok)
	assert.Len(t, contributors, 2)
}

func TestHandleSetBookContributors_InvalidRole(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Invalid Role Book")

	reqBody := SetContributorsRequest{
		Contributors: []ContributorInput{
			{Name: "Test Author", Roles: []string{"invalid-role"}},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/contributors", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "Invalid role")
}

func TestHandleSetBookContributors_EmptyName(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Empty Name Book")

	reqBody := SetContributorsRequest{
		Contributors: []ContributorInput{
			{Name: "", Roles: []string{"author"}},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/contributors", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSetBookContributors_WhitespaceOnlyName(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Whitespace Name Book")

	reqBody := SetContributorsRequest{
		Contributors: []ContributorInput{
			{Name: "   ", Roles: []string{"author"}}, // Whitespace only
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/contributors", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "name is required")
}

func TestHandleSetBookContributors_NoRoles(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "No Roles Book")

	reqBody := SetContributorsRequest{
		Contributors: []ContributorInput{
			{Name: "Test Author", Roles: []string{}}, // Empty roles
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/contributors", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "At least one role is required")
}

func TestHandleSetBookContributors_Unauthorized(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	book := createTestBook(t, server, "Unauthorized Contributors Book")

	reqBody := SetContributorsRequest{
		Contributors: []ContributorInput{
			{Name: "Test Author", Roles: []string{"author"}},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// No Authorization header.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/contributors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleSetBookContributors_ClearAll(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Clear Contributors Book")

	// First add some contributors.
	reqBody := SetContributorsRequest{
		Contributors: []ContributorInput{
			{Name: "Test Author", Roles: []string{"author"}},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/contributors", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Now clear all contributors.
	reqBody = SetContributorsRequest{
		Contributors: []ContributorInput{}, // Empty list
	}
	body, err = json.Marshal(reqBody)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/contributors", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result response.Envelope
	err = json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result.Success)

	// Verify contributors is now empty.
	data, ok := result.Data.(map[string]any)
	require.True(t, ok)

	contributors, ok := data["contributors"].([]any)
	if ok {
		assert.Len(t, contributors, 0)
	}
	// If contributors key is missing, that's also valid (no contributors).
}

// --- UploadCover Tests ---

func TestHandleUploadCover_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Cover Upload Book")

	// Create a minimal valid JPEG (smallest valid JPEG is 125 bytes).
	// This is a 1x1 red pixel JPEG.
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

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/cover", &buf)
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
	assert.Contains(t, data["cover_url"], book.ID)
}

func TestHandleUploadCover_InvalidImageFormat(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "Invalid Cover Book")

	// Create fake "image" that's not a valid format.
	fakeData := []byte("this is not an image file, just plain text")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "fake.jpg")
	require.NoError(t, err)
	_, err = part.Write(fakeData)
	require.NoError(t, err)
	writer.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/cover", &buf)
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

func TestHandleUploadCover_NoFile(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	token := createTestUserWithToken(t, server)
	book := createTestBook(t, server, "No File Book")

	// Create multipart form without a file.
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/cover", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleUploadCover_Unauthorized(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	book := createTestBook(t, server, "Unauthorized Cover Book")

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
	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/"+book.ID+"/cover", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleUploadCover_BookNotFound(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/nonexistent-book-id/cover", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- detectImageType Unit Tests ---

func TestDetectImageType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "JPEG",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46},
			expected: "image/jpeg",
		},
		{
			name:     "PNG",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expected: "image/png",
		},
		{
			name:     "GIF",
			data:     []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00},
			expected: "image/gif",
		},
		{
			name:     "WebP",
			data:     []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50},
			expected: "image/webp",
		},
		{
			name:     "Unknown",
			data:     []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
			expected: "",
		},
		{
			name:     "Too short",
			data:     []byte{0xFF, 0xD8},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectImageType(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}
