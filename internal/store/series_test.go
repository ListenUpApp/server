package store

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSeries(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	seriesID, err := id.Generate("series")
	require.NoError(t, err)

	series := &domain.Series{
		Syncable: domain.Syncable{
			ID: seriesID,
		},
		Name:        "The Stormlight Archive",
		Description: "Epic fantasy series",
	}
	series.InitTimestamps()

	err = store.CreateSeries(ctx, series)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := store.GetSeries(ctx, seriesID)
	require.NoError(t, err)
	assert.Equal(t, series.Name, retrieved.Name)
	assert.Equal(t, series.Description, retrieved.Description)
}

func TestCreateSeries_AlreadyExists(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	seriesID, err := id.Generate("series")
	require.NoError(t, err)

	series := &domain.Series{
		Syncable: domain.Syncable{
			ID: seriesID,
		},
		Name: "Test Series",
	}
	series.InitTimestamps()

	err = store.CreateSeries(ctx, series)
	require.NoError(t, err)

	// Try to create again
	err = store.CreateSeries(ctx, series)
	assert.ErrorIs(t, err, ErrSeriesExists)
}

func TestGetSeries_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetSeries(ctx, "nonexistent-id")
	assert.ErrorIs(t, err, ErrSeriesNotFound)
}

func TestUpdateSeries(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	seriesID, err := id.Generate("series")
	require.NoError(t, err)

	series := &domain.Series{
		Syncable: domain.Syncable{
			ID: seriesID,
		},
		Name: "The Expanse",
	}
	series.InitTimestamps()

	err = store.CreateSeries(ctx, series)
	require.NoError(t, err)

	// Update the series
	series.Description = "Sci-fi series"
	err = store.UpdateSeries(ctx, series)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetSeries(ctx, seriesID)
	require.NoError(t, err)
	assert.Equal(t, "Sci-fi series", retrieved.Description)
}

func TestUpdateSeries_NameChange(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	seriesID, err := id.Generate("series")
	require.NoError(t, err)

	series := &domain.Series{
		Syncable: domain.Syncable{
			ID: seriesID,
		},
		Name: "Old Name",
	}
	series.InitTimestamps()

	err = store.CreateSeries(ctx, series)
	require.NoError(t, err)

	// Change the name
	series.Name = "New Name"
	err = store.UpdateSeries(ctx, series)
	require.NoError(t, err)

	// Verify we can find by new name
	found, err := store.GetOrCreateSeriesByName(ctx, "New Name")
	require.NoError(t, err)
	assert.Equal(t, seriesID, found.ID)
}

func TestGetOrCreateSeriesByName_Create(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	series, err := store.GetOrCreateSeriesByName(ctx, "The Stormlight Archive")
	require.NoError(t, err)
	assert.NotEmpty(t, series.ID)
	assert.Equal(t, "The Stormlight Archive", series.Name)
}

func TestGetOrCreateSeriesByName_ExistingReturned(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create first time
	first, err := store.GetOrCreateSeriesByName(ctx, "The Stormlight Archive")
	require.NoError(t, err)

	// Get second time - should return same series
	second, err := store.GetOrCreateSeriesByName(ctx, "The Stormlight Archive")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
}

func TestGetOrCreateSeriesByName_Normalization(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create with one spelling
	first, err := store.GetOrCreateSeriesByName(ctx, "The Stormlight Archive")
	require.NoError(t, err)

	// Try variations - should all return the same series
	variations := []string{
		"THE STORMLIGHT ARCHIVE",
		"the stormlight archive",
		"  The   Stormlight  Archive  ",
		"The  Stormlight  Archive", // Multiple spaces
	}

	for _, variant := range variations {
		series, err := store.GetOrCreateSeriesByName(ctx, variant)
		require.NoError(t, err, "Failed for variant: %s", variant)
		assert.Equal(t, first.ID, series.ID, "Different ID for variant: %s", variant)
	}
}

func TestListSeries(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple series
	names := []string{"Series A", "Series B", "Series C"}
	for _, name := range names {
		_, err := store.GetOrCreateSeriesByName(ctx, name)
		require.NoError(t, err)
	}

	// List all
	result, err := store.ListSeries(ctx, PaginationParams{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, result.Items, 3)
	assert.False(t, result.HasMore)
}

func TestListSeries_Pagination(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create 5 series
	for i := range 5 {
		name := string('A' + rune(i))
		_, err := store.GetOrCreateSeriesByName(ctx, name)
		require.NoError(t, err)
	}

	// Get first page (limit 2)
	page1, err := store.ListSeries(ctx, PaginationParams{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, page1.Items, 2)
	assert.True(t, page1.HasMore)
	assert.NotEmpty(t, page1.NextCursor)

	// Get second page
	page2, err := store.ListSeries(ctx, PaginationParams{Limit: 2, Cursor: page1.NextCursor})
	require.NoError(t, err)
	assert.Len(t, page2.Items, 2)
	assert.True(t, page2.HasMore)

	// Get third page
	page3, err := store.ListSeries(ctx, PaginationParams{Limit: 2, Cursor: page2.NextCursor})
	require.NoError(t, err)
	assert.Len(t, page3.Items, 1)
	assert.False(t, page3.HasMore)
}

func TestGetSeriesUpdatedAfter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Record start time
	startTime := time.Now()

	// Create series before timestamp
	oldSeries, err := store.GetOrCreateSeriesByName(ctx, "Old Series")
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)
	checkpointTime := time.Now()
	time.Sleep(10 * time.Millisecond)

	// Create series after timestamp
	newSeries, err := store.GetOrCreateSeriesByName(ctx, "New Series")
	require.NoError(t, err)

	// Query for series updated after checkpoint
	seriesList, err := store.GetSeriesUpdatedAfter(ctx, checkpointTime)
	require.NoError(t, err)

	// Should only include new series
	assert.Len(t, seriesList, 1)
	assert.Equal(t, newSeries.ID, seriesList[0].ID)

	// Query from start should include both
	allSeries, err := store.GetSeriesUpdatedAfter(ctx, startTime)
	require.NoError(t, err)
	assert.Len(t, allSeries, 2)

	// Verify both are included
	ids := []string{allSeries[0].ID, allSeries[1].ID}
	assert.Contains(t, ids, oldSeries.ID)
	assert.Contains(t, ids, newSeries.ID)
}

func TestTouchSeries(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	series, err := store.GetOrCreateSeriesByName(ctx, "Test Series")
	require.NoError(t, err)

	originalUpdatedAt := series.UpdatedAt

	// Wait and touch
	time.Sleep(10 * time.Millisecond)
	err = store.touchSeries(ctx, series.ID)
	require.NoError(t, err)

	// Verify timestamp updated
	retrieved, err := store.GetSeries(ctx, series.ID)
	require.NoError(t, err)
	assert.True(t, retrieved.UpdatedAt.After(originalUpdatedAt))
}

func TestNormalizeSeriesName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The Stormlight Archive", "the stormlight archive"},
		{"THE STORMLIGHT ARCHIVE", "the stormlight archive"},
		{"  The   Stormlight  Archive  ", "the stormlight archive"},
		{"The  Stormlight  Archive", "the stormlight archive"},
		{"  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSeriesName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
