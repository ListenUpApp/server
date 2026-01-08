package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetup_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Admin",
		"last_name":  "User",
	})

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[AuthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.NotEmpty(t, envelope.Data.AccessToken)
	assert.NotEmpty(t, envelope.Data.RefreshToken)
	assert.Equal(t, "admin@example.com", envelope.Data.User.Email)
	assert.Equal(t, "Admin", envelope.Data.User.FirstName)
	assert.Equal(t, "User", envelope.Data.User.LastName)
	assert.Positive(t, envelope.Data.ExpiresIn)
}

func TestSetup_AlreadyConfigured(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// First setup succeeds
	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Admin",
		"last_name":  "User",
	})
	require.Equal(t, http.StatusOK, resp.Code)

	// Second setup fails
	resp = ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin2@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Admin2",
		"last_name":  "User",
	})

	assert.Equal(t, http.StatusConflict, resp.Code)
}

func TestSetup_ValidationErrors(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
	}{
		{
			name: "missing email",
			body: map[string]any{
				"password":   "SecurePassword123!",
				"first_name": "Admin",
				"last_name":  "User",
			},
			wantStatus: http.StatusUnprocessableEntity, // Huma returns 422 for missing required fields
		},
		{
			name: "invalid email format",
			body: map[string]any{
				"email":      "not-an-email",
				"password":   "SecurePassword123!",
				"first_name": "Admin",
				"last_name":  "User",
			},
			wantStatus: http.StatusBadRequest, // Validation errors return 400
		},
		{
			name: "password too short",
			body: map[string]any{
				"email":      "admin@example.com",
				"password":   "short",
				"first_name": "Admin",
				"last_name":  "User",
			},
			wantStatus: http.StatusBadRequest, // Validation errors return 400
		},
		{
			name: "missing first name",
			body: map[string]any{
				"email":      "admin@example.com",
				"password":   "SecurePassword123!",
				"first_name": "",
				"last_name":  "User",
			},
			wantStatus: http.StatusBadRequest, // Validation errors return 400
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := ts.api.Post("/api/v1/auth/setup", tt.body)
			assert.Equal(t, tt.wantStatus, resp.Code)
		})
	}
}

func TestLogin_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// First setup the server
	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Admin",
		"last_name":  "User",
	})
	require.Equal(t, http.StatusOK, resp.Code)

	// Now login
	resp = ts.api.Post("/api/v1/auth/login", map[string]any{
		"email":    "admin@example.com",
		"password": "SecurePassword123!",
		"device_info": map[string]any{
			"device_type": "mobile",
			"platform":    "iOS",
		},
	})

	assert.Equal(t, http.StatusOK, resp.Code)

	var envelope testEnvelope[AuthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &envelope)
	require.NoError(t, err)

	assert.True(t, envelope.Success)
	assert.NotEmpty(t, envelope.Data.AccessToken)
	assert.NotEmpty(t, envelope.Data.RefreshToken)
	assert.Equal(t, "admin@example.com", envelope.Data.User.Email)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Setup first
	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Admin",
		"last_name":  "User",
	})
	require.Equal(t, http.StatusOK, resp.Code)

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{
			name:     "wrong email",
			email:    "wrong@example.com",
			password: "SecurePassword123!",
		},
		{
			name:     "wrong password",
			email:    "admin@example.com",
			password: "WrongPassword123!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := ts.api.Post("/api/v1/auth/login", map[string]any{
				"email":    tt.email,
				"password": tt.password,
				"device_info": map[string]any{
					"device_type": "web",
					"platform":    "Web",
				},
			})

			assert.Equal(t, http.StatusUnauthorized, resp.Code)
		})
	}
}

func TestLogin_MissingDeviceInfo(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Setup first
	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Admin",
		"last_name":  "User",
	})
	require.Equal(t, http.StatusOK, resp.Code)

	// Login without device_info
	resp = ts.api.Post("/api/v1/auth/login", map[string]any{
		"email":    "admin@example.com",
		"password": "SecurePassword123!",
	})

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestRefresh_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Setup and login
	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Admin",
		"last_name":  "User",
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var setupEnvelope testEnvelope[AuthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &setupEnvelope)
	require.NoError(t, err)

	// Refresh tokens
	resp = ts.api.Post("/api/v1/auth/refresh", map[string]any{
		"refresh_token": setupEnvelope.Data.RefreshToken,
		"device_info": map[string]any{
			"device_type": "mobile",
			"platform":    "iOS",
		},
	})

	assert.Equal(t, http.StatusOK, resp.Code)

	var refreshEnvelope testEnvelope[AuthResponse]
	err = json.Unmarshal(resp.Body.Bytes(), &refreshEnvelope)
	require.NoError(t, err)

	assert.True(t, refreshEnvelope.Success)
	assert.NotEmpty(t, refreshEnvelope.Data.AccessToken)
	assert.NotEmpty(t, refreshEnvelope.Data.RefreshToken)
	// New tokens should be different
	assert.NotEqual(t, setupEnvelope.Data.AccessToken, refreshEnvelope.Data.AccessToken)
	assert.NotEqual(t, setupEnvelope.Data.RefreshToken, refreshEnvelope.Data.RefreshToken)
}

func TestRefresh_InvalidToken(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	resp := ts.api.Post("/api/v1/auth/refresh", map[string]any{
		"refresh_token": "invalid-token-12345",
	})

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestLogout_Success(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.cleanup()

	// Setup first
	resp := ts.api.Post("/api/v1/auth/setup", map[string]any{
		"email":      "admin@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Admin",
		"last_name":  "User",
	})
	require.Equal(t, http.StatusOK, resp.Code)

	// Login to get a session
	resp = ts.api.Post("/api/v1/auth/login", map[string]any{
		"email":    "admin@example.com",
		"password": "SecurePassword123!",
		"device_info": map[string]any{
			"device_type": "mobile",
			"platform":    "iOS",
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var loginEnvelope testEnvelope[AuthResponse]
	err := json.Unmarshal(resp.Body.Bytes(), &loginEnvelope)
	require.NoError(t, err)
	require.True(t, loginEnvelope.Success)

	// Get user's sessions to find session ID
	// For now we'll use a placeholder - logout should work even for non-existent sessions
	resp = ts.api.Post("/api/v1/auth/logout", map[string]any{
		"session_id": "session_test123",
	})

	assert.Equal(t, http.StatusOK, resp.Code)
}
