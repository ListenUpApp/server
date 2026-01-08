package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLibrary_GetAccessMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     AccessMode
		expected AccessMode
	}{
		{"empty defaults to open", "", AccessModeOpen},
		{"explicit open", AccessModeOpen, AccessModeOpen},
		{"explicit restricted", AccessModeRestricted, AccessModeRestricted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := &Library{AccessMode: tt.mode}
			assert.Equal(t, tt.expected, lib.GetAccessMode())
		})
	}
}

func TestLibrary_IsOpen(t *testing.T) {
	tests := []struct {
		name   string
		mode   AccessMode
		isOpen bool
	}{
		{"empty mode is open", "", true},
		{"explicit open is open", AccessModeOpen, true},
		{"restricted is not open", AccessModeRestricted, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := &Library{AccessMode: tt.mode}
			assert.Equal(t, tt.isOpen, lib.IsOpen())
		})
	}
}

func TestLibrary_IsRestricted(t *testing.T) {
	tests := []struct {
		name         string
		mode         AccessMode
		isRestricted bool
	}{
		{"empty mode is not restricted", "", false},
		{"explicit open is not restricted", AccessModeOpen, false},
		{"restricted is restricted", AccessModeRestricted, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := &Library{AccessMode: tt.mode}
			assert.Equal(t, tt.isRestricted, lib.IsRestricted())
		})
	}
}
