package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultPaginationParams tests the default pagination parameters.
func TestDefaultPaginationParams(t *testing.T) {
	params := DefaultPaginationParms()
	assert.Equal(t, 100, params.Limit)
	assert.Empty(t, params.Cursor)
}

// TestPaginationParams_Validate tests validation of pagination parameters.
func TestPaginationParams_Validate(t *testing.T) {
	tests := []struct {
		name          string
		input         PaginationParams
		expectedLimit int
	}{
		{
			name:          "valid parameters",
			input:         PaginationParams{Limit: 50, Cursor: ""},
			expectedLimit: 50,
		},
		{
			name:          "zero limit should default to 100",
			input:         PaginationParams{Limit: 0, Cursor: ""},
			expectedLimit: 100,
		},
		{
			name:          "negative limit should default to 100",
			input:         PaginationParams{Limit: -10, Cursor: ""},
			expectedLimit: 100,
		},
		{
			name:          "limit over 1000 should cap at 1000",
			input:         PaginationParams{Limit: 5000, Cursor: ""},
			expectedLimit: 1000,
		},
		{
			name:          "limit exactly 1000 should stay at 1000",
			input:         PaginationParams{Limit: 1000, Cursor: ""},
			expectedLimit: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := tt.input
			params.Validate()
			assert.Equal(t, tt.expectedLimit, params.Limit)
		})
	}
}

// TestEncodeCursor tests cursor encoding.
func TestEncodeCursor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple key",
			input:    "book:001",
			expected: "Ym9vazowMDE=",
		},
		{
			name:     "complex key with special chars",
			input:    "idx:books:path:/test/path",
			expected: "aWR4OmJvb2tzOnBhdGg6L3Rlc3QvcGF0aA==",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeCursor(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDecodeCursor tests cursor decoding.
func TestDecodeCursor(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		shouldError bool
	}{
		{
			name:        "empty string",
			input:       "",
			expected:    "",
			shouldError: false,
		},
		{
			name:        "valid encoded cursor",
			input:       "Ym9vazowMDE=",
			expected:    "book:001",
			shouldError: false,
		},
		{
			name:        "complex encoded cursor",
			input:       "aWR4OmJvb2tzOnBhdGg6L3Rlc3QvcGF0aA==",
			expected:    "idx:books:path:/test/path",
			shouldError: false,
		},
		{
			name:        "invalid base64",
			input:       "not-valid-base64!!!",
			expected:    "",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeCursor(tt.input)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestEncodeDecode_RoundTrip tests encoding and decoding round trip.
func TestEncodeDecode_RoundTrip(t *testing.T) {
	tests := []string{
		"book:001",
		"book:test-book-id-123",
		"idx:books:path:/var/audiobooks/fiction",
		"collection:coll-456",
		"user:user-789",
	}

	for _, original := range tests {
		t.Run(original, func(t *testing.T) {
			encoded := EncodeCursor(original)
			decoded, err := DecodeCursor(encoded)
			require.NoError(t, err)
			assert.Equal(t, original, decoded)
		})
	}
}

// TestPaginatedResult_Structure tests the structure of paginated results.
func TestPaginatedResult_Structure(t *testing.T) {
	result := &PaginatedResult[string]{
		Items:      []string{"item1", "item2", "item3"},
		NextCursor: "cursor123",
		HasMore:    true,
		Total:      10,
	}

	assert.Len(t, result.Items, 3)
	assert.Equal(t, "cursor123", result.NextCursor)
	assert.True(t, result.HasMore)
	assert.Equal(t, 10, result.Total)
}

// TestPaginatedResult_NoMorePages tests paginated result with no more pages.
func TestPaginatedResult_NoMorePages(t *testing.T) {
	result := &PaginatedResult[string]{
		Items:      []string{"item1", "item2"},
		NextCursor: "",
		HasMore:    false,
		Total:      2,
	}

	assert.Len(t, result.Items, 2)
	assert.Empty(t, result.NextCursor)
	assert.False(t, result.HasMore)
	assert.Equal(t, 2, result.Total)
}

// TestPaginatedResult_EmptyResult tests paginated result with no items.
func TestPaginatedResult_EmptyResult(t *testing.T) {
	result := &PaginatedResult[string]{
		Items:      []string{},
		NextCursor: "",
		HasMore:    false,
		Total:      0,
	}

	assert.Empty(t, result.Items)
	assert.Empty(t, result.NextCursor)
	assert.False(t, result.HasMore)
	assert.Equal(t, 0, result.Total)
}
