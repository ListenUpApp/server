package domain

import (
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContributorRole_String(t *testing.T) {
	tests := []struct {
		role     ContributorRole
		expected string
	}{
		{RoleAuthor, "author"},
		{RoleNarrator, "narrator"},
		{RoleEditor, "editor"},
		{RoleTranslator, "translator"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.role.String())
		})
	}
}

func TestContributorRole_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		role     ContributorRole
		expected bool
	}{
		{"author is valid", RoleAuthor, true},
		{"narrator is valid", RoleNarrator, true},
		{"editor is valid", RoleEditor, true},
		{"translator is valid", RoleTranslator, true},
		{"unknown role is invalid", ContributorRole("unknown"), false},
		{"empty role is invalid", ContributorRole(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.role.IsValid())
		})
	}
}

func TestContributor_JSONMarshaling(t *testing.T) {
	contributor := &Contributor{
		Syncable: Syncable{
			ID: "contrib-123",
		},
		Name:      "Brandon Sanderson",
		SortName:  "Sanderson, Brandon",
		Biography: "Epic fantasy author",
		ImageURL:  "https://example.com/brandon.jpg",
		ASIN:      "B001IGFHW6",
	}
	contributor.InitTimestamps()

	// Marshal to JSON
	data, err := json.Marshal(contributor)
	require.NoError(t, err)

	// Unmarshal back
	var decoded Contributor
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, contributor.ID, decoded.ID)
	assert.Equal(t, contributor.Name, decoded.Name)
	assert.Equal(t, contributor.SortName, decoded.SortName)
	assert.Equal(t, contributor.Biography, decoded.Biography)
	assert.Equal(t, contributor.ImageURL, decoded.ImageURL)
	assert.Equal(t, contributor.ASIN, decoded.ASIN)
	assert.Equal(t, contributor.CreatedAt.Unix(), decoded.CreatedAt.Unix())
	assert.Equal(t, contributor.UpdatedAt.Unix(), decoded.UpdatedAt.Unix())
}

func TestBookContributor_JSONMarshaling(t *testing.T) {
	bc := BookContributor{
		ContributorID: "contrib-123",
		Roles:         []ContributorRole{RoleAuthor, RoleNarrator},
	}

	// Marshal to JSON
	data, err := json.Marshal(bc)
	require.NoError(t, err)

	// Unmarshal back
	var decoded BookContributor
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, bc.ContributorID, decoded.ContributorID)
	require.Len(t, decoded.Roles, 2)
	assert.Equal(t, RoleAuthor, decoded.Roles[0])
	assert.Equal(t, RoleNarrator, decoded.Roles[1])
}

func TestBookContributor_MultipleRoles(t *testing.T) {
	// Test the "one person, many roles" scenario
	// Brandon Sanderson narrating his own work
	bc := BookContributor{
		ContributorID: "contrib-sanderson",
		Roles:         []ContributorRole{RoleAuthor, RoleNarrator},
	}

	assert.Len(t, bc.Roles, 2)
	assert.Contains(t, bc.Roles, RoleAuthor)
	assert.Contains(t, bc.Roles, RoleNarrator)
}
