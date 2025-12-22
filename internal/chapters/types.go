// Package chapters provides chapter name detection and alignment functionality.
package chapters

// AlignedChapter represents a local chapter with a suggested remote name.
type AlignedChapter struct {
	Index         int     `json:"index"`
	StartTime     int64   `json:"startTime"`
	Duration      int64   `json:"duration"`
	CurrentName   string  `json:"currentName"`
	SuggestedName string  `json:"suggestedName"`
	Confidence    float64 `json:"confidence"`
}

// AlignmentResult contains the full alignment suggestion.
type AlignmentResult struct {
	Chapters          []AlignedChapter `json:"chapters"`
	OverallConfidence float64          `json:"overallConfidence"`
	NeedsUpdate       bool             `json:"needsUpdate"`
	ChapterCountMatch bool             `json:"chapterCountMatch"`
}

// AnalysisResult contains chapter analysis statistics.
type AnalysisResult struct {
	Total          int     `json:"total"`
	GenericCount   int     `json:"genericCount"`
	GenericPercent float64 `json:"genericPercent"`
	NeedsUpdate    bool    `json:"needsUpdate"`
}

// Chapter is an alias for domain.Chapter to avoid circular imports in tests.
// In production code, use domain.Chapter directly.
type Chapter struct {
	Title     string
	StartTime int64
	EndTime   int64
}

// RemoteChapter represents a chapter from Audible.
type RemoteChapter struct {
	Title      string
	StartMs    int64
	DurationMs int64
}
