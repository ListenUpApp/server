package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
)

const (
	// inviteCodeSize is the number of bytes for invite codes (16 bytes = 128 bits of entropy).
	inviteCodeSize = 16
	// defaultInviteExpiry is the default time until an invite expires.
	defaultInviteExpiry = 7 * 24 * time.Hour // 7 days
)

// InviteService handles invite creation, validation, and claiming.
type InviteService struct {
	store          *store.Store
	sessionService *SessionService
	logger         *slog.Logger
	serverURL      string // Base URL for generating invite links
}

// NewInviteService creates a new invite service.
func NewInviteService(
	store *store.Store,
	sessionService *SessionService,
	logger *slog.Logger,
	serverURL string,
) *InviteService {
	return &InviteService{
		store:          store,
		sessionService: sessionService,
		logger:         logger,
		serverURL:      serverURL,
	}
}

// CreateInviteRequest contains the data needed to create an invite.
type CreateInviteRequest struct {
	Name         string      `json:"name" validate:"required,max=100"`
	Email        string      `json:"email" validate:"required,email"`
	Role         domain.Role `json:"role" validate:"required,oneof=admin member"`
	ExpiresInDays int        `json:"expires_in_days"` // 0 = use default (7 days)
}

// InviteResponse is returned after creating an invite.
type InviteResponse struct {
	*domain.Invite
	URL string `json:"url"` // Full URL for sharing
}

// InviteDetailsResponse is returned for public invite lookups.
type InviteDetailsResponse struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	ServerName string `json:"server_name"`
	InvitedBy  string `json:"invited_by"`
	Valid      bool   `json:"valid"`
}

// ClaimInviteRequest contains the data needed to claim an invite.
type ClaimInviteRequest struct {
	Code       string          `json:"code" validate:"required"`
	Password   string          `json:"password" validate:"required,min=8,max=1024"`
	DeviceInfo auth.DeviceInfo `json:"device_info"`
	IPAddress  string          `json:"-"` // Extracted from request by handler
}

// CreateInvite creates a new invite.
func (s *InviteService) CreateInvite(ctx context.Context, adminUserID string, req CreateInviteRequest) (*InviteResponse, error) {
	// Validate request
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	// Check if email is already in use
	existingUser, err := s.store.GetUserByEmail(ctx, req.Email)
	if err == nil && existingUser != nil {
		return nil, domainerrors.AlreadyExists("a user with this email already exists")
	}
	if err != nil && !errors.Is(err, store.ErrUserNotFound) {
		return nil, fmt.Errorf("check email: %w", err)
	}

	// Generate invite code
	code, err := generateInviteCode()
	if err != nil {
		return nil, fmt.Errorf("generate invite code: %w", err)
	}

	// Generate invite ID
	inviteID, err := id.Generate("invite")
	if err != nil {
		return nil, fmt.Errorf("generate invite ID: %w", err)
	}

	// Calculate expiration
	expiresIn := defaultInviteExpiry
	if req.ExpiresInDays > 0 {
		expiresIn = time.Duration(req.ExpiresInDays) * 24 * time.Hour
	}

	invite := &domain.Invite{
		Syncable: domain.Syncable{
			ID: inviteID,
		},
		Code:      code,
		Name:      req.Name,
		Email:     req.Email,
		Role:      req.Role,
		CreatedBy: adminUserID,
		ExpiresAt: time.Now().Add(expiresIn),
	}
	invite.InitTimestamps()

	if err := s.store.CreateInvite(ctx, invite); err != nil {
		if errors.Is(err, store.ErrInviteCodeExists) {
			// Extremely unlikely with 128-bit entropy, but handle it
			return nil, domainerrors.Conflict("invite code collision, please try again")
		}
		return nil, fmt.Errorf("create invite: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Invite created",
			"invite_id", invite.ID,
			"name", invite.Name,
			"email", invite.Email,
			"role", invite.Role,
			"created_by", adminUserID,
		)
	}

	return &InviteResponse{
		Invite: invite,
		URL:    s.serverURL + "/join/" + code,
	}, nil
}

