package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/store"
)

// validate is a shared validator instance for request validation.
var validate = func() *validator.Validate {
	v := validator.New()
	// Use JSON tag names for field names in error messages
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := fld.Tag.Get("json")
		if name == "" {
			return fld.Name
		}
		// Remove any options (like omitempty, -)
		for i := range len(name) {
			if name[i] == ',' {
				return name[:i]
			}
		}
		return name
	})
	return v
}()

// AuthService handles user authentication (login, setup, token verification).
// Session management is delegated to SessionService.
type AuthService struct {
	store           store.Store
	tokenService    *auth.TokenService
	sessionService  *SessionService
	instanceService *InstanceService
	logger          *slog.Logger
}

// NewAuthService creates a new authentication service.
func NewAuthService(
	store store.Store,
	tokenService *auth.TokenService,
	sessionService *SessionService,
	instanceService *InstanceService,
	logger *slog.Logger,
) *AuthService {
	return &AuthService{
		store:           store,
		tokenService:    tokenService,
		sessionService:  sessionService,
		instanceService: instanceService,
		logger:          logger,
	}
}

// SetupRequest contains the initial root user creation data.
type SetupRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8,max=1024"`
	FirstName string `json:"first_name" validate:"required"`
	LastName  string `json:"last_name" validate:"required"`
}

// LoginRequest contains user credentials and device information.
type LoginRequest struct {
	Email      string          `json:"email" validate:"required,email"`
	Password   string          `json:"password" validate:"required"`
	DeviceInfo auth.DeviceInfo `json:"device_info"`
	IPAddress  string          `json:"-"` // Extracted from request by handler
}

// RefreshRequest contains the refresh token and updated device info.
type RefreshRequest struct {
	RefreshToken string          `json:"refresh_token" validate:"required"`
	DeviceInfo   auth.DeviceInfo `json:"device_info"` // Optional updates
	IPAddress    string          `json:"-"`           // Extracted from request by handler
}

// RegisterRequest contains user registration data for open registration.
type RegisterRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8,max=1024"`
	FirstName string `json:"first_name" validate:"required"`
	LastName  string `json:"last_name" validate:"required"`
}

// RegisterResponse contains the result of a registration request.
type RegisterResponse struct {
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

// AuthResponse contains authentication tokens and user data.
type AuthResponse struct {
	User *domain.User `json:"user"`
	SessionResponse
}

// Setup creates the first user (root) and completes initial server configuration.
// This endpoint can only be used once, before any users exist.
func (s *AuthService) Setup(ctx context.Context, req SetupRequest) (*AuthResponse, error) {
	// Validate request
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	// Verify setup is required
	setupRequired, err := s.instanceService.IsSetupRequired(ctx)
	if err != nil {
		return nil, fmt.Errorf("check setup status: %w", err)
	}
	if !setupRequired {
		return nil, domainerrors.AlreadyConfigured("server is already configured")
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create root user
	userID, err := id.Generate("user")
	if err != nil {
		return nil, fmt.Errorf("generate user ID: %w", err)
	}

	now := time.Now()
	user := &domain.User{
		Syncable: domain.Syncable{
			ID: userID,
		},
		Email:        req.Email,
		PasswordHash: passwordHash,
		IsRoot:       true,             // First user is root
		Role:         domain.RoleAdmin, // Root user is always admin
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		DisplayName:  req.FirstName + " " + req.LastName, // Auto-generate from names
		LastLoginAt:  now,
		Permissions:  domain.DefaultPermissions(),
	}
	user.InitTimestamps()

	// Save user
	if err := s.store.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrEmailExists) {
			return nil, domainerrors.AlreadyExists("email already in use")
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Configure instance with root user
	if err := s.instanceService.SetRootUser(ctx, userID); err != nil {
		return nil, fmt.Errorf("configure instance: %w", err)
	}

	// Create initial session
	// Setup happens via web UI, so use basic web device info
	deviceInfo := auth.DeviceInfo{
		DeviceType:    "web",
		Platform:      "Web",
		ClientName:    "ListenUp Web",
		ClientVersion: "1.0.0",
	}

	sessionResp, err := s.sessionService.CreateSession(ctx, user, deviceInfo, "")
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Server setup complete",
			"user_id", userID,
			"email", user.Email,
		)
	}

	return &AuthResponse{
		User:            user,
		SessionResponse: *sessionResp,
	}, nil
}

