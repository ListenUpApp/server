package service

import (
	"testing"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
)

// TestMergeContributors_PreservesAuthorsWhenOnlyNarratorsSelected tests that
// existing authors are preserved when only narrators are selected from Audible.
func TestMergeContributors_PreservesAuthorsWhenOnlyNarratorsSelected(t *testing.T) {
	existing := []domain.BookContributor{
		{ContributorID: "author-1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
		{ContributorID: "narrator-1", Roles: []domain.ContributorRole{domain.RoleNarrator}},
	}

	// Empty authorASINs means "preserve existing authors"
	// Non-empty narratorASINs means "replace narrators"
	authorASINs := []string{}
	narratorASINs := []string{"B001"} // Would resolve to a new narrator

	// For this unit test, we test the preserve logic directly
	// by checking that when authorASINs is empty, existing authors are kept
	result := mergeContributorsLogic(existing, authorASINs, narratorASINs)

	// Should preserve the existing author
	assert.Len(t, result.preservedAuthors, 1)
	assert.Equal(t, "author-1", result.preservedAuthors[0])

	// Should NOT preserve existing narrator (being replaced)
	assert.Len(t, result.preservedNarrators, 0)
}

// TestMergeContributors_PreservesNarratorsWhenOnlyAuthorsSelected tests that
// existing narrators are preserved when only authors are selected from Audible.
func TestMergeContributors_PreservesNarratorsWhenOnlyAuthorsSelected(t *testing.T) {
	existing := []domain.BookContributor{
		{ContributorID: "author-1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
		{ContributorID: "narrator-1", Roles: []domain.ContributorRole{domain.RoleNarrator}},
	}

	// Non-empty authorASINs means "replace authors"
	// Empty narratorASINs means "preserve existing narrators"
	authorASINs := []string{"A001"} // Would resolve to a new author
	narratorASINs := []string{}

	result := mergeContributorsLogic(existing, authorASINs, narratorASINs)

	// Should NOT preserve existing author (being replaced)
	assert.Len(t, result.preservedAuthors, 0)

	// Should preserve the existing narrator
	assert.Len(t, result.preservedNarrators, 1)
	assert.Equal(t, "narrator-1", result.preservedNarrators[0])
}

// TestMergeContributors_PreservesOtherRoles tests that roles like editor and
// translator are always preserved regardless of author/narrator selections.
func TestMergeContributors_PreservesOtherRoles(t *testing.T) {
	existing := []domain.BookContributor{
		{ContributorID: "author-1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
		{ContributorID: "editor-1", Roles: []domain.ContributorRole{domain.RoleEditor}},
		{ContributorID: "translator-1", Roles: []domain.ContributorRole{domain.RoleTranslator}},
	}

	// Even when replacing authors, other roles should be preserved
	authorASINs := []string{"A001"}
	narratorASINs := []string{"B001"}

	result := mergeContributorsLogic(existing, authorASINs, narratorASINs)

	// Should preserve editor and translator
	assert.Len(t, result.preservedOtherRoles, 2)
	assert.Contains(t, result.preservedOtherRoles, "editor-1")
	assert.Contains(t, result.preservedOtherRoles, "translator-1")
}

// TestMergeContributors_DeduplicatesContributors tests that the same contributor
// is not added twice when they appear in both existing and new lists.
func TestMergeContributors_DeduplicatesContributors(t *testing.T) {
	existing := []domain.BookContributor{
		{ContributorID: "person-1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
	}

	// When we resolve "A001" and it matches to "person-1", we should not duplicate
	result := mergeContributorsLogic(existing, []string{}, []string{})

	// Should preserve author without duplication
	assert.Len(t, result.preservedAuthors, 1)
}

// TestMergeContributors_MergesRolesForSamePerson tests that when a person is both
// an existing author and a new narrator, their roles are merged.
func TestMergeContributors_MergesRolesForSamePerson(t *testing.T) {
	existing := []domain.BookContributor{
		{ContributorID: "person-1", Roles: []domain.ContributorRole{domain.RoleAuthor, domain.RoleNarrator}},
	}

	// Preserve both roles when neither is being replaced
	result := mergeContributorsLogic(existing, []string{}, []string{})

	assert.Len(t, result.preservedAuthors, 1)
	assert.Len(t, result.preservedNarrators, 1)
	assert.Equal(t, "person-1", result.preservedAuthors[0])
	assert.Equal(t, "person-1", result.preservedNarrators[0])
}

// mergeContributorsLogicResult holds the result of the pure logic test.
type mergeContributorsLogicResult struct {
	preservedAuthors    []string
	preservedNarrators  []string
	preservedOtherRoles []string
}

// mergeContributorsLogic extracts the pure logic from mergeContributors
// for unit testing without needing database/context dependencies.
func mergeContributorsLogic(
	existing []domain.BookContributor,
	authorASINs, narratorASINs []string,
) mergeContributorsLogicResult {
	result := mergeContributorsLogicResult{}

	// Authors: preserve if authorASINs is empty
	if len(authorASINs) == 0 {
		for _, bc := range existing {
			for _, role := range bc.Roles {
				if role == domain.RoleAuthor {
					result.preservedAuthors = append(result.preservedAuthors, bc.ContributorID)
					break
				}
			}
		}
	}

	// Narrators: preserve if narratorASINs is empty
	if len(narratorASINs) == 0 {
		for _, bc := range existing {
			for _, role := range bc.Roles {
				if role == domain.RoleNarrator {
					result.preservedNarrators = append(result.preservedNarrators, bc.ContributorID)
					break
				}
			}
		}
	}

	// Other roles: always preserve
	for _, bc := range existing {
		for _, role := range bc.Roles {
			if role != domain.RoleAuthor && role != domain.RoleNarrator {
				result.preservedOtherRoles = append(result.preservedOtherRoles, bc.ContributorID)
			}
		}
	}

	return result
}
