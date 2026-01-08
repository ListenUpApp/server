package domain

import "time"

// Role represents the user's permission level in the system.
type Role string

const (
	// RoleAdmin grants full administrative access.
	RoleAdmin Role = "admin"
	// RoleMember grants standard user access.
	RoleMember Role = "member"
)

// UserStatus represents the user's account status.
type UserStatus string

const (
	// UserStatusActive indicates the user can log in and use the system.
	UserStatusActive UserStatus = "active"
	// UserStatusPending indicates the user is awaiting admin approval.
	UserStatusPending UserStatus = "pending"
)

// UserPermissions defines action-level permissions for a user.
// These control what actions a user can perform, not what content they can see.
// Content visibility is controlled by Library.AccessMode and Collections.
type UserPermissions struct {
	// CanDownload allows downloading audio files for offline use.
	// When false, user can only stream (useful for bandwidth/storage control).
	// Default: true for all roles.
	CanDownload bool `json:"can_download"`

	// CanShare allows creating collection shares with other users.
	// When false, user can receive shares but cannot grant them.
	// Useful for child accounts who shouldn't redistribute content.
	// Default: true for all roles.
	CanShare bool `json:"can_share"`
}

// DefaultPermissions returns the default permissions for new users.
// All permissions default to true - restrictions are opt-in.
func DefaultPermissions() UserPermissions {
	return UserPermissions{
		CanDownload: true,
		CanShare:    true,
	}
}

// User represents an authenticated user account in the system.
type User struct {
	Syncable
	Email        string          `json:"email"`
	PasswordHash string          `json:"password_hash,omitempty"` // Stored hashed, filter from API responses
	IsRoot       bool            `json:"is_root"`
	Role         Role            `json:"role"`                 // admin or member
	Status       UserStatus      `json:"status,omitempty"`     // active or pending (empty = active for backward compat)
	InvitedBy    string          `json:"invited_by,omitempty"` // User ID who invited this user
	ApprovedBy   string          `json:"approved_by,omitempty"`
	ApprovedAt   time.Time       `json:"approved_at,omitempty"`
	DisplayName  string          `json:"display_name"`
	FirstName    string          `json:"first_name"`
	LastName     string          `json:"last_name"`
	LastLoginAt  time.Time       `json:"last_login_at"`
	Permissions  UserPermissions `json:"permissions"` // Action-level permissions
}

// IsAdmin returns true if the user has administrative privileges.
// Root users are automatically admins, regardless of their role field.
func (u *User) IsAdmin() bool {
	return u.IsRoot || u.Role == RoleAdmin
}

// IsActive returns true if the user can log in and use the system.
// Empty status is treated as active for backward compatibility with existing users.
func (u *User) IsActive() bool {
	return u.Status == "" || u.Status == UserStatusActive
}

// IsPending returns true if the user is awaiting admin approval.
func (u *User) IsPending() bool {
	return u.Status == UserStatusPending
}

// CanDownload returns true if the user is allowed to download content.
func (u *User) CanDownload() bool {
	return u.Permissions.CanDownload
}

// CanShare returns true if the user is allowed to share collections.
func (u *User) CanShare() bool {
	return u.Permissions.CanShare
}

// FullName returns the user's full name, composed from first and last names.
func (u *User) FullName() string {
	if u.FirstName == "" && u.LastName == "" {
		return u.DisplayName
	}
	if u.FirstName == "" {
		return u.LastName
	}
	if u.LastName == "" {
		return u.FirstName
	}
	return u.FirstName + " " + u.LastName
}

// Name returns the best available name to display for the user.
// Prefers DisplayName, falls back to FullName, then email.
func (u *User) Name() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	fullName := u.FullName()
	if fullName != "" {
		return fullName
	}
	return u.Email
}

// Session represents an active user session with refresh token.
// Each device gets its own session - you can see what's connected.
type Session struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	RefreshTokenHash string    `json:"refresh_token_hash,omitempty"` // Stored hashed, filter from API responses
	ExpiresAt        time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	IPAddress        string    `json:"ip_address,omitempty"`

	// Device information - structured data from client
	DeviceType      string `json:"device_type"`               // mobile, tablet, desktop, web, tv
	Platform        string `json:"platform"`                  // iOS, Android, Windows, macOS, Linux, Web
	PlatformVersion string `json:"platform_version"`          // 17.2, 14.0, 11, etc.
	ClientName      string `json:"client_name"`               // ListenUp Mobile, ListenUp Web
	ClientVersion   string `json:"client_version"`            // 1.0.0
	ClientBuild     string `json:"client_build,omitempty"`    // 245 (optional)
	DeviceName      string `json:"device_name,omitempty"`     // Simon's iPhone (optional, user-set)
	DeviceModel     string `json:"device_model,omitempty"`    // iPhone 15 Pro, Pixel 8 (optional)
	BrowserName     string `json:"browser_name,omitempty"`    // Chrome, Firefox, Safari (web only)
	BrowserVersion  string `json:"browser_version,omitempty"` // 120.0.6099.109 (web only)
}

// Touch updates the session's last seen timestamp.
func (s *Session) Touch() {
	s.LastSeenAt = time.Now()
}

// IsExpired checks if the session has passed its expiration time.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// DisplayName returns a human-readable description of the device.
// This follows the priority logic from DeviceInfo.DisplayName().
func (s *Session) DisplayName() string {
	if s.DeviceName != "" {
		return s.DeviceName
	}

	if s.DeviceModel != "" {
		// "iPhone 15 Pro - iOS 17.2"
		if s.PlatformVersion != "" {
			return s.DeviceModel + " - " + s.Platform + " " + s.PlatformVersion
		}
		// "iPhone 15 Pro"
		return s.DeviceModel
	}

	if s.Platform != "" {
		// "iOS 17.2"
		if s.PlatformVersion != "" {
			return s.Platform + " " + s.PlatformVersion
		}
		// "iOS"
		return s.Platform
	}

	// "ListenUp Mobile 1.0.0"
	if s.ClientVersion != "" {
		return s.ClientName + " " + s.ClientVersion
	}

	// "ListenUp Mobile"
	if s.ClientName != "" {
		return s.ClientName
	}

	return "Unknown Device"
}

// ShortName returns a concise device identifier for space-constrained UI.
func (s *Session) ShortName() string {
	if s.DeviceName != "" {
		return s.DeviceName
	}
	if s.DeviceModel != "" {
		return s.DeviceModel
	}
	if s.BrowserName != "" {
		return s.BrowserName
	}
	return s.Platform
}
