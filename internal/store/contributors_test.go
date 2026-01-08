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
	for i := range 5 {
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

// --- Alias and Merge Tests ---

func TestGetOrCreateContributorByName_FindsByAlias(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create contributor with alias
	contributorID, err := id.Generate("contributor")
	require.NoError(t, err)

	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name:    "Stephen King",
		Aliases: []string{"Richard Bachman", "John Swithen"},
	}
	contributor.InitTimestamps()

	err = store.CreateContributor(ctx, contributor)
	require.NoError(t, err)

	// Search by alias should find the contributor
	found, foundByAlias, err := store.GetOrCreateContributorByNameWithAlias(ctx, "Richard Bachman")
	require.NoError(t, err)
	assert.True(t, foundByAlias, "Should have found by alias")
	assert.Equal(t, contributorID, found.ID)
	assert.Equal(t, "Stephen King", found.Name)
}

func TestGetOrCreateContributorByName_AliasCaseInsensitive(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	contributorID, err := id.Generate("contributor")
	require.NoError(t, err)

	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name:    "Stephen King",
		Aliases: []string{"Richard Bachman"},
	}
	contributor.InitTimestamps()

	err = store.CreateContributor(ctx, contributor)
	require.NoError(t, err)

	// Case variations should all find the contributor
	variations := []string{
		"RICHARD BACHMAN",
		"richard bachman",
		"  Richard   Bachman  ",
	}

	for _, variant := range variations {
		found, foundByAlias, err := store.GetOrCreateContributorByNameWithAlias(ctx, variant)
		require.NoError(t, err, "Failed for variant: %s", variant)
		assert.True(t, foundByAlias, "Should have found by alias for: %s", variant)
		assert.Equal(t, contributorID, found.ID, "Wrong ID for variant: %s", variant)
	}
}

func TestMergeContributors_Basic(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create source contributor
	source, err := store.GetOrCreateContributorByName(ctx, "Richard Bachman")
	require.NoError(t, err)

	// Create target contributor
	target, err := store.GetOrCreateContributorByName(ctx, "Stephen King")
	require.NoError(t, err)

	// Merge source into target
	merged, err := store.MergeContributors(ctx, source.ID, target.ID)
	require.NoError(t, err)

	// Verify merge result
	assert.Equal(t, target.ID, merged.ID)
	assert.Equal(t, "Stephen King", merged.Name)
	assert.Contains(t, merged.Aliases, "Richard Bachman")

	// Source should be soft-deleted
	_, err = store.GetContributor(ctx, source.ID)
	assert.ErrorIs(t, err, ErrContributorNotFound)

	// Future searches for "Richard Bachman" should find Stephen King
	found, foundByAlias, err := store.GetOrCreateContributorByNameWithAlias(ctx, "Richard Bachman")
	require.NoError(t, err)
	assert.True(t, foundByAlias)
	assert.Equal(t, target.ID, found.ID)
}

func TestMergeContributors_WithBooks(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create source contributor
	source, err := store.GetOrCreateContributorByName(ctx, "Richard Bachman")
	require.NoError(t, err)

	// Create target contributor
	target, err := store.GetOrCreateContributorByName(ctx, "Stephen King")
	require.NoError(t, err)

	// Create a test book with source contributor
	bookID := "book-merge-test"
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Title: "The Long Walk",
		Path:  "/test/books/long-walk",
		Contributors: []domain.BookContributor{
			{
				ContributorID: source.ID,
				Roles:         []domain.ContributorRole{domain.RoleAuthor},
			},
		},
	}
	err = store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Merge source into target
	merged, err := store.MergeContributors(ctx, source.ID, target.ID)
	require.NoError(t, err)

	// Verify target has the alias
	assert.Contains(t, merged.Aliases, "Richard Bachman")

	// Verify book now has target contributor with creditedAs
	updatedBook, err := store.GetBook(ctx, bookID, "")
	require.NoError(t, err)

	require.Len(t, updatedBook.Contributors, 1)
	assert.Equal(t, target.ID, updatedBook.Contributors[0].ContributorID)
	assert.Equal(t, "Richard Bachman", updatedBook.Contributors[0].CreditedAs)
	assert.Contains(t, updatedBook.Contributors[0].Roles, domain.RoleAuthor)
}

