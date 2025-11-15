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

func TestCreateContributor(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	contributorID, err := id.Generate("contributor")
	require.NoError(t, err)

	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name:     "Brandon Sanderson",
		SortName: "Sanderson, Brandon",
	}
	contributor.InitTimestamps()

	err = store.CreateContributor(ctx, contributor)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := store.GetContributor(ctx, contributorID)
	require.NoError(t, err)
	assert.Equal(t, contributor.Name, retrieved.Name)
	assert.Equal(t, contributor.SortName, retrieved.SortName)
}

func TestCreateContributor_AlreadyExists(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	contributorID, err := id.Generate("contributor")
	require.NoError(t, err)

	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name: "Test Contributor",
	}
	contributor.InitTimestamps()

	err = store.CreateContributor(ctx, contributor)
	require.NoError(t, err)

	// Try to create again
	err = store.CreateContributor(ctx, contributor)
	assert.ErrorIs(t, err, ErrContributorExists)
}

func TestGetContributor_NotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetContributor(ctx, "nonexistent-id")
	assert.ErrorIs(t, err, ErrContributorNotFound)
}

func TestUpdateContributor(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	contributorID, err := id.Generate("contributor")
	require.NoError(t, err)

	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name: "Brandon Sanderson",
	}
	contributor.InitTimestamps()

	err = store.CreateContributor(ctx, contributor)
	require.NoError(t, err)

	// Update the contributor
	contributor.Biography = "Epic fantasy author"
	err = store.UpdateContributor(ctx, contributor)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetContributor(ctx, contributorID)
	require.NoError(t, err)
	assert.Equal(t, "Epic fantasy author", retrieved.Biography)
}

func TestUpdateContributor_NameChange(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	contributorID, err := id.Generate("contributor")
	require.NoError(t, err)

	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name: "Old Name",
	}
	contributor.InitTimestamps()

	err = store.CreateContributor(ctx, contributor)
	require.NoError(t, err)

	// Change the name
	contributor.Name = "New Name"
	err = store.UpdateContributor(ctx, contributor)
	require.NoError(t, err)

	// Verify we can find by new name
	found, err := store.GetOrCreateContributorByName(ctx, "New Name")
	require.NoError(t, err)
	assert.Equal(t, contributorID, found.ID)
}

func TestGetOrCreateContributorByName_Create(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	contributor, err := store.GetOrCreateContributorByName(ctx, "Brandon Sanderson")
	require.NoError(t, err)
	assert.NotEmpty(t, contributor.ID)
	assert.Equal(t, "Brandon Sanderson", contributor.Name)
}

func TestGetOrCreateContributorByName_ExistingReturned(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create first time
	first, err := store.GetOrCreateContributorByName(ctx, "Brandon Sanderson")
	require.NoError(t, err)

	// Get second time - should return same contributor
	second, err := store.GetOrCreateContributorByName(ctx, "Brandon Sanderson")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
}

func TestGetOrCreateContributorByName_Normalization(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create with one spelling
	first, err := store.GetOrCreateContributorByName(ctx, "Brandon Sanderson")
	require.NoError(t, err)

	// Try variations - should all return the same contributor
	variations := []string{
		"BRANDON SANDERSON",
		"brandon sanderson",
		"  Brandon   Sanderson  ",
		"Brandon  Sanderson", // Multiple spaces
	}

	for _, variant := range variations {
		contributor, err := store.GetOrCreateContributorByName(ctx, variant)
		require.NoError(t, err, "Failed for variant: %s", variant)
		assert.Equal(t, first.ID, contributor.ID, "Different ID for variant: %s", variant)
	}
}

