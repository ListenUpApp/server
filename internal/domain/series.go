package domain

// Series represents a sequence of related audiobooks.
// The wheel weaves as the wheel wills, and books flow in sequence.
type Series struct {
	Syncable
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	TotalBooks  int    `json:"total_books"` // Known total, 0 if unknown/ongoing
	// Future: Completed bool, StartDate, EndDate
}
