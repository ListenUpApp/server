package service

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAuthTest creates services with temporary storage for testing.
func setupAuthTest(t *testing.T) (*AuthService, *InstanceService, *auth.TokenService, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "listenup-auth-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	// Create test config
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name:      "Test Server",
			LocalURL:  "http://localhost:8080",
			RemoteURL: "",
		},
		Auth: config.AuthConfig{
			AccessTokenDuration:  15 * time.Minute,
			RefreshTokenDuration: 30 * 24 * time.Hour,
		},
	}

	// Load or generate auth key
	authKey, err := auth.LoadOrGenerateKey(tmpDir)
	require.NoError(t, err)
	cfg.Auth.AccessTokenKey = authKey

	// Create token service (convert key to hex string)
	tokenService, err := auth.NewTokenService(
		hex.EncodeToString(authKey),
		cfg.Auth.AccessTokenDuration,
		cfg.Auth.RefreshTokenDuration,
	)
	require.NoError(t, err)

	// Create session service
	sessionService := NewSessionService(s, tokenService, nil)

	// Create instance service
	instanceService := NewInstanceService(s, nil, cfg)

	// Create auth service
	authService := NewAuthService(s, tokenService, sessionService, instanceService, nil)

	// Cleanup function
	cleanup := func() {
		_ = s.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return authService, instanceService, tokenService, cleanup
}

