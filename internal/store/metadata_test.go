package store

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/metadata/audible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchCache(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	region := audible.RegionUS
	query := "test query"

	// Initially empty
	cached, err := store.GetCachedSearch(ctx, region, query)
	require.NoError(t, err)
	assert.Nil(t, cached)

	// Set cache
	results := []audible.SearchResult{
		{ASIN: "B001", Title: "Test Book 1"},
		{ASIN: "B002", Title: "Test Book 2"},
	}
	err = store.SetCachedSearch(ctx, region, query, results)
	require.NoError(t, err)

	// Get cache hit
	cached, err = store.GetCachedSearch(ctx, region, query)
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.Equal(t, query, cached.Query)
	assert.Equal(t, region, cached.Region)
	assert.Len(t, cached.Results, 2)
	assert.Equal(t, "B001", cached.Results[0].ASIN)

	// Different query = miss
	cached, err = store.GetCachedSearch(ctx, region, "different query")
	require.NoError(t, err)
	assert.Nil(t, cached)

	// Different region = miss
	cached, err = store.GetCachedSearch(ctx, audible.RegionUK, query)
	require.NoError(t, err)
	assert.Nil(t, cached)

	// Delete
	err = store.DeleteCachedSearch(ctx, region, query)
	require.NoError(t, err)

	cached, err = store.GetCachedSearch(ctx, region, query)
	require.NoError(t, err)
	assert.Nil(t, cached)
}

func TestSearchCacheTTL(t *testing.T) {
	assert.Equal(t, 24*time.Hour, searchCacheDuration)
	assert.Equal(t, 7*24*time.Hour, bookCacheDuration)
	assert.Equal(t, 30*24*time.Hour, chapterCacheDuration)
}
