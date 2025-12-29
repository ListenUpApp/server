package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerAuthRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "setup",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/setup",
		Summary:     "Initial server setup",
		Description: "Creates the first admin user. Can only be called once.",
		Tags:        []string{"Authentication"},
	}, s.handleSetup)

	huma.Register(s.api, huma.Operation{
		OperationID: "register",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/register",
		Summary:     "Register new user",
		Description: "Creates a new user account (requires open registration to be enabled). User will be in pending status until approved by admin.",
		Tags:        []string{"Authentication"},
	}, s.handleRegister)

	huma.Register(s.api, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/login",
		Summary:     "User login",
		Description: "Authenticates a user and returns access and refresh tokens",
		Tags:        []string{"Authentication"},
	}, s.handleLogin)

	huma.Register(s.api, huma.Operation{
		OperationID: "refresh",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/refresh",
		Summary:     "Refresh tokens",
		Description: "Exchanges a refresh token for new tokens",
		Tags:        []string{"Authentication"},
	}, s.handleRefresh)

	huma.Register(s.api, huma.Operation{
		OperationID: "logout",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/logout",
		Summary:     "Logout",
		Description: "Revokes the specified session",
		Tags:        []string{"Authentication"},
	}, s.handleLogout)
}

// === DTOs ===

// DeviceInfo contains device metadata for session tracking.
type DeviceInfo struct {
	DeviceType      string `json:"device_type,omitempty" validate:"omitempty,max=50" doc:"Device type (mobile, tablet, desktop, web, tv)"`
	Platform        string `json:"platform,omitempty" validate:"omitempty,max=50" doc:"Platform (iOS, Android, Windows, macOS, Linux, Web)"`
	PlatformVersion string `json:"platform_version,omitempty" validate:"omitempty,max=50" doc:"Platform version (17.2, 14.0, etc.)"`
	ClientName      string `json:"client_name,omitempty" validate:"omitempty,max=100" doc:"Client name (ListenUp Mobile, etc.)"`
	ClientVersion   string `json:"client_version,omitempty" validate:"omitempty,max=50" doc:"Client version (1.0.0)"`
	ClientBuild     string `json:"client_build,omitempty" validate:"omitempty,max=50" doc:"Client build number"`
	DeviceName      string `json:"device_name,omitempty" validate:"omitempty,max=100" doc:"Human-readable device name"`
	DeviceModel     string `json:"device_model,omitempty" validate:"omitempty,max=100" doc:"Device model (iPhone 15 Pro, etc.)"`
	BrowserName     string `json:"browser_name,omitempty" validate:"omitempty,max=100" doc:"Browser name (for web clients)"`
	BrowserVersion  string `json:"browser_version,omitempty" validate:"omitempty,max=50" doc:"Browser version (for web clients)"`
}

// SetupRequest is the request body for initial server setup.
type SetupRequest struct {
	Email     string `json:"email" validate:"required,email,max=254" doc:"Admin email address"`
	Password  string `json:"password" validate:"required,min=8,max=1024" doc:"Admin password"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100" doc:"Admin first name"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100" doc:"Admin last name"`
}

// SetupInput wraps the setup request for Huma.
type SetupInput struct {
	Body SetupRequest
}

// LoginRequest is the request body for user login.
type LoginRequest struct {
	Email      string     `json:"email" validate:"required,email,max=254" doc:"User email"`
	Password   string     `json:"password" validate:"required,max=1024" doc:"User password"`
	DeviceInfo DeviceInfo `json:"device_info,omitempty" doc:"Client device info"`
}

// LoginInput wraps the login request with headers for Huma.
type LoginInput struct {
	Body          LoginRequest
	XForwardedFor string `header:"X-Forwarded-For"`
	XRealIP       string `header:"X-Real-IP"`
}

// RefreshRequest is the request body for token refresh.
type RefreshRequest struct {
	RefreshToken string     `json:"refresh_token" validate:"required" doc:"Refresh token"`
	DeviceInfo   DeviceInfo `json:"device_info,omitempty" doc:"Updated device info"`
}

// RefreshInput wraps the refresh request with headers for Huma.
type RefreshInput struct {
	Body          RefreshRequest
	XForwardedFor string `header:"X-Forwarded-For"`
	XRealIP       string `header:"X-Real-IP"`
}

// LogoutRequest is the request body for logout.
type LogoutRequest struct {
	SessionID string `json:"session_id" validate:"required,max=100" doc:"Session ID to revoke"`
}

// LogoutInput wraps the logout request for Huma.
type LogoutInput struct {
	Body LogoutRequest
}

// RegisterRequest is the request body for user registration.
type RegisterRequest struct {
	Email     string `json:"email" validate:"required,email,max=254" doc:"User email address"`
	Password  string `json:"password" validate:"required,min=8,max=1024" doc:"User password"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100" doc:"User first name"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100" doc:"User last name"`
}

// RegisterInput wraps the register request for Huma.
type RegisterInput struct {
	Body RegisterRequest
}

// RegisterResponse contains the result of a registration.
type RegisterResponse struct {
	UserID  string `json:"user_id" doc:"Created user ID"`
	Message string `json:"message" doc:"Status message"`
}

// RegisterOutput wraps the register response for Huma.
type RegisterOutput struct {
	Body RegisterResponse
}

