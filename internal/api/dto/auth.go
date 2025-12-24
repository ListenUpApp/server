package dto

import "time"

// DeviceInfo contains information about the client device.
type DeviceInfo struct {
	DeviceID   string `json:"device_id,omitempty" doc:"Unique device identifier"`
	DeviceName string `json:"device_name,omitempty" doc:"Human-readable device name"`
	Platform   string `json:"platform,omitempty" doc:"Platform (android, ios, web)"`
	AppVersion string `json:"app_version,omitempty" doc:"Client application version"`
}

// SetupRequest is the request body for initial server setup.
type SetupRequest struct {
	Email     string `json:"email" validate:"required,email" doc:"Admin email address"`
	Password  string `json:"password" validate:"required,min=8,max=1024" doc:"Admin password (8-1024 chars)"`
	FirstName string `json:"first_name" validate:"required" doc:"Admin first name"`
	LastName  string `json:"last_name" validate:"required" doc:"Admin last name"`
}

// SetupInput wraps the setup request for huma.
type SetupInput struct {
	Body SetupRequest
}

// LoginRequest is the request body for user login.
type LoginRequest struct {
	Email      string     `json:"email" validate:"required,email" doc:"User email address"`
	Password   string     `json:"password" validate:"required" doc:"User password"`
	DeviceInfo DeviceInfo `json:"device_info,omitempty" doc:"Client device information"`
}

// LoginInput wraps the login request for huma.
type LoginInput struct {
	Body            LoginRequest
	XForwardedFor   string `header:"X-Forwarded-For" doc:"Client IP from proxy"`
	XRealIP         string `header:"X-Real-IP" doc:"Client real IP"`
	RemoteAddr      string `header:"Remote-Addr" doc:"Remote address"`
}

// AuthResponse is the response for successful authentication.
type AuthResponse struct {
	AccessToken  string `json:"access_token" doc:"PASETO access token"`
	RefreshToken string `json:"refresh_token" doc:"Refresh token for obtaining new access tokens"`
	ExpiresIn    int    `json:"expires_in" doc:"Access token expiry in seconds"`
	User         User   `json:"user" doc:"Authenticated user details"`
}

// AuthOutput wraps the auth response for huma.
type AuthOutput struct {
	Body AuthResponse
}

// RefreshRequest is the request body for token refresh.
type RefreshRequest struct {
	RefreshToken string     `json:"refresh_token" validate:"required" doc:"Refresh token from previous auth"`
	DeviceInfo   DeviceInfo `json:"device_info,omitempty" doc:"Updated device information"`
}

// RefreshInput wraps the refresh request for huma.
type RefreshInput struct {
	Body          RefreshRequest
	XForwardedFor string `header:"X-Forwarded-For" doc:"Client IP from proxy"`
	XRealIP       string `header:"X-Real-IP" doc:"Client real IP"`
}

// LogoutRequest is the request body for logout.
type LogoutRequest struct {
	SessionID string `json:"session_id" validate:"required" doc:"Session ID to revoke"`
}

// LogoutInput wraps the logout request for huma.
type LogoutInput struct {
	Body LogoutRequest
}

// User represents a user in API responses.
type User struct {
	ID        string    `json:"id" doc:"User ID"`
	Email     string    `json:"email" doc:"User email address"`
	Name      string    `json:"name" doc:"User display name"`
	Role      string    `json:"role" doc:"User role (admin, user)"`
	CreatedAt time.Time `json:"created_at" doc:"Account creation timestamp"`
	UpdatedAt time.Time `json:"updated_at" doc:"Last update timestamp"`
}

// UserOutput wraps a user response for huma.
type UserOutput struct {
	Body User
}

// InviteDetails represents public invite information.
type InviteDetails struct {
	Code         string     `json:"code" doc:"Invite code"`
	Role         string     `json:"role" doc:"Role granted by this invite"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty" doc:"Expiration timestamp"`
	ServerName   string     `json:"server_name" doc:"Name of the server"`
	InvitedBy    string     `json:"invited_by,omitempty" doc:"Name of inviter"`
}

// InviteDetailsOutput wraps invite details for huma.
type InviteDetailsOutput struct {
	Body InviteDetails
}

// ClaimInviteRequest is the request body for claiming an invite.
type ClaimInviteRequest struct {
	Email    string `json:"email" validate:"required,email" doc:"New user email address"`
	Password string `json:"password" validate:"required,min=8" doc:"New user password"`
	Name     string `json:"name,omitempty" doc:"New user display name"`
}

// ClaimInviteInput wraps the claim invite request for huma.
type ClaimInviteInput struct {
	Code string `path:"code" doc:"Invite code to claim"`
	Body ClaimInviteRequest
}
