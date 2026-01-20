package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollection_GrantsGlobalAccess(t *testing.T) {
	tests := []struct {
		name           string
		isGlobalAccess bool
		expected       bool
	}{
		{"false by default", false, false},
		{"true when set", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coll := &Collection{IsGlobalAccess: tt.isGlobalAccess}
			assert.Equal(t, tt.expected, coll.GrantsGlobalAccess())
		})
	}
}

func TestCollection_IsSystemCollection_IncludesGlobalAccess(t *testing.T) {
	tests := []struct {
		name     string
		coll     *Collection
		isSystem bool
	}{
		{"regular collection", &Collection{}, false},
		{"inbox collection", &Collection{IsInbox: true}, true},
		{"global access collection is not system by default", &Collection{IsGlobalAccess: true}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isSystem, tt.coll.IsSystemCollection())
		})
	}
}
