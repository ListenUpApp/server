package chapters

import "math"

// Align matches local chapters with remote chapters and returns suggestions.
// Local timestamps are preserved; only remote names are used.
func Align(local []Chapter, remote []RemoteChapter) AlignmentResult {
	if len(local) == 0 {
		return AlignmentResult{ChapterCountMatch: len(remote) == 0}
	}

	if len(remote) == 0 {
		// No remote chapters - return locals unchanged
		chapters := make([]AlignedChapter, len(local))
		for i, lc := range local {
			chapters[i] = AlignedChapter{
				Index:         i,
				StartTime:     lc.StartTime,
				Duration:      lc.EndTime - lc.StartTime,
				CurrentName:   lc.Title,
				SuggestedName: "",
				Confidence:    0,
			}
		}
		return AlignmentResult{Chapters: chapters}
	}

	countMatch := len(local) == len(remote)

	if countMatch {
		return alignExact(local, remote)
	}
	return alignByPosition(local, remote)
}

// alignExact performs 1:1 mapping when counts match.
func alignExact(local []Chapter, remote []RemoteChapter) AlignmentResult {
	chapters := make([]AlignedChapter, len(local))
	var totalConfidence float64

	for i, lc := range local {
		// Calculate position-based confidence even for exact match
		localTotal := totalLocalDuration(local)
		remoteTotal := totalRemoteDuration(remote)

		localPos := float64(lc.StartTime) / float64(localTotal)
		remotePos := float64(remote[i].StartMs) / float64(remoteTotal)
		dist := math.Abs(localPos - remotePos)
		confidence := 1.0 - dist

		chapters[i] = AlignedChapter{
			Index:         i,
			StartTime:     lc.StartTime,
			Duration:      lc.EndTime - lc.StartTime,
			CurrentName:   lc.Title,
			SuggestedName: remote[i].Title,
			Confidence:    confidence,
		}
		totalConfidence += confidence
	}

	return AlignmentResult{
		Chapters:          chapters,
		OverallConfidence: totalConfidence / float64(len(chapters)),
		ChapterCountMatch: true,
	}
}

// alignByPosition performs fuzzy matching when counts differ.
func alignByPosition(local []Chapter, remote []RemoteChapter) AlignmentResult {
	localTotal := totalLocalDuration(local)
	remoteTotal := totalRemoteDuration(remote)

	used := make([]bool, len(remote))
	chapters := make([]AlignedChapter, len(local))
	var totalConfidence float64
	matchedCount := 0

	for i, lc := range local {
		localPos := float64(lc.StartTime) / float64(localTotal)

		bestIdx := -1
		bestDist := 2.0

		for j, rc := range remote {
			if used[j] {
				continue
			}

			remotePos := float64(rc.StartMs) / float64(remoteTotal)
			dist := math.Abs(localPos - remotePos)

			if dist < bestDist {
				bestDist = dist
				bestIdx = j
			}
		}

		ac := AlignedChapter{
			Index:       i,
			StartTime:   lc.StartTime,
			Duration:    lc.EndTime - lc.StartTime,
			CurrentName: lc.Title,
		}

		if bestIdx >= 0 {
			used[bestIdx] = true
			ac.SuggestedName = remote[bestIdx].Title
			ac.Confidence = 1.0 - bestDist
			totalConfidence += ac.Confidence
			matchedCount++
		}

		chapters[i] = ac
	}

	var overallConfidence float64
	if matchedCount > 0 {
		overallConfidence = totalConfidence / float64(matchedCount)
	}

	return AlignmentResult{
		Chapters:          chapters,
		OverallConfidence: overallConfidence,
		ChapterCountMatch: false,
	}
}

func totalLocalDuration(chapters []Chapter) int64 {
	if len(chapters) == 0 {
		return 0
	}
	last := chapters[len(chapters)-1]
	return last.EndTime
}

func totalRemoteDuration(chapters []RemoteChapter) int64 {
	var total int64
	for _, ch := range chapters {
		total += ch.DurationMs
	}
	return total
}
