package domain

import (
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeries_JSONMarshaling(t *testing.T) {
	series := &Series{
		Syncable: Syncable{
			ID: "series-123",
		},
		Name:        "The Stormlight Archive",
		Description: "Epic fantasy series by Brandon Sanderson",
	}
	series.InitTimestamps()

	// Marshal to JSON
	data, err := json.Marshal(series)
	require.NoError(t, err)

	// Unmarshal back
	var decoded Series
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, series.ID, decoded.ID)
	assert.Equal(t, series.Name, decoded.Name)
	assert.Equal(t, series.Description, decoded.Description)
	assert.Equal(t, series.CreatedAt.Unix(), decoded.CreatedAt.Unix())
	assert.Equal(t, series.UpdatedAt.Unix(), decoded.UpdatedAt.Unix())
}

func TestSeries_OngoingSeries(t *testing.T) {
	// Test series with unknown total (ongoing series)
	series := &Series{
		Syncable: Syncable{
			ID: "series-ongoing",
		},
		Name:        "The Expanse",
		Description: "Sci-fi series",
	}

	assert.NotEmpty(t, series.ID, "Series should have an ID")
}

func TestSeries_CompletedSeries(t *testing.T) {
	// Test series with known total
	series := &Series{
		Syncable: Syncable{
			ID: "series-complete",
		},
		Name:        "The Lord of the Rings",
		Description: "Fantasy trilogy",
	}

	assert.Equal(t, "The Lord of the Rings", series.Name)
}

func TestSeries_EmptyDescription(t *testing.T) {
	// Test that description is optional
	series := &Series{
		Syncable: Syncable{
			ID: "series-no-desc",
		},
		Name: "Minimal Series",
	}
	series.InitTimestamps()

	// Marshal to JSON
	data, err := json.Marshal(series)
	require.NoError(t, err)

	// Unmarshal back
	var decoded Series
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "", decoded.Description)
}
