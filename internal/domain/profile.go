package domain

import "time"

// AvatarType represents the type of avatar a user has.
type AvatarType string

const (
	// AvatarTypeAuto uses hash-based color with initials (default).
	AvatarTypeAuto AvatarType = "auto"
	// AvatarTypeImage uses an uploaded image file.
	AvatarTypeImage AvatarType = "image"
)

// UserProfile contains user customization settings.
// Stored separately from User to keep auth concerns separate from social features.
type UserProfile struct {
	UserID      string     `json:"user_id"`
	AvatarType  AvatarType `json:"avatar_type"`
	AvatarValue string     `json:"avatar_value"` // Image path (empty for auto)
	Tagline     string     `json:"tagline"`      // Max 60 characters
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// NewUserProfile creates a default profile for a user.
func NewUserProfile(userID string) *UserProfile {
	now := time.Now()
	return &UserProfile{
		UserID:     userID,
		AvatarType: AvatarTypeAuto,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// AvatarImagePath returns the image path if avatar type is image, empty otherwise.
func (p *UserProfile) AvatarImagePath() string {
	if p.AvatarType == AvatarTypeImage {
		return p.AvatarValue
	}
	return ""
}
