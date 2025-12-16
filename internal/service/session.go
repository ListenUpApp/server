package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
)

// SessionService handles user session management and lifecycle.
// Sessions track authenticated devices and their refresh tokens.
type SessionService struct {
	store        *store.Store
	tokenService *auth.TokenService
	logger       *slog.Logger
}

// NewSessionService creates a new session management service.
func NewSessionService(
	store *store.Store,
	tokenService *auth.TokenService,
	logger *slog.Logger,
) *SessionService {
	return &SessionService{
		store:        store,
		tokenService: tokenService,
		logger:       logger,
	}
}

// CreateSession generates tokens and creates a new session for a user.
// Returns access token, refresh token, and session metadata.
func (s *SessionService) CreateSession(
	ctx context.Context,
	user *domain.User,
	deviceInfo auth.DeviceInfo,
	ipAddress string,
) (*SessionResponse, error) {
	// Generate tokens
	accessToken, err := s.tokenService.GenerateAccessToken(user)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.tokenService.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Create session record
	sessionID, err := id.Generate("session")
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}

	now := time.Now()
	session := &domain.Session{
		ID:               sessionID,
		UserID:           user.ID,
		RefreshTokenHash: auth.HashRefreshToken(refreshToken),
		ExpiresAt:        now.Add(s.tokenService.RefreshTokenDuration()),
		CreatedAt:        now,
		LastSeenAt:       now,
		IPAddress:        ipAddress,
	}
	updateSessionDeviceInfo(session, deviceInfo)

	if err := s.store.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}

	// Calculate expiry in seconds
	expiresIn := int(s.tokenService.AccessTokenDuration().Seconds())

	return &SessionResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		SessionID:    sessionID,
	}, nil
}

// RefreshSession rotates tokens for an existing session.
// The old refresh token is invalidated (token rotation for security).
func (s *SessionService) RefreshSession(
	ctx context.Context,
	refreshToken string,
	deviceInfo auth.DeviceInfo,
	ipAddress string,
) (*SessionResponse, *domain.User, error) {
	// Look up session by refresh token
	tokenHash := auth.HashRefreshToken(refreshToken)
	session, err := s.store.GetSessionByRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, nil, domainerrors.TokenExpired("invalid or expired refresh token").WithCause(err)
	}

	// Get user
	user, err := s.store.GetUser(ctx, session.UserID)
	if err != nil {
		// User was deleted, clean up session
		_ = s.store.DeleteSession(ctx, session.ID)
		return nil, nil, domainerrors.NotFound("user not found").WithCause(err)
	}

	// Generate new tokens
	accessToken, err := s.tokenService.GenerateAccessToken(user)
	if err != nil {
		return nil, nil, fmt.Errorf("generate access token: %w", err)
	}

	newRefreshToken, err := s.tokenService.GenerateRefreshToken()
	if err != nil {
		return nil, nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Update session with new refresh token (rotation)
	session.RefreshTokenHash = auth.HashRefreshToken(newRefreshToken)
	session.Touch()

	// Update device info if provided and valid
	if deviceInfo.IsValid() {
		updateSessionDeviceInfo(session, deviceInfo)
	}

	// Update IP if provided
	if ipAddress != "" {
		session.IPAddress = ipAddress
	}

	if err := s.store.UpdateSession(ctx, session); err != nil {
		return nil, nil, fmt.Errorf("update session: %w", err)
	}

	// Calculate expiry
	expiresIn := int(s.tokenService.AccessTokenDuration().Seconds())

	return &SessionResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		SessionID:    session.ID,
	}, user, nil
}

// DeleteSession ends a session (logout).
func (s *SessionService) DeleteSession(ctx context.Context, sessionID string) error {
	if err := s.store.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Session deleted", "session_id", sessionID)
	}

	return nil
}

// ListUserSessions returns all active sessions for a user.
func (s *SessionService) ListUserSessions(ctx context.Context, userID string) ([]*domain.Session, error) {
	sessions, err := s.store.ListUserSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user sessions: %w", err)
	}
	return sessions, nil
}

// DeleteExpiredSessions removes all expired sessions.
// This should be run periodically as a cleanup job.
func (s *SessionService) DeleteExpiredSessions(ctx context.Context) (int, error) {
	count, err := s.store.DeleteExpiredSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}

	if s.logger != nil && count > 0 {
		s.logger.Info("Deleted expired sessions", "count", count)
	}

	return count, nil
}

// SessionResponse contains session tokens and metadata.
type SessionResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // Seconds until access token expires
	SessionID    string `json:"session_id"`
}

// updateSessionDeviceInfo copies device info fields to session.
// Extracted to avoid duplication between create and refresh flows.
func updateSessionDeviceInfo(session *domain.Session, info auth.DeviceInfo) {
	session.DeviceType = info.DeviceType
	session.Platform = info.Platform
	session.PlatformVersion = info.PlatformVersion
	session.ClientName = info.ClientName
	session.ClientVersion = info.ClientVersion
	session.ClientBuild = info.ClientBuild
	session.DeviceName = info.DeviceName
	session.DeviceModel = info.DeviceModel
	session.BrowserName = info.BrowserName
	session.BrowserVersion = info.BrowserVersion
}