// GetInviteDetails returns invite details by code (public, for landing page).
func (s *InviteService) GetInviteDetails(ctx context.Context, code string) (*InviteDetailsResponse, error) {
	invite, err := s.store.GetInviteByCode(ctx, code)
	if err != nil {
		if errors.Is(err, store.ErrInviteNotFound) {
			return nil, domainerrors.NotFound("invite not found")
		}
		return nil, fmt.Errorf("get invite: %w", err)
	}

	// Get inviter's name
	inviterName := "Unknown"
	inviter, err := s.store.GetUser(ctx, invite.CreatedBy)
	if err == nil {
		inviterName = inviter.Name()
	}

	// Get server name (from instance settings)
	serverName := "ListenUp Server" // TODO: Get from instance settings

	return &InviteDetailsResponse{
		Name:       invite.Name,
		Email:      invite.Email,
		ServerName: serverName,
		InvitedBy:  inviterName,
		Valid:      invite.IsValid(),
	}, nil
}

// ClaimInvite claims an invite and creates a new user.
func (s *InviteService) ClaimInvite(ctx context.Context, req ClaimInviteRequest) (*AuthResponse, error) {
	// Validate request
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	// Get invite
	invite, err := s.store.GetInviteByCode(ctx, req.Code)
	if err != nil {
		if errors.Is(err, store.ErrInviteNotFound) {
			return nil, domainerrors.NotFound("invite not found")
		}
		return nil, fmt.Errorf("get invite: %w", err)
	}

	// Check invite is valid
	if invite.IsClaimed() {
		return nil, domainerrors.Conflict("invite has already been claimed")
	}
	if invite.IsExpired() {
		return nil, domainerrors.Conflict("invite has expired")
	}
	if invite.IsDeleted() {
		return nil, domainerrors.NotFound("invite not found")
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Generate user ID
	userID, err := id.Generate("user")
	if err != nil {
		return nil, fmt.Errorf("generate user ID: %w", err)
	}

	// Create user
	now := time.Now()
	user := &domain.User{
		Syncable: domain.Syncable{
			ID: userID,
		},
		Email:        invite.Email,
		PasswordHash: passwordHash,
		IsRoot:       false,
		Role:         invite.Role,
		InvitedBy:    invite.CreatedBy,
		DisplayName:  invite.Name,
		FirstName:    "", // Not collected during invite claim
		LastName:     "", // Not collected during invite claim
		LastLoginAt:  now,
	}
	user.InitTimestamps()

	// Save user
	if err := s.store.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrEmailExists) {
			return nil, domainerrors.AlreadyExists("email already in use")
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Mark invite as claimed
	invite.ClaimedAt = &now
	invite.ClaimedBy = userID
	if err := s.store.UpdateInvite(ctx, invite); err != nil {
		// Log but don't fail - user is created
		if s.logger != nil {
			s.logger.Warn("Failed to mark invite as claimed",
				"invite_id", invite.ID,
				"user_id", userID,
				"error", err,
			)
		}
	}

	// Create session
	sessionResp, err := s.sessionService.CreateSession(ctx, user, req.DeviceInfo, req.IPAddress)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Invite claimed",
			"invite_id", invite.ID,
			"user_id", userID,
			"email", user.Email,
			"role", user.Role,
		)
	}

	return &AuthResponse{
		User:            user,
		SessionResponse: *sessionResp,
	}, nil
}

// ListInvites returns all invites.
func (s *InviteService) ListInvites(ctx context.Context) ([]*domain.Invite, error) {
	invites, err := s.store.ListInvites(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	return invites, nil
}

// DeleteInvite revokes an unclaimed invite.
func (s *InviteService) DeleteInvite(ctx context.Context, inviteID string) error {
	invite, err := s.store.GetInvite(ctx, inviteID)
	if err != nil {
		if errors.Is(err, store.ErrInviteNotFound) {
			return domainerrors.NotFound("invite not found")
		}
		return fmt.Errorf("get invite: %w", err)
	}

	if invite.IsClaimed() {
		return domainerrors.Conflict("cannot revoke a claimed invite")
	}

	if err := s.store.DeleteInvite(ctx, inviteID); err != nil {
		return fmt.Errorf("delete invite: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Invite revoked",
			"invite_id", inviteID,
		)
	}

	return nil
}

// GetInviteURL returns the full URL for an invite code.
func (s *InviteService) GetInviteURL(code string) string {
	return s.serverURL + "/join/" + code
}

// generateInviteCode generates a cryptographically random, URL-safe invite code.
func generateInviteCode() (string, error) {
	b := make([]byte, inviteCodeSize)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
