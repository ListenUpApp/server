package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultPermissions(t *testing.T) {
	perms := DefaultPermissions()

	assert.True(t, perms.CanShare, "CanShare should default to true")
}

func TestUser_CanShare(t *testing.T) {
	tests := []struct {
		name        string
		permissions UserPermissions
		expected    bool
	}{
		{"true when allowed", UserPermissions{CanShare: true}, true},
		{"false when disallowed", UserPermissions{CanShare: false}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Permissions: tt.permissions}
			assert.Equal(t, tt.expected, user.CanShare())
		})
	}
}

func TestUserPermissions_ZeroValue(t *testing.T) {
	// Zero value should be restrictive (all false)
	// This ensures existing users without permissions set are safe by default
	var perms UserPermissions

	assert.False(t, perms.CanShare, "zero value CanShare should be false")
}