func TestMergeContributors_PreservesExistingAliases(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create source
	source, err := store.GetOrCreateContributorByName(ctx, "Richard Bachman")
	require.NoError(t, err)

	// Create target with existing alias
	targetID, err := id.Generate("contributor")
	require.NoError(t, err)

	target := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: targetID,
		},
		Name:    "Stephen King",
		Aliases: []string{"Existing Alias"},
	}
	target.InitTimestamps()

	err = store.CreateContributor(ctx, target)
	require.NoError(t, err)

	// Merge
	merged, err := store.MergeContributors(ctx, source.ID, targetID)
	require.NoError(t, err)

	// Should have both aliases
	assert.Len(t, merged.Aliases, 2)
	assert.Contains(t, merged.Aliases, "Existing Alias")
	assert.Contains(t, merged.Aliases, "Richard Bachman")
}

func TestMergeContributors_SourceNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	target, err := store.GetOrCreateContributorByName(ctx, "Stephen King")
	require.NoError(t, err)

	_, err = store.MergeContributors(ctx, "nonexistent-id", target.ID)
	assert.ErrorIs(t, err, ErrContributorNotFound)
}

func TestMergeContributors_TargetNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	source, err := store.GetOrCreateContributorByName(ctx, "Richard Bachman")
	require.NoError(t, err)

	_, err = store.MergeContributors(ctx, source.ID, "nonexistent-id")
	assert.ErrorIs(t, err, ErrContributorNotFound)
}

func TestMergeContributors_MergesRoles(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create source and target
	source, err := store.GetOrCreateContributorByName(ctx, "Source Contributor")
	require.NoError(t, err)

	target, err := store.GetOrCreateContributorByName(ctx, "Target Contributor")
	require.NoError(t, err)

	// Create book with both contributors in different roles
	bookID := "book-role-merge"
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID:        bookID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Title: "Test Book",
		Path:  "/test/books/role-merge",
		Contributors: []domain.BookContributor{
			{
				ContributorID: source.ID,
				Roles:         []domain.ContributorRole{domain.RoleNarrator},
			},
			{
				ContributorID: target.ID,
				Roles:         []domain.ContributorRole{domain.RoleAuthor},
			},
		},
	}
	err = store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Merge source into target
	_, err = store.MergeContributors(ctx, source.ID, target.ID)
	require.NoError(t, err)

	// Book should now have one contributor with merged roles
	updatedBook, err := store.GetBook(ctx, bookID, "")
	require.NoError(t, err)

	require.Len(t, updatedBook.Contributors, 1)
	assert.Equal(t, target.ID, updatedBook.Contributors[0].ContributorID)
	assert.Len(t, updatedBook.Contributors[0].Roles, 2)
	assert.Contains(t, updatedBook.Contributors[0].Roles, domain.RoleAuthor)
	assert.Contains(t, updatedBook.Contributors[0].Roles, domain.RoleNarrator)
}