func TestListContributors(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple contributors
	names := []string{"Author A", "Author B", "Author C"}
	for _, name := range names {
		_, err := store.GetOrCreateContributorByName(ctx, name)
		require.NoError(t, err)
	}

	// List all
	result, err := store.ListContributors(ctx, PaginationParams{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, result.Items, 3)
	assert.False(t, result.HasMore)
}

func TestListContributors_Pagination(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create 5 contributors
	for i := 0; i < 5; i++ {
		name := string('A' + rune(i))
		_, err := store.GetOrCreateContributorByName(ctx, name)
		require.NoError(t, err)
	}

	// Get first page (limit 2)
	page1, err := store.ListContributors(ctx, PaginationParams{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, page1.Items, 2)
	assert.True(t, page1.HasMore)
	assert.NotEmpty(t, page1.NextCursor)

	// Get second page
	page2, err := store.ListContributors(ctx, PaginationParams{Limit: 2, Cursor: page1.NextCursor})
	require.NoError(t, err)
	assert.Len(t, page2.Items, 2)
	assert.True(t, page2.HasMore)

	// Get third page
	page3, err := store.ListContributors(ctx, PaginationParams{Limit: 2, Cursor: page2.NextCursor})
	require.NoError(t, err)
	assert.Len(t, page3.Items, 1)
	assert.False(t, page3.HasMore)
}

func TestGetContributorsUpdatedAfter(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Record start time
	startTime := time.Now()

	// Create contributor before timestamp
	oldContributor, err := store.GetOrCreateContributorByName(ctx, "Old Contributor")
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)
	checkpointTime := time.Now()
	time.Sleep(10 * time.Millisecond)

	// Create contributor after timestamp
	newContributor, err := store.GetOrCreateContributorByName(ctx, "New Contributor")
	require.NoError(t, err)

	// Query for contributors updated after checkpoint
	contributors, err := store.GetContributorsUpdatedAfter(ctx, checkpointTime)
	require.NoError(t, err)

	// Should only include new contributor
	assert.Len(t, contributors, 1)
	assert.Equal(t, newContributor.ID, contributors[0].ID)

	// Query from start should include both
	allContributors, err := store.GetContributorsUpdatedAfter(ctx, startTime)
	require.NoError(t, err)
	assert.Len(t, allContributors, 2)

	// Verify both are included
	ids := []string{allContributors[0].ID, allContributors[1].ID}
	assert.Contains(t, ids, oldContributor.ID)
	assert.Contains(t, ids, newContributor.ID)
}

func TestGetContributorsByIDs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create contributors
	contributor1, err := store.GetOrCreateContributorByName(ctx, "Contributor 1")
	require.NoError(t, err)

	contributor2, err := store.GetOrCreateContributorByName(ctx, "Contributor 2")
	require.NoError(t, err)

	// Get by IDs
	contributors, err := store.GetContributorsByIDs(ctx, []string{contributor1.ID, contributor2.ID})
	require.NoError(t, err)
	assert.Len(t, contributors, 2)

	// Verify IDs
	ids := []string{contributors[0].ID, contributors[1].ID}
	assert.Contains(t, ids, contributor1.ID)
	assert.Contains(t, ids, contributor2.ID)
}

func TestGetContributorsByIDs_SkipMissing(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create one contributor
	contributor, err := store.GetOrCreateContributorByName(ctx, "Existing")
	require.NoError(t, err)

	// Request existing and non-existing IDs
	contributors, err := store.GetContributorsByIDs(ctx, []string{contributor.ID, "nonexistent-id"})
	require.NoError(t, err)

	// Should only return the existing one
	assert.Len(t, contributors, 1)
	assert.Equal(t, contributor.ID, contributors[0].ID)
}

func TestTouchContributor(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	contributor, err := store.GetOrCreateContributorByName(ctx, "Test Contributor")
	require.NoError(t, err)

	originalUpdatedAt := contributor.UpdatedAt

	// Wait and touch
	time.Sleep(10 * time.Millisecond)
	err = store.touchContributor(ctx, contributor.ID)
	require.NoError(t, err)

	// Verify timestamp updated
	retrieved, err := store.GetContributor(ctx, contributor.ID)
	require.NoError(t, err)
	assert.True(t, retrieved.UpdatedAt.After(originalUpdatedAt))
}

func TestNormalizeContributorName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Brandon Sanderson", "brandon sanderson"},
		{"BRANDON SANDERSON", "brandon sanderson"},
		{"  Brandon   Sanderson  ", "brandon sanderson"},
		{"Brandon  Sanderson", "brandon sanderson"},
		{"  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeContributorName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
