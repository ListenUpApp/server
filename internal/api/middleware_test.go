package api

import (
	"encoding/json/v2"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvelopeTransformer_AlwaysIncludesVersion(t *testing.T) {
	tests := []struct {
		name   string
		status string
		input  any
	}{
		{
			name:   "success response",
			status: "200",
			input:  map[string]string{"key": "value"},
		},
		{
			name:   "created response",
			status: "201",
			input:  map[string]string{"id": "123"},
		},
		{
			name:   "no content response",
			status: "204",
			input:  nil,
		},
		{
			name:   "bad request error",
			status: "400",
			input:  errors.New("invalid input"),
		},
		{
			name:   "not found error",
			status: "404",
			input:  errors.New("resource not found"),
		},
		{
			name:   "conflict error with details",
			status: "409",
			input: &APIError{
				Code:    "conflict",
				Message: "Entity already exists",
				Details: map[string]string{"existing_id": "123"},
			},
		},
		{
			name:   "internal server error",
			status: "500",
			input:  errors.New("internal error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EnvelopeTransformer(nil, tt.status, tt.input)
			require.NoError(t, err)

			// Marshal to JSON and parse as generic map to check structure
			jsonBytes, err := json.Marshal(result)
			require.NoError(t, err)

			var envelope map[string]any
			err = json.Unmarshal(jsonBytes, &envelope)
			require.NoError(t, err)

			// Verify version field exists and has correct value
			require.Contains(t, envelope, "v", "Envelope must contain version field 'v'")
			assert.Equal(t, float64(EnvelopeVersion), envelope["v"], "Version must be %d", EnvelopeVersion)
		})
	}
}

func TestEnvelopeTransformer_SuccessResponse(t *testing.T) {
	data := map[string]string{"name": "Test Book"}

	result, err := EnvelopeTransformer(nil, "200", data)
	require.NoError(t, err)

	envelope, ok := result.(APIEnvelope)
	require.True(t, ok, "Expected APIEnvelope type")

	assert.Equal(t, EnvelopeVersion, envelope.Version)
	assert.True(t, envelope.Success)
	assert.Equal(t, data, envelope.Data)
	assert.Empty(t, envelope.Error)
}

func TestEnvelopeTransformer_ErrorResponse(t *testing.T) {
	result, err := EnvelopeTransformer(nil, "400", errors.New("validation failed"))
	require.NoError(t, err)

	envelope, ok := result.(APIEnvelope)
	require.True(t, ok, "Expected APIEnvelope type")

	assert.Equal(t, EnvelopeVersion, envelope.Version)
	assert.False(t, envelope.Success)
	assert.Nil(t, envelope.Data)
	assert.Equal(t, "validation failed", envelope.Error)
}

func TestEnvelopeTransformer_ErrorWithDetails(t *testing.T) {
	apiErr := &APIError{
		Code:    "disambiguation_required",
		Message: "Multiple matches found",
		Details: []string{"option1", "option2"},
	}

	result, err := EnvelopeTransformer(nil, "409", apiErr)
	require.NoError(t, err)

	envelope, ok := result.(APIErrorEnvelope)
	require.True(t, ok, "Expected APIErrorEnvelope type")

	assert.Equal(t, EnvelopeVersion, envelope.Version)
	assert.Equal(t, "disambiguation_required", envelope.Code)
	assert.Equal(t, "Multiple matches found", envelope.Message)
	assert.Equal(t, []string{"option1", "option2"}, envelope.Details)
}