func TestUpdateContributor_AliasIndexUpdate(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create contributor without aliases
	contributorID, err := id.Generate("contributor")
	require.NoError(t, err)

	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: contributorID,
		},
		Name: "Stephen King",
	}
	contributor.InitTimestamps()

	err = store.CreateContributor(ctx, contributor)
	require.NoError(t, err)

	// Add alias via update
	contributor.Aliases = []string{"Richard Bachman"}
	err = store.UpdateContributor(ctx, contributor)
	require.NoError(t, err)

	// Should now be findable by alias
	found, foundByAlias, err := store.GetOrCreateContributorByNameWithAlias(ctx, "Richard Bachman")
	require.NoError(t, err)
	assert.True(t, foundByAlias)
	assert.Equal(t, contributorID, found.ID)

	// Remove alias via update
	contributor.Aliases = nil
	err = store.UpdateContributor(ctx, contributor)
	require.NoError(t, err)

	// Should no longer be findable by old alias
	found, foundByAlias, err = store.GetOrCreateContributorByNameWithAlias(ctx, "Richard Bachman")
	require.NoError(t, err)
	assert.False(t, foundByAlias)
	assert.Equal(t, "Richard Bachman", found.Name) // Should have created new contributor
}

// =============================================================================
// Unmerge Tests
// =============================================================================

func TestUnmergeContributor_Basic(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create Stephen King with an alias
	kingID, err := id.Generate("contributor")
	require.NoError(t, err)

	king := &domain.Contributor{
		Syncable: domain.Syncable{ID: kingID},
		Name:     "Stephen King",
		Aliases:  []string{"Richard Bachman"},
	}
	king.InitTimestamps()
	err = store.CreateContributor(ctx, king)
	require.NoError(t, err)

	// Unmerge Richard Bachman
	newContributor, err := store.UnmergeContributor(ctx, kingID, "Richard Bachman")
	require.NoError(t, err)

	// Verify new contributor was created
	assert.Equal(t, "Richard Bachman", newContributor.Name)
	assert.NotEqual(t, kingID, newContributor.ID)

	// Verify alias was removed from source
	updatedKing, err := store.GetContributor(ctx, kingID)
	require.NoError(t, err)
	assert.Empty(t, updatedKing.Aliases)

	// Verify new contributor is findable by name
	found, err := store.GetContributor(ctx, newContributor.ID)
	require.NoError(t, err)
	assert.Equal(t, "Richard Bachman", found.Name)
}

func TestUnmergeContributor_RelinksBooks(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create Stephen King with an alias
	kingID, err := id.Generate("contributor")
	require.NoError(t, err)

	king := &domain.Contributor{
		Syncable: domain.Syncable{ID: kingID},
		Name:     "Stephen King",
		Aliases:  []string{"Richard Bachman"},
	}
	king.InitTimestamps()
	err = store.CreateContributor(ctx, king)
	require.NoError(t, err)

	// Create a book credited to Richard Bachman (but linked to Stephen King)
	bookID, err := id.Generate("book")
	require.NoError(t, err)

	book := &domain.Book{
		Syncable: domain.Syncable{ID: bookID},
		Title:    "The Running Man",
		Contributors: []domain.BookContributor{
			{
				ContributorID: kingID,
				Roles:         []domain.ContributorRole{domain.RoleAuthor},
				CreditedAs:    "Richard Bachman",
			},
		},
	}
	book.InitTimestamps()
	err = store.CreateBook(ctx, book)
	require.NoError(t, err)

	// Unmerge Richard Bachman
	newContributor, err := store.UnmergeContributor(ctx, kingID, "Richard Bachman")
	require.NoError(t, err)

	// Verify the book was re-linked to the new contributor
	// Use GetBooksByContributor to avoid needing user access setup
	books, err := store.GetBooksByContributor(ctx, newContributor.ID)
	require.NoError(t, err)
	require.Len(t, books, 1)

	updatedBook := books[0]
	require.Len(t, updatedBook.Contributors, 1)
	assert.Equal(t, newContributor.ID, updatedBook.Contributors[0].ContributorID)
	assert.Empty(t, updatedBook.Contributors[0].CreditedAs) // Cleared since now linked correctly
}

