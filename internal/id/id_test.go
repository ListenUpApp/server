package id

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_Uniqueness(t *testing.T) {
	// Generate many IDs and verify they're unique
	ids := make(map[string]bool)
	count := 1000

	for i := 0; i < count; i++ {
		id, err := Generate("test")
		require.NoError(t, err)
		assert.False(t, ids[id], "ID should be unique: %s", id)
		ids[id] = true
	}

	assert.Len(t, ids, count)
}

func TestGenerate_Format(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{"library", "lib"},
		{"collection", "coll"},
		{"book", "book"},
		{"user", "user"},
		{"author", "author"},
		{"session", "sess"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := Generate(tt.prefix)
			require.NoError(t, err)

			// Should start with prefix followed by hyphen
			assert.True(t, strings.HasPrefix(id, tt.prefix+"-"))

			// Should not be empty
			assert.NotEmpty(t, id)

			// NanoID default is 21 characters
			// Total should be len(prefix) + 1 (hyphen) + 21
			expectedLen := len(tt.prefix) + 1 + 21
			assert.Equal(t, expectedLen, len(id), "ID: %s", id)

			// Extract the NanoID part (everything after the prefix and hyphen)
			nanoidPart := strings.TrimPrefix(id, tt.prefix+"-")
			assert.Len(t, nanoidPart, 21, "NanoID part should be 21 characters")

			// Check all characters are URL-safe (NanoID uses: A-Za-z0-9_-)
			for _, char := range nanoidPart {
				assert.True(t,
					(char >= 'A' && char <= 'Z') ||
						(char >= 'a' && char <= 'z') ||
						(char >= '0' && char <= '9') ||
						char == '_' || char == '-',
					"Character %c should be URL-safe", char)
			}
		})
	}
}

func TestMustGenerate_Format(t *testing.T) {
	id := MustGenerate("test")

	assert.True(t, strings.HasPrefix(id, "test-"))
	assert.Equal(t, len("test")+1+21, len(id))
}

func TestMustGenerate_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		id := MustGenerate("test")
		assert.False(t, ids[id], "ID should be unique: %s", id)
		ids[id] = true
	}

	assert.Len(t, ids, count)
}

func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = Generate("bench")
	}
}

func BenchmarkMustGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		MustGenerate("bench")
	}
}
