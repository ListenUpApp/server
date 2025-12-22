package chapters

import (
	"testing"
)

func TestAlign_ExactMatch(t *testing.T) {
	local := []Chapter{
		{Title: "Chapter 1", StartTime: 0, EndTime: 60000},
		{Title: "Chapter 2", StartTime: 60000, EndTime: 120000},
		{Title: "Chapter 3", StartTime: 120000, EndTime: 180000},
	}

	remote := []RemoteChapter{
		{Title: "The Boy Who Lived", StartMs: 0, DurationMs: 60000},
		{Title: "The Vanishing Glass", StartMs: 60000, DurationMs: 60000},
		{Title: "The Letters from No One", StartMs: 120000, DurationMs: 60000},
	}

	result := Align(local, remote)

	if !result.ChapterCountMatch {
		t.Error("expected ChapterCountMatch to be true")
	}

	if len(result.Chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(result.Chapters))
	}

	expected := []string{"The Boy Who Lived", "The Vanishing Glass", "The Letters from No One"}
	for i, ch := range result.Chapters {
		if ch.SuggestedName != expected[i] {
			t.Errorf("chapter %d: expected %q, got %q", i, expected[i], ch.SuggestedName)
		}
		if ch.Confidence < 0.9 {
			t.Errorf("chapter %d: expected high confidence, got %f", i, ch.Confidence)
		}
	}
}

func TestAlign_MoreLocalThanRemote(t *testing.T) {
	// 6 local chapters, 3 remote - should use greedy matching
	local := []Chapter{
		{Title: "Track 1", StartTime: 0, EndTime: 30000},
		{Title: "Track 2", StartTime: 30000, EndTime: 60000},
		{Title: "Track 3", StartTime: 60000, EndTime: 90000},
		{Title: "Track 4", StartTime: 90000, EndTime: 120000},
		{Title: "Track 5", StartTime: 120000, EndTime: 150000},
		{Title: "Track 6", StartTime: 150000, EndTime: 180000},
	}

	remote := []RemoteChapter{
		{Title: "Part One", StartMs: 0, DurationMs: 60000},
		{Title: "Part Two", StartMs: 60000, DurationMs: 60000},
		{Title: "Part Three", StartMs: 120000, DurationMs: 60000},
	}

	result := Align(local, remote)

	if result.ChapterCountMatch {
		t.Error("expected ChapterCountMatch to be false")
	}

	// Should have suggestions for all 6, but only 3 unique remote names
	if len(result.Chapters) != 6 {
		t.Fatalf("expected 6 chapters, got %d", len(result.Chapters))
	}

	// Check no duplicate remote names (greedy unique)
	usedNames := make(map[string]bool)
	namedCount := 0
	for _, ch := range result.Chapters {
		if ch.SuggestedName != "" {
			if usedNames[ch.SuggestedName] {
				t.Errorf("duplicate suggested name: %q", ch.SuggestedName)
			}
			usedNames[ch.SuggestedName] = true
			namedCount++
		}
	}

	if namedCount != 3 {
		t.Errorf("expected 3 named chapters, got %d", namedCount)
	}
}

func TestAlign_MoreRemoteThanLocal(t *testing.T) {
	local := []Chapter{
		{Title: "1", StartTime: 0, EndTime: 90000},
		{Title: "2", StartTime: 90000, EndTime: 180000},
	}

	remote := []RemoteChapter{
		{Title: "Opening", StartMs: 0, DurationMs: 30000},
		{Title: "Act One", StartMs: 30000, DurationMs: 30000},
		{Title: "Intermission", StartMs: 60000, DurationMs: 30000},
		{Title: "Act Two", StartMs: 90000, DurationMs: 60000},
		{Title: "Finale", StartMs: 150000, DurationMs: 30000},
	}

	result := Align(local, remote)

	if result.ChapterCountMatch {
		t.Error("expected ChapterCountMatch to be false")
	}

	// All local chapters should have suggestions
	for i, ch := range result.Chapters {
		if ch.SuggestedName == "" {
			t.Errorf("chapter %d has no suggestion", i)
		}
	}
}

func TestAlign_ConfidenceScoring(t *testing.T) {
	// Perfect alignment should have high confidence
	local := []Chapter{
		{Title: "Ch 1", StartTime: 0, EndTime: 100000},
	}
	remote := []RemoteChapter{
		{Title: "Introduction", StartMs: 0, DurationMs: 100000},
	}

	result := Align(local, remote)

	if result.Chapters[0].Confidence < 0.95 {
		t.Errorf("expected high confidence for perfect match, got %f", result.Chapters[0].Confidence)
	}
}
