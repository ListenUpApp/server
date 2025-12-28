package chapters

import (
	"regexp"
	"strings"
)

var genericPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^chapter\s+\d+$`),
	regexp.MustCompile(`(?i)^chapter\s+(one|two|three|four|five|six|seven|eight|nine|ten)$`),
	regexp.MustCompile(`(?i)^track\s+\d+$`),
	regexp.MustCompile(`(?i)^part\s+\d+$`),
	regexp.MustCompile(`(?i)^part\s+(one|two|three|four|five|six|seven|eight|nine|ten)$`),
	regexp.MustCompile(`^\d+$`),
	regexp.MustCompile(`^\d+\.\s*$`),
	regexp.MustCompile(`^\d+\s*-\s*$`),
}

// IsGenericName returns true if the chapter name is a placeholder.
func IsGenericName(name string) bool {
	name = strings.TrimSpace(name)

	// Empty or whitespace-only
	if name == "" {
		return true
	}

	// Check against patterns
	for _, pattern := range genericPatterns {
		if pattern.MatchString(name) {
			return true
		}
	}

	return false
}

// AnalyzeChapters returns statistics about the chapter names.
func AnalyzeChapters(chapters []Chapter) AnalysisResult {
	if len(chapters) == 0 {
		return AnalysisResult{}
	}

	generic := 0
	for _, ch := range chapters {
		if IsGenericName(ch.Title) {
			generic++
		}
	}

	percent := float64(generic) / float64(len(chapters))

	return AnalysisResult{
		Total:          len(chapters),
		GenericCount:   generic,
		GenericPercent: percent,
		NeedsUpdate:    percent > 0.5,
	}
}
