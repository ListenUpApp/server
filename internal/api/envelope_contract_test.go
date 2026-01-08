package api

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getFixturePath returns the path to the testdata directory within the server repo.
// Client tests embed matching JSON strings to verify parsing compatibility.
func getFixturePath(t *testing.T) string {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "Failed to get caller info")

	// Navigate from server/internal/api to server/testdata/envelope
	serverDir := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
	return filepath.Join(serverDir, "testdata", "envelope")
}

// TestEnvelopeContract_SuccessMatchesFixture verifies the server produces
// exactly the same JSON structure as defined in the shared fixture.
func TestEnvelopeContract_SuccessMatchesFixture(t *testing.T) {
	fixturePath := filepath.Join(getFixturePath(t), "success.json")
	fixtureBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "Failed to read fixture file - contract tests require shared fixtures")

	// Parse fixture to understand expected structure
	var expected map[string]any
	err = json.Unmarshal(fixtureBytes, &expected)
	require.NoError(t, err)

	// Generate envelope using server's EnvelopeTransformer
	data := map[string]string{"id": "test-123", "name": "Test Item"}
	result, err := EnvelopeTransformer(nil, "200", data)
	require.NoError(t, err)

	// Marshal server output
	serverBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var serverOutput map[string]any
	err = json.Unmarshal(serverBytes, &serverOutput)
	require.NoError(t, err)

	// Verify structure matches fixture
	assert.Equal(t, expected["v"], serverOutput["v"], "Version field 'v' must match fixture")
	assert.Equal(t, expected["success"], serverOutput["success"], "Success field must match fixture")
	assert.Contains(t, serverOutput, "data", "Response must contain 'data' field")

	// Verify no unexpected fields
	for key := range serverOutput {
		assert.Contains(t, expected, key, "Server output contains unexpected field: %s", key)
	}
}

// TestEnvelopeContract_SuccessNullDataMatchesFixture verifies success responses
// without data match the fixture structure.
func TestEnvelopeContract_SuccessNullDataMatchesFixture(t *testing.T) {
	fixturePath := filepath.Join(getFixturePath(t), "success_null_data.json")
	fixtureBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "Failed to read fixture file")

	var expected map[string]any
	err = json.Unmarshal(fixtureBytes, &expected)
	require.NoError(t, err)

	// Generate envelope with nil data
	result, err := EnvelopeTransformer(nil, "204", nil)
	require.NoError(t, err)

	serverBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var serverOutput map[string]any
	err = json.Unmarshal(serverBytes, &serverOutput)
	require.NoError(t, err)

	// Verify required fields
	assert.Equal(t, expected["v"], serverOutput["v"], "Version field must match")
	assert.Equal(t, expected["success"], serverOutput["success"], "Success field must match")
}

// TestEnvelopeContract_SimpleErrorMatchesFixture verifies simple error responses
// match the fixture structure.
func TestEnvelopeContract_SimpleErrorMatchesFixture(t *testing.T) {
	fixturePath := filepath.Join(getFixturePath(t), "error_simple.json")
	fixtureBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "Failed to read fixture file")

	var expected map[string]any
	err = json.Unmarshal(fixtureBytes, &expected)
	require.NoError(t, err)

	// Generate error envelope
	result, err := EnvelopeTransformer(nil, "404", &APIError{Message: "Resource not found"})
	require.NoError(t, err)

	serverBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var serverOutput map[string]any
	err = json.Unmarshal(serverBytes, &serverOutput)
	require.NoError(t, err)

	// Verify structure matches fixture
	assert.Equal(t, expected["v"], serverOutput["v"], "Version field must match")
	assert.Equal(t, expected["success"], serverOutput["success"], "Success must be false")
	assert.Contains(t, serverOutput, "error", "Must contain 'error' field")
	assert.IsType(t, "", serverOutput["error"], "Error must be a string")
}

// TestEnvelopeContract_DetailedErrorMatchesFixture verifies detailed error responses
// with code/message/details match the fixture structure.
func TestEnvelopeContract_DetailedErrorMatchesFixture(t *testing.T) {
	fixturePath := filepath.Join(getFixturePath(t), "error_detailed.json")
	fixtureBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "Failed to read fixture file")

	var expected map[string]any
	err = json.Unmarshal(fixtureBytes, &expected)
	require.NoError(t, err)

	// Generate detailed error envelope
	result, err := EnvelopeTransformer(nil, "409", &APIError{
		Code:    "conflict",
		Message: "Entity already exists",
		Details: map[string]string{"existing_id": "abc-123"},
	})
	require.NoError(t, err)

	serverBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var serverOutput map[string]any
	err = json.Unmarshal(serverBytes, &serverOutput)
	require.NoError(t, err)

	// Verify structure matches fixture
	assert.Equal(t, expected["v"], serverOutput["v"], "Version field must match")
	assert.Contains(t, serverOutput, "code", "Must contain 'code' field")
	assert.Contains(t, serverOutput, "message", "Must contain 'message' field")
	assert.Contains(t, serverOutput, "details", "Must contain 'details' field")

	// Verify types
	assert.IsType(t, "", serverOutput["code"], "Code must be a string")
	assert.IsType(t, "", serverOutput["message"], "Message must be a string")
}

// TestEnvelopeContract_VersionFieldName verifies the version field is named exactly 'v'.
// This is critical - if renamed to 'version', client will break silently.
func TestEnvelopeContract_VersionFieldName(t *testing.T) {
	result, err := EnvelopeTransformer(nil, "200", nil)
	require.NoError(t, err)

	serverBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var serverOutput map[string]any
	err = json.Unmarshal(serverBytes, &serverOutput)
	require.NoError(t, err)

	// CRITICAL: Field must be 'v', not 'version' or anything else
	assert.Contains(t, serverOutput, "v", "Must use 'v' as version field name")
	assert.NotContains(t, serverOutput, "version", "Must NOT use 'version' as field name")
	assert.NotContains(t, serverOutput, "Version", "Must NOT use 'Version' as field name")
}
