package auth

import (
	"time"
)

// AccessClaims represents the claims stored in a PASETO access token.
// These are encrypted in v4.local tokens, so they're not readable without the key.
type AccessClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	IsRoot bool   `json:"is_root"`

	// Standard PASETO claims
	Issuer     string    `json:"iss"`
	Subject    string    `json:"sub"`
	Audience   string    `json:"aud"`
	Expiration time.Time `json:"exp"`
	NotBefore  time.Time `json:"nbf"`
	IssuedAt   time.Time `json:"iat"`
	TokenID    string    `json:"jti"`
}

// DeviceInfo represents information sent by the client about itself.
// This gets stored in Session and is used for display and security.
type DeviceInfo struct {
	// Core identification
	DeviceType string `json:"device_type"` // mobile, tablet, desktop, web, tv

	// Platform details
	Platform        string `json:"platform"`         // iOS, Android, Windows, macOS, Linux, Web
	PlatformVersion string `json:"platform_version"` // 17.2, 14.0, 11, etc.

	// App/Client details
	ClientName    string `json:"client_name"`    // ListenUp Mobile, ListenUp Web
	ClientVersion string `json:"client_version"` // 1.0.0
	ClientBuild   string `json:"client_build"`   // 245 (optional)

	// Device identification (optional, user-set)
	DeviceName  string `json:"device_name"`  // Simon's iPhone, Work Laptop
	DeviceModel string `json:"device_model"` // iPhone 15 Pro, Pixel 8, MacBook Pro

	// Browser-specific (for web clients)
	BrowserName    string `json:"browser_name"`    // Chrome, Firefox, Safari
	BrowserVersion string `json:"browser_version"` // 120.0.6099.109
}

// IsValid performs basic validation on the device info.
func (d DeviceInfo) IsValid() bool {
	// At minimum, we need device type and platform
	return d.DeviceType != "" && d.Platform != ""
}