// UserResponse contains user information in auth responses.
type UserResponse struct {
	ID          string    `json:"id" doc:"User ID"`
	Email       string    `json:"email" doc:"User email"`
	DisplayName string    `json:"display_name" doc:"Display name"`
	FirstName   string    `json:"first_name" doc:"First name"`
	LastName    string    `json:"last_name" doc:"Last name"`
	IsRoot      bool      `json:"is_root" doc:"Whether user is root admin"`
	CreatedAt   time.Time `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt   time.Time `json:"updated_at" doc:"Last update timestamp"`
	LastLoginAt time.Time `json:"last_login_at" doc:"Last login timestamp"`
}

// AuthResponse contains authentication tokens and user info.
type AuthResponse struct {
	AccessToken  string       `json:"access_token" doc:"PASETO access token"`
	RefreshToken string       `json:"refresh_token" doc:"Refresh token"`
	SessionID    string       `json:"session_id" doc:"Session identifier"`
	TokenType    string       `json:"token_type" doc:"Token type (Bearer)"`
	ExpiresIn    int          `json:"expires_in" doc:"Token expiry in seconds"`
	User         UserResponse `json:"user" doc:"Authenticated user"`
}

// AuthOutput wraps the auth response for Huma.
type AuthOutput struct {
	Body AuthResponse
}

// MessageResponse contains a simple message.
type MessageResponse struct {
	Message string `json:"message" doc:"Success message"`
}

// MessageOutput wraps the message response for Huma.
type MessageOutput struct {
	Body MessageResponse
}

// === Handlers ===

func (s *Server) handleSetup(ctx context.Context, input *SetupInput) (*AuthOutput, error) {
	req := service.SetupRequest{
		Email:     input.Body.Email,
		Password:  input.Body.Password,
		FirstName: input.Body.FirstName,
		LastName:  input.Body.LastName,
	}

	resp, err := s.services.Auth.Setup(ctx, req)
	if err != nil {
		return nil, err
	}

	return &AuthOutput{Body: mapAuthResponse(resp)}, nil
}

func (s *Server) handleRegister(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
	req := service.RegisterRequest{
		Email:     input.Body.Email,
		Password:  input.Body.Password,
		FirstName: input.Body.FirstName,
		LastName:  input.Body.LastName,
	}

	resp, err := s.services.Auth.Register(ctx, req)
	if err != nil {
		return nil, err
	}

	return &RegisterOutput{
		Body: RegisterResponse{
			UserID:  resp.UserID,
			Message: resp.Message,
		},
	}, nil
}

func (s *Server) handleLogin(ctx context.Context, input *LoginInput) (*AuthOutput, error) {
	req := service.LoginRequest{
		Email:    input.Body.Email,
		Password: input.Body.Password,
		DeviceInfo: auth.DeviceInfo{
			DeviceType:      input.Body.DeviceInfo.DeviceType,
			Platform:        input.Body.DeviceInfo.Platform,
			PlatformVersion: input.Body.DeviceInfo.PlatformVersion,
			ClientName:      input.Body.DeviceInfo.ClientName,
			ClientVersion:   input.Body.DeviceInfo.ClientVersion,
			ClientBuild:     input.Body.DeviceInfo.ClientBuild,
			DeviceName:      input.Body.DeviceInfo.DeviceName,
			DeviceModel:     input.Body.DeviceInfo.DeviceModel,
		},
		IPAddress: extractIP(input.XForwardedFor, input.XRealIP),
	}

	resp, err := s.services.Auth.Login(ctx, req)
	if err != nil {
		return nil, err
	}

	return &AuthOutput{Body: mapAuthResponse(resp)}, nil
}

func (s *Server) handleRefresh(ctx context.Context, input *RefreshInput) (*AuthOutput, error) {
	req := service.RefreshRequest{
		RefreshToken: input.Body.RefreshToken,
		DeviceInfo: auth.DeviceInfo{
			DeviceType:      input.Body.DeviceInfo.DeviceType,
			Platform:        input.Body.DeviceInfo.Platform,
			PlatformVersion: input.Body.DeviceInfo.PlatformVersion,
			ClientName:      input.Body.DeviceInfo.ClientName,
			ClientVersion:   input.Body.DeviceInfo.ClientVersion,
			ClientBuild:     input.Body.DeviceInfo.ClientBuild,
			DeviceName:      input.Body.DeviceInfo.DeviceName,
			DeviceModel:     input.Body.DeviceInfo.DeviceModel,
		},
		IPAddress: extractIP(input.XForwardedFor, input.XRealIP),
	}

	resp, err := s.services.Auth.RefreshTokens(ctx, req)
	if err != nil {
		return nil, err
	}

	return &AuthOutput{Body: mapAuthResponse(resp)}, nil
}

func (s *Server) handleLogout(ctx context.Context, input *LogoutInput) (*MessageOutput, error) {
	if err := s.services.Auth.Logout(ctx, input.Body.SessionID); err != nil {
		return nil, err
	}

	return &MessageOutput{Body: MessageResponse{Message: "Logged out successfully"}}, nil
}

// === Helpers ===

func mapAuthResponse(resp *service.AuthResponse) AuthResponse {
	return AuthResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		SessionID:    resp.SessionID,
		TokenType:    resp.TokenType,
		ExpiresIn:    resp.ExpiresIn,
		User: UserResponse{
			ID:          resp.User.ID,
			Email:       resp.User.Email,
			DisplayName: resp.User.DisplayName,
			FirstName:   resp.User.FirstName,
			LastName:    resp.User.LastName,
			IsRoot:      resp.User.IsRoot,
			CreatedAt:   resp.User.CreatedAt,
			UpdatedAt:   resp.User.UpdatedAt,
			LastLoginAt: resp.User.LastLoginAt,
		},
	}
}

func extractIP(xForwardedFor, xRealIP string) string {
	if xForwardedFor != "" {
		for i := 0; i < len(xForwardedFor); i++ {
			if xForwardedFor[i] == ',' {
				return xForwardedFor[:i]
			}
		}
		return xForwardedFor
	}
	return xRealIP
}