func TestAuthService_Setup_Success(t *testing.T) {
	authService, instanceService, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize instance
	_, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	// Setup should work
	req := SetupRequest{
		Email:       "admin@example.com",
		Password:    "SecurePassword123!",
		DisplayName: "Admin User",
	}

	resp, err := authService.Setup(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Verify response
	assert.NotNil(t, resp.User)
	assert.Equal(t, "admin@example.com", resp.User.Email)
	assert.Equal(t, "Admin User", resp.User.DisplayName)
	assert.True(t, resp.User.IsRoot)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.Equal(t, "Bearer", resp.TokenType)
	assert.NotEmpty(t, resp.SessionID)
	assert.Greater(t, resp.ExpiresIn, 0)

	// Verify instance is configured
	setupRequired, err := instanceService.IsSetupRequired(ctx)
	require.NoError(t, err)
	assert.False(t, setupRequired)
}

func TestAuthService_Setup_AlreadyConfigured(t *testing.T) {
	authService, instanceService, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize and configure instance
	_, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	// First setup succeeds
	req := SetupRequest{
		Email:       "admin@example.com",
		Password:    "SecurePassword123!",
		DisplayName: "Admin User",
	}

	_, err = authService.Setup(ctx, req)
	require.NoError(t, err)

	// Second setup should fail
	req2 := SetupRequest{
		Email:       "admin2@example.com",
		Password:    "SecurePassword123!",
		DisplayName: "Admin User 2",
	}

	_, err = authService.Setup(ctx, req2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already configured")
}

func TestAuthService_Setup_WeakPassword(t *testing.T) {
	authService, instanceService, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize instance
	_, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	tests := []struct {
		name     string
		password string
		wantErr  string
	}{
		{
			name:     "empty password",
			password: "",
			wantErr:  "required",
		},
		{
			name:     "too short",
			password: "short",
			wantErr:  "at least 8 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := SetupRequest{
				Email:       "admin@example.com",
				Password:    tt.password,
				DisplayName: "Admin User",
			}

			_, err := authService.Setup(ctx, req)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test user directly
	password := "SecurePassword123!"
	passwordHash, err := auth.HashPassword(password)
	require.NoError(t, err)

	testStore := authService.store
	user := createTestUser(t, testStore, "test@example.com", passwordHash)

	// Login should work
	req := LoginRequest{
		Email:    "test@example.com",
		Password: password,
		DeviceInfo: auth.DeviceInfo{
			DeviceType: "mobile",
			Platform:   "iOS",
		},
		IPAddress: "192.168.1.1",
	}

	resp, err := authService.Login(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Verify response
	assert.Equal(t, user.ID, resp.User.ID)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
	assert.NotEmpty(t, resp.SessionID)
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test user
	passwordHash, err := auth.HashPassword("CorrectPassword123!")
	require.NoError(t, err)

	createTestUser(t, authService.store, "test@example.com", passwordHash)

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{
			name:     "wrong email",
			email:    "wrong@example.com",
			password: "CorrectPassword123!",
		},
		{
			name:     "wrong password",
			email:    "test@example.com",
			password: "WrongPassword123!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := LoginRequest{
				Email:    tt.email,
				Password: tt.password,
				DeviceInfo: auth.DeviceInfo{
					DeviceType: "web",
					Platform:   "Web",
				},
			}

			_, err := authService.Login(ctx, req)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid email or password")
		})
	}
}

func TestAuthService_Login_MissingDeviceInfo(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test user
	passwordHash, err := auth.HashPassword("SecurePassword123!")
	require.NoError(t, err)

	createTestUser(t, authService.store, "test@example.com", passwordHash)

	req := LoginRequest{
		Email:      "test@example.com",
		Password:   "SecurePassword123!",
		DeviceInfo: auth.DeviceInfo{}, // Empty device info
	}

	_, err = authService.Login(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "device_info is required")
}

func TestAuthService_RefreshTokens_Success(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create user and login to get initial tokens
	passwordHash, err := auth.HashPassword("SecurePassword123!")
	require.NoError(t, err)

	createTestUser(t, authService.store, "test@example.com", passwordHash)

	loginResp, err := authService.Login(ctx, LoginRequest{
		Email:    "test@example.com",
		Password: "SecurePassword123!",
		DeviceInfo: auth.DeviceInfo{
			DeviceType: "mobile",
			Platform:   "iOS",
		},
	})
	require.NoError(t, err)

	// Wait a moment to ensure new tokens will have different timestamps
	time.Sleep(10 * time.Millisecond)

	// Refresh tokens
	refreshReq := RefreshRequest{
		RefreshToken: loginResp.RefreshToken,
		DeviceInfo: auth.DeviceInfo{
			DeviceType: "mobile",
			Platform:   "iOS",
		},
	}

	refreshResp, err := authService.RefreshTokens(ctx, refreshReq)
	require.NoError(t, err)
	assert.NotNil(t, refreshResp)

	// Verify new tokens are different
	assert.NotEqual(t, loginResp.AccessToken, refreshResp.AccessToken)
	assert.NotEqual(t, loginResp.RefreshToken, refreshResp.RefreshToken)
	assert.Equal(t, loginResp.SessionID, refreshResp.SessionID) // Same session

	// Old refresh token should be invalidated
	_, err = authService.RefreshTokens(ctx, RefreshRequest{
		RefreshToken: loginResp.RefreshToken,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired")
}

func TestAuthService_RefreshTokens_InvalidToken(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	req := RefreshRequest{
		RefreshToken: "invalid-token-12345",
	}

	_, err := authService.RefreshTokens(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired")
}

func TestAuthService_Logout_Success(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create user and login
	passwordHash, err := auth.HashPassword("SecurePassword123!")
	require.NoError(t, err)

	createTestUser(t, authService.store, "test@example.com", passwordHash)

	loginResp, err := authService.Login(ctx, LoginRequest{
		Email:    "test@example.com",
		Password: "SecurePassword123!",
		DeviceInfo: auth.DeviceInfo{
			DeviceType: "web",
			Platform:   "Web",
		},
	})
	require.NoError(t, err)

	// Logout
	err = authService.Logout(ctx, loginResp.SessionID)
	assert.NoError(t, err)

	// Refresh token should no longer work
	_, err = authService.RefreshTokens(ctx, RefreshRequest{
		RefreshToken: loginResp.RefreshToken,
	})
	assert.Error(t, err)
}

func TestAuthService_Logout_NonExistentSession(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Logout of non-existent session should not error
	err := authService.Logout(ctx, "session_nonexistent")
	assert.NoError(t, err)
}

func TestAuthService_VerifyAccessToken_Success(t *testing.T) {
	authService, _, tokenService, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create user
	passwordHash, err := auth.HashPassword("SecurePassword123!")
	require.NoError(t, err)

	user := createTestUser(t, authService.store, "test@example.com", passwordHash)

	// Generate token
	token, err := tokenService.GenerateAccessToken(user)
	require.NoError(t, err)

	// Verify token
	verifiedUser, claims, err := authService.VerifyAccessToken(ctx, token)
	require.NoError(t, err)
	assert.NotNil(t, verifiedUser)
	assert.NotNil(t, claims)

	assert.Equal(t, user.ID, verifiedUser.ID)
	assert.Equal(t, user.Email, verifiedUser.Email)
	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, user.Email, claims.Email)
}

func TestAuthService_VerifyAccessToken_InvalidToken(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	_, _, err := authService.VerifyAccessToken(ctx, "invalid-token")
	assert.Error(t, err)
}

func TestAuthService_VerifyAccessToken_DeletedUser(t *testing.T) {
	authService, _, tokenService, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create user and generate token
	passwordHash, err := auth.HashPassword("SecurePassword123!")
	require.NoError(t, err)

	user := createTestUser(t, authService.store, "test@example.com", passwordHash)

	token, err := tokenService.GenerateAccessToken(user)
	require.NoError(t, err)

	// Soft delete user
	user.MarkDeleted()
	err = authService.store.UpdateUser(ctx, user)
	require.NoError(t, err)

	// Token should fail verification
	_, _, err = authService.VerifyAccessToken(ctx, token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user not found")
}

func TestAuthService_Login_ValidationErrors(t *testing.T) {
	authService, _, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name    string
		req     LoginRequest
		wantErr string
	}{
		{
			name: "invalid email format",
			req: LoginRequest{
				Email:    "not-an-email",
				Password: "ValidPassword123!",
				DeviceInfo: auth.DeviceInfo{
					DeviceType: "mobile",
					Platform:   "iOS",
				},
			},
			wantErr: "email",
		},
		{
			name: "missing email",
			req: LoginRequest{
				Email:    "",
				Password: "ValidPassword123!",
				DeviceInfo: auth.DeviceInfo{
					DeviceType: "mobile",
					Platform:   "iOS",
				},
			},
			wantErr: "email",
		},
		{
			name: "missing password",
			req: LoginRequest{
				Email:    "user@example.com",
				Password: "",
				DeviceInfo: auth.DeviceInfo{
					DeviceType: "mobile",
					Platform:   "iOS",
				},
			},
			wantErr: "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := authService.Login(ctx, tt.req)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAuthService_Setup_ValidationErrors(t *testing.T) {
	authService, instanceService, _, cleanup := setupAuthTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initialize instance
	_, err := instanceService.InitializeInstance(ctx)
	require.NoError(t, err)

	tests := []struct {
		name    string
		req     SetupRequest
		wantErr string
	}{
		{
			name: "invalid email format",
			req: SetupRequest{
				Email:       "not-an-email",
				Password:    "ValidPassword123!",
				DisplayName: "Admin User",
			},
			wantErr: "email",
		},
		{
			name: "missing email",
			req: SetupRequest{
				Email:       "",
				Password:    "ValidPassword123!",
				DisplayName: "Admin User",
			},
			wantErr: "email",
		},
		{
			name: "missing display name",
			req: SetupRequest{
				Email:       "admin@example.com",
				Password:    "ValidPassword123!",
				DisplayName: "",
			},
			wantErr: "display_name",
		},
		{
			name: "password too long",
			req: SetupRequest{
				Email:       "admin@example.com",
				Password:    string(make([]byte, 1025)),
				DisplayName: "Admin User",
			},
			wantErr: "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := authService.Setup(ctx, tt.req)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// Helper function to create a test user
func createTestUser(t *testing.T, s *store.Store, email, passwordHash string) *domain.User {
	t.Helper()

	userID, err := id.Generate("user")
	require.NoError(t, err)

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: userID,
		},
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  "Test User",
		IsRoot:       false,
	}
	user.InitTimestamps()

	err = s.CreateUser(context.Background(), user)
	require.NoError(t, err)

	return user
}