// Register creates a new user account when open registration is enabled.
// The user is created with pending status and must be approved by an admin.
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	// Validate request
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	// Check if open registration is enabled
	instance, err := s.instanceService.GetInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if !instance.OpenRegistration {
		return nil, domainerrors.Forbidden("registration is not open")
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create user with pending status
	userID, err := id.Generate("user")
	if err != nil {
		return nil, fmt.Errorf("generate user ID: %w", err)
	}

	user := &domain.User{
		Syncable: domain.Syncable{
			ID: userID,
		},
		Email:        req.Email,
		PasswordHash: passwordHash,
		IsRoot:       false,
		Role:         domain.RoleMember,
		Status:       domain.UserStatusPending,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		DisplayName:  req.FirstName + " " + req.LastName,
		Permissions:  domain.DefaultPermissions(),
	}
	user.InitTimestamps()

	// Save user
	if err := s.store.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrEmailExists) {
			return nil, domainerrors.AlreadyExists("email already in use")
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Broadcast SSE event for admin users
	s.store.BroadcastUserPending(user)

	if s.logger != nil {
		s.logger.Info("User registered (pending approval)",
			"user_id", userID,
			"email", user.Email,
		)
	}

	return &RegisterResponse{
		UserID:  userID,
		Message: "Registration submitted. Please wait for admin approval.",
	}, nil
}

// Login authenticates a user and creates a new session.
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	// Validate request
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	// Validate device info
	if !req.DeviceInfo.IsValid() {
		return nil, domainerrors.Validation("device_info is required (device_type and platform)")
	}

	// Find user by email
	user, err := s.store.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			// Don't leak whether email exists
			return nil, domainerrors.InvalidCredentials("invalid email or password")
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	// Verify password
	valid, err := auth.VerifyPassword(user.PasswordHash, req.Password)
	if err != nil {
		return nil, fmt.Errorf("verify password: %w", err)
	}
	if !valid {
		return nil, domainerrors.InvalidCredentials("invalid email or password")
	}

	// Check if user is pending approval
	if user.IsPending() {
		return nil, domainerrors.Forbidden("your account is pending admin approval")
	}

	// Update last login
	user.LastLoginAt = time.Now()
	user.Touch()
	if err := s.store.UpdateUser(ctx, user); err != nil {
		// Log but don't fail login
		if s.logger != nil {
			s.logger.Warn("Failed to update last login time",
				"user_id", user.ID,
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
		s.logger.Info("User logged in",
			"user_id", user.ID,
			"device", req.DeviceInfo.Platform,
		)
	}

	return &AuthResponse{
		User:            user,
		SessionResponse: *sessionResp,
	}, nil
}

// RefreshTokens generates new tokens using a refresh token.
// The old refresh token is invalidated (token rotation).
func (s *AuthService) RefreshTokens(ctx context.Context, req RefreshRequest) (*AuthResponse, error) {
	// Validate request
	if err := validate.Struct(req); err != nil {
		return nil, formatValidationError(err)
	}

	sessionResp, user, err := s.sessionService.RefreshSession(ctx, req.RefreshToken, req.DeviceInfo, req.IPAddress)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		User:            user,
		SessionResponse: *sessionResp,
	}, nil
}

// Logout revokes a session, invalidating its refresh token.
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	return s.sessionService.DeleteSession(ctx, sessionID)
}

// VerifyAccessToken validates a token and returns the associated user.
// Used by authentication middleware.
func (s *AuthService) VerifyAccessToken(ctx context.Context, tokenString string) (*domain.User, *auth.AccessClaims, error) {
	// Verify and parse token
	claims, err := s.tokenService.VerifyAccessToken(tokenString)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid token: %w", err)
	}

	// Get user
	user, err := s.store.GetUser(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return nil, nil, errors.New("user not found")
		}
		return nil, nil, fmt.Errorf("get user: %w", err)
	}

	return user, claims, nil
}

// formatValidationError converts validator errors to user-friendly domain errors.
func formatValidationError(err error) error {
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		// Return first validation error as a domain error
		for _, e := range validationErrs {
			field := e.Field()
			switch e.Tag() {
			case "required":
				return domainerrors.Validationf("%s is required", field)
			case "email":
				return domainerrors.Validationf("%s must be a valid email address", field)
			case "min":
				return domainerrors.Validationf("%s must be at least %s characters", field, e.Param())
			case "max":
				return domainerrors.Validationf("%s exceeds maximum length of %s characters", field, e.Param())
			default:
				return domainerrors.Validationf("%s is invalid", field)
			}
		}
	}
	return err
}
