package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlexTime_UnmarshalJSON_RFC3339(t *testing.T) {
	input := `"2024-01-15T10:30:00Z"`
	var ft FlexTime
	err := json.Unmarshal([]byte(input), &ft)
	require.NoError(t, err)

	expected := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	assert.Equal(t, expected, ft.Time)
}

func TestFlexTime_UnmarshalJSON_RFC3339Nano(t *testing.T) {
	input := `"2024-01-15T10:30:00.123456789Z"`
	var ft FlexTime
	err := json.Unmarshal([]byte(input), &ft)
	require.NoError(t, err)

	expected := time.Date(2024, 1, 15, 10, 30, 0, 123456789, time.UTC)
	assert.Equal(t, expected, ft.Time)
}

func TestFlexTime_UnmarshalJSON_EpochMs_Number(t *testing.T) {
	// 2024-01-15T10:30:00Z in epoch milliseconds
	input := `1705314600000`
	var ft FlexTime
	err := json.Unmarshal([]byte(input), &ft)
	require.NoError(t, err)

	expected := time.UnixMilli(1705314600000)
	assert.Equal(t, expected, ft.Time)
}

func TestFlexTime_UnmarshalJSON_EpochMs_String(t *testing.T) {
	// Same time as above, but as string
	input := `"1705314600000"`
	var ft FlexTime
	err := json.Unmarshal([]byte(input), &ft)
	require.NoError(t, err)

	expected := time.UnixMilli(1705314600000)
	assert.Equal(t, expected, ft.Time)
}

func TestFlexTime_MarshalJSON(t *testing.T) {
	ft := FlexTime{Time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)}
	data, err := json.Marshal(ft)
	require.NoError(t, err)

	assert.Equal(t, `"2024-01-15T10:30:00Z"`, string(data))
}

func TestFlexTime_InStruct(t *testing.T) {
	type TestStruct struct {
		StartedAt FlexTime `json:"started_at"`
		EndedAt   FlexTime `json:"ended_at"`
	}

	// Test with mixed formats
	input := `{"started_at":"2024-01-15T10:30:00Z","ended_at":1705314600000}`
	var ts TestStruct
	err := json.Unmarshal([]byte(input), &ts)
	require.NoError(t, err)

	assert.Equal(t, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), ts.StartedAt.Time)
	assert.Equal(t, time.UnixMilli(1705314600000), ts.EndedAt.Time)
}