func TestUnmergeContributor_OnlyRelinksMatchingCreditedAs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create Stephen King with multiple aliases
	kingID, err := id.Generate("contributor")
	require.NoError(t, err)

	king := &domain.Contributor{
		Syncable: domain.Syncable{ID: kingID},
		Name:     "Stephen King",
		Aliases:  []string{"Richard Bachman", "John Swithen"},
	}
	king.InitTimestamps()
	err = store.CreateContributor(ctx, king)
	require.NoError(t, err)

	// Create books - one by Bachman, one by Swithen, one by King
	bachmanBookID, err := id.Generate("book")
	require.NoError(t, err)
	bachmanBook := &domain.Book{
		Syncable: domain.Syncable{ID: bachmanBookID},
		Title:    "The Running Man",
		Contributors: []domain.BookContributor{
			{ContributorID: kingID, Roles: []domain.ContributorRole{domain.RoleAuthor}, CreditedAs: "Richard Bachman"},
		},
	}
	bachmanBook.InitTimestamps()
	err = store.CreateBook(ctx, bachmanBook)
	require.NoError(t, err)

	swithenBookID, err := id.Generate("book")
	require.NoError(t, err)
	swithenBook := &domain.Book{
		Syncable: domain.Syncable{ID: swithenBookID},
		Title:    "Rage",
		Contributors: []domain.BookContributor{
			{ContributorID: kingID, Roles: []domain.ContributorRole{domain.RoleAuthor}, CreditedAs: "John Swithen"},
		},
	}
	swithenBook.InitTimestamps()
	err = store.CreateBook(ctx, swithenBook)
	require.NoError(t, err)

	kingBookID, err := id.Generate("book")
	require.NoError(t, err)
	kingBook := &domain.Book{
		Syncable: domain.Syncable{ID: kingBookID},
		Title:    "The Shining",
		Contributors: []domain.BookContributor{
			{ContributorID: kingID, Roles: []domain.ContributorRole{domain.RoleAuthor}, CreditedAs: ""},
		},
	}
	kingBook.InitTimestamps()
	err = store.CreateBook(ctx, kingBook)
	require.NoError(t, err)

	// Unmerge only Richard Bachman
	bachman, err := store.UnmergeContributor(ctx, kingID, "Richard Bachman")
	require.NoError(t, err)

	// Verify only Bachman book was re-linked (use GetBooksByContributor)
	bachmanBooks, err := store.GetBooksByContributor(ctx, bachman.ID)
	require.NoError(t, err)
	require.Len(t, bachmanBooks, 1)
	assert.Equal(t, "The Running Man", bachmanBooks[0].Title)
	assert.Equal(t, bachman.ID, bachmanBooks[0].Contributors[0].ContributorID)

	// King should still have Swithen and King books
	kingBooks, err := store.GetBooksByContributor(ctx, kingID)
	require.NoError(t, err)
	require.Len(t, kingBooks, 2) // Rage and The Shining

	// Find each book and verify linkage
	for _, book := range kingBooks {
		switch book.Title {
		case "Rage":
			assert.Equal(t, kingID, book.Contributors[0].ContributorID)
			assert.Equal(t, "John Swithen", book.Contributors[0].CreditedAs)
		case "The Shining":
			assert.Equal(t, kingID, book.Contributors[0].ContributorID)
			assert.Empty(t, book.Contributors[0].CreditedAs)
		}
	}

	// King should only have Swithen alias left
	updatedKing, err := store.GetContributor(ctx, kingID)
	require.NoError(t, err)
	assert.Equal(t, []string{"John Swithen"}, updatedKing.Aliases)
}

func TestUnmergeContributor_AliasNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create contributor without aliases
	kingID, err := id.Generate("contributor")
	require.NoError(t, err)

	king := &domain.Contributor{
		Syncable: domain.Syncable{ID: kingID},
		Name:     "Stephen King",
	}
	king.InitTimestamps()
	err = store.CreateContributor(ctx, king)
	require.NoError(t, err)

	// Try to unmerge non-existent alias
	_, err = store.UnmergeContributor(ctx, kingID, "Richard Bachman")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alias")
}
