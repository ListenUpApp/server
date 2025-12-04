package domain

// Genre represents a category for classifying audiobooks.
// Genres form a hierarchy: Fiction -> Fantasy -> Epic Fantasy
// Books can belong to multiple genres.
type Genre struct {
	Syncable
	Name        string `json:"name"`                  // Display name: "Epic Fantasy"
	Slug        string `json:"slug"`                  // URL-safe key: "epic-fantasy"
	Description string `json:"description,omitempty"` // Optional description
	ParentID    string `json:"parent_id,omitempty"`   // Parent genre ID (empty for root)
	Path        string `json:"path"`                  // Materialized path: "/fiction/fantasy/epic-fantasy"
	Depth       int    `json:"depth"`                 // 0=root, 1=child, 2=grandchild
	SortOrder   int    `json:"sort_order"`            // Manual ordering within siblings
	Color       string `json:"color,omitempty"`       // Hex color for UI badges
	Icon        string `json:"icon,omitempty"`        // Icon identifier
	BookCount   int    `json:"book_count"`            // Books directly in this genre
	TotalBooks  int    `json:"total_books"`           // Books in this + all descendants
	IsSystem    bool   `json:"is_system"`             // System genres can't be deleted
}

// IsRoot returns true if this genre has no parent.
func (g *Genre) IsRoot() bool {
	return g.ParentID == ""
}

// BuildPath constructs the materialized path from parent path and slug.
func (g *Genre) BuildPath(parentPath string) string {
	if parentPath == "" {
		return "/" + g.Slug
	}
	return parentPath + "/" + g.Slug
}
