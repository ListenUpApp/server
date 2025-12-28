package domain

// Series represents a sequence of related audiobooks.
// The wheel weaves as the wheel wills, and books flow in sequence.
type Series struct {
	Syncable
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	CoverImage  *ImageFileInfo `json:"cover_image,omitempty"`
	ASIN        string         `json:"asin,omitempty"` // Audible series identifier
}
