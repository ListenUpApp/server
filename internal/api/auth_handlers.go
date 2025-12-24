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

type DeviceInfo struct {
	DeviceType      string `json:"device_type,omitempty" validate:"omitempty,max=50" doc:"Device type (mobile, tablet, desktop, web, tv)"`
	Platform        string `json:"platform,omitempty" validate:"omitempty,max=50" doc:"Platform (iOS, Android, Windows, macOS, Linux, Web)"`
	PlatformVersion string `json:"platform_version,omitempty" validate:"omitempty,max=50" doc:"Platform version (17.2, 14.0, etc.)"`
	ClientName      string `json:"client_name,omitempty" validate:"omitempty,max=100" doc:"Client name (ListenUp Mobile, etc.)"`
	ClientVersion   string `json:"client_version,omitempty" validate:"omitempty,max=50" doc:"Client version (1.0.0)"`
	ClientBuild     string `json:"client_build,omitempty" validate:"omitempty,max=50" doc:"Client build number"`
	DeviceName      string `json:"device_name,omitempty" validate:"omitempty,max=100" doc:"Human-readable device name"`
	DeviceModel     string `json:"device_model,omitempty" validate:"omitempty,max=100" doc:"Device model (iPhone 15 Pro, etc.)"`
}

type SetupRequest struct {
	Email     string `json:"email" validate:"required,email,max=254" doc:"Admin email address"`
	Password  string `json:"password" validate:"required,min=8,max=1024" doc:"Admin password"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100" doc:"Admin first name"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100" doc:"Admin last name"`
}

type SetupInput struct {
	Body SetupRequest
}

type LoginRequest struct {
	Email      string     `json:"email" validate:"required,email,max=254" doc:"User email"`
	Password   string     `json:"password" validate:"required,max=1024" doc:"User password"`
	DeviceInfo DeviceInfo `json:"device_info,omitempty" doc:"Client device info"`
}

type LoginInput struct {
	Body          LoginRequest
	XForwardedFor string `header:"X-Forwarded-For"`
	XRealIP       string `header:"X-Real-IP"`
}

type RefreshRequest struct {
	RefreshToken string     `json:"refresh_token" validate:"required" doc:"Refresh token"`
	DeviceInfo   DeviceInfo `json:"device_info,omitempty" doc:"Updated device info"`
}

type RefreshInput struct {
	Body          RefreshRequest
	XForwardedFor string `header:"X-Forwarded-For"`
	XRealIP       string `header:"X-Real-IP"`
}

type LogoutRequest struct {
	SessionID string `json:"session_id" validate:"required,max=100" doc:"Session ID to revoke"`
}

type LogoutInput struct {
	Body LogoutRequest
}

type UserResponse struct {
	ID        string    `json:"id" doc:"User ID"`
	Email     string    `json:"email" doc:"User email"`
	Name      string    `json:"name" doc:"Display name"`
	Role      string    `json:"role" doc:"User role"`
	CreatedAt time.Time `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt time.Time `json:"updated_at" doc:"Last update timestamp"`
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token" doc:"PASETO access token"`
	RefreshToken string       `json:"refresh_token" doc:"Refresh token"`
	ExpiresIn    int          `json:"expires_in" doc:"Token expiry in seconds"`
	User         UserResponse `json:"user" doc:"Authenticated user"`
}

type AuthOutput struct {
	Body AuthResponse
}

type MessageResponse struct {
	Message string `json:"message" doc:"Success message"`
}

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
		ExpiresIn:    resp.ExpiresIn,
		User: UserResponse{
			ID:        resp.User.ID,
			Email:     resp.User.Email,
			Name:      resp.User.FirstName + " " + resp.User.LastName,
			Role:      string(resp.User.Role),
			CreatedAt: resp.User.CreatedAt,
			UpdatedAt: resp.User.UpdatedAt,
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
