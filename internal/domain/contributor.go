package domain

// Contributor represents a person who contributed to a book in any capacity.
type Contributor struct {
	Syncable
	Name          string   `json:"name"`
	SortName      string   `json:"sort_name,omitempty"` // "Sanderson, Brandon" for proper sorting
	Biography     string   `json:"biography,omitempty"`
	ImageURL      string   `json:"image_url,omitempty"`
	ImageBlurHash string   `json:"image_blur_hash,omitempty"` // BlurHash placeholder for image
	ASIN          string   `json:"asin,omitempty"`            // Amazon author ID for future metadata enrichment
	Aliases       []string `json:"aliases,omitempty"`         // Pen names: ["Richard Bachman", "John Swithen"]
	Website       string   `json:"website,omitempty"`         // Author's website URL
	BirthDate     string   `json:"birth_date,omitempty"`      // ISO 8601 date (e.g., "1947-09-21")
	DeathDate     string   `json:"death_date,omitempty"`      // ISO 8601 date (e.g., "2024-01-15")
}

// ContributorRole defines the type of contribution.
// "I have forgotten the face of my father" if you don't properly credit narrators.
type ContributorRole string

// Contributor role constants define the different types of contributions to a book.
const (
	RoleAuthor       ContributorRole = "author"
	RoleNarrator     ContributorRole = "narrator"
	RoleEditor       ContributorRole = "editor"
	RoleTranslator   ContributorRole = "translator"
	RoleForeword     ContributorRole = "foreword"
	RoleIntroduction ContributorRole = "introduction"
	RoleAfterword    ContributorRole = "afterword"
	RoleProducer     ContributorRole = "producer"
	RoleAdapter      ContributorRole = "adapter"
	RoleIllustrator  ContributorRole = "illustrator"
)

// String returns the string representation of the role.
func (r ContributorRole) String() string {
	return string(r)
}

// IsValid checks if the role is a recognized value.
func (r ContributorRole) IsValid() bool {
	switch r {
	case RoleAuthor, RoleNarrator, RoleEditor, RoleTranslator,
		RoleForeword, RoleIntroduction, RoleAfterword,
		RoleProducer, RoleAdapter, RoleIllustrator:
		return true
	default:
		return false
	}
}

// BookContributor links a book to a contributor with specific role(s).
// One person, many roles - the Ka of audiobook credits.
type BookContributor struct {
	ContributorID string            `json:"contributor_id"`
	Roles         []ContributorRole `json:"roles"`
	CreditedAs    string            `json:"credited_as,omitempty"` // Original name on this book (e.g., "Richard Bachman")
}
