package domain

import "time"

// Invite represents an invitation to join the server.
// Invites are created by admins and claimed by new users during registration.
type Invite struct {
	Syncable
	Code      string     `json:"code"`                 // Unique, URL-safe invite code
	Name      string     `json:"name"`                 // Display name for the invitee
	Email     string     `json:"email"`                // Pre-filled email for registration
	Role      Role       `json:"role"`                 // Role to assign on claim (admin or member)
	CreatedBy string     `json:"created_by"`           // Admin user ID who created the invite
	ExpiresAt time.Time  `json:"expires_at"`           // When the invite expires
	ClaimedAt *time.Time `json:"claimed_at,omitempty"` // When the invite was claimed
	ClaimedBy string     `json:"claimed_by,omitempty"` // User ID who claimed the invite
}

// IsClaimed returns true if the invite has been used.
func (i *Invite) IsClaimed() bool {
	return i.ClaimedAt != nil
}

// IsExpired returns true if the invite has passed its expiration time.
func (i *Invite) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// IsValid returns true if the invite can still be claimed.
// An invite is valid if it hasn't been claimed, hasn't expired, and hasn't been deleted.
func (i *Invite) IsValid() bool {
	return !i.IsClaimed() && !i.IsExpired() && !i.IsDeleted()
}

// Status returns a human-readable status string for the invite.
func (i *Invite) Status() string {
	if i.IsDeleted() {
		return "revoked"
	}
	if i.IsClaimed() {
		return "claimed"
	}
	if i.IsExpired() {
		return "expired"
	}
	return "pending"
}
